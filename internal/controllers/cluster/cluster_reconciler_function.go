/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package cluster

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/miekg/dns"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clnt "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	privatev1 "github.com/innabox/fulfillment-service/internal/api/private/v1"
	sharedv1 "github.com/innabox/fulfillment-service/internal/api/shared/v1"
	"github.com/innabox/fulfillment-service/internal/controllers"
	"github.com/innabox/fulfillment-service/internal/controllers/finalizers"
	"github.com/innabox/fulfillment-service/internal/jq"
	"github.com/innabox/fulfillment-service/internal/json"
	"github.com/innabox/fulfillment-service/internal/kubernetes/gvks"
	"github.com/innabox/fulfillment-service/internal/kubernetes/labels"
)

// FunctionBuilder contains the data and logic needed to build a function that reconciles clustes.
type FunctionBuilder struct {
	logger       *slog.Logger
	connection   *grpc.ClientConn
	hubCache     *controllers.HubCache
	dnsServer    string
	dnsZone      string
	dnsKeyName   string
	dnsKeySecret string
}

type function struct {
	logger            *slog.Logger
	hubCache          *controllers.HubCache
	clustersClient    privatev1.ClustersClient
	hubsClient        privatev1.HubsClient
	hostClassesClient privatev1.HostClassesClient
	hostsClient       privatev1.HostsClient
	dnsServer         string
	dnsZone           string
	dnsZoneFq         string
	dnsKeyName        string
	dnsKeyNameFq      string
	dnsKeySecret      string
	dnsClient         *dns.Client
	jqTool            *jq.Tool
}

type task struct {
	parent        *function
	logger        *slog.Logger
	cluster       *privatev1.Cluster
	hub           *privatev1.Hub
	alias         string
	baseDomain    string
	apiDomain     string
	apiIntDomain  string
	ingressDomain string
	hubClient     clnt.Client
	pullSecret    *corev1.Secret
	sshSecret     *corev1.Secret
	hostedCluster *unstructured.Unstructured
	nodePools     map[string]*unstructured.Unstructured
}

// NewFunction creates a new builder that can then be used to create a new cluster reconciler function.
func NewFunction() *FunctionBuilder {
	return &FunctionBuilder{}
}

// SetLogger sets the logger. This is mandatory.
func (b *FunctionBuilder) SetLogger(value *slog.Logger) *FunctionBuilder {
	b.logger = value
	return b
}

// SetConnection sets the gRPC client connection. This is mandatory.
func (b *FunctionBuilder) SetConnection(value *grpc.ClientConn) *FunctionBuilder {
	b.connection = value
	return b
}

// SetHubCache sets the cache of hubs. This is mandatory.
func (b *FunctionBuilder) SetHubCache(value *controllers.HubCache) *FunctionBuilder {
	b.hubCache = value
	return b
}

// SetDnsServer sets the DNS server address for dynamic updates (e.g., "my-dns-server.com:53"). This is optional.
func (b *FunctionBuilder) SetDnsServer(value string) *FunctionBuilder {
	b.dnsServer = value
	return b
}

// SetDnsZone sets the DNS zone for dynamic updates (e.g., "my.demo"). This is optional.
func (b *FunctionBuilder) SetDnsZone(value string) *FunctionBuilder {
	b.dnsZone = value
	return b
}

// SetDnsKeyName sets the name of the TSIG key for authentication. This is optional.
//
// For example, to generate a TSIG key for use wit the 'bind' DNS server you can use the follwing command:
//
//	tsig-keygen -a hmac-sha256 fulfillment-service
//
// It will generate something like this:
//
//	key "my-key" {
//		algorithm hmac-sha256;
//		secret "...";
//	};
//
// That needs to be included in the 'named.confg' configuration file.
//
// You will also need a zone with update permissions for this key, something like this:
//
//	zone "my.demo" in {
//		type master;
//		file "my.demo.zone";
//		update-policy {
//			grant my-key zonesub ANY;
//		};
//	};
//
// The you will need to reload the DNS server to apply the changes.
//
// The key name, 'my-key' in this example, is what should be passed to the SetDnsKeyName function. The secret is what
// should be passed to the SetDnsKeySecret function.
func (b *FunctionBuilder) SetDnsKeyName(value string) *FunctionBuilder {
	b.dnsKeyName = value
	return b
}

// SetDnsKeySecret sets the secret value of the TSIG key for authentication. This is optional.
//
// See the SetDnsKeyName function for more information.
func (b *FunctionBuilder) SetDnsKeySecret(value string) *FunctionBuilder {
	b.dnsKeySecret = value
	return b
}

// Build uses the information stored in the buidler to create a new cluster reconciler.
func (b *FunctionBuilder) Build() (result controllers.ReconcilerFunction[*privatev1.Cluster], err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.connection == nil {
		err = errors.New("client is mandatory")
		return
	}
	if b.hubCache == nil {
		err = errors.New("hub cache is mandatory")
		return
	}

	// The DNS library that we use requires fully qualified domain names, but HyperShift chokes on the training
	// dot, so we need to have both flavours.
	dnsZoneFq := dns.Fqdn(b.dnsZone)
	dnsKeyNameFq := dns.Fqdn(b.dnsKeyName)

	// Create DNS client if DNS server is provided:
	var dnsClient *dns.Client
	if b.dnsServer != "" {
		dnsClient = &dns.Client{
			Net:     "tcp",
			Timeout: 5 * time.Second,
		}
		if b.dnsKeyName != "" && b.dnsKeySecret != "" {
			dnsClient.TsigSecret = map[string]string{
				dnsKeyNameFq: b.dnsKeySecret,
			}
		}
	}

	// Create the JQ tool:
	jqTool, err := jq.NewTool().
		SetLogger(b.logger).
		Build()
	if err != nil {
		return nil, fmt.Errorf("failed to create JQ tool: %w", err)
	}

	// Create and populate the object:
	object := &function{
		logger:            b.logger,
		clustersClient:    privatev1.NewClustersClient(b.connection),
		hubsClient:        privatev1.NewHubsClient(b.connection),
		hostsClient:       privatev1.NewHostsClient(b.connection),
		hostClassesClient: privatev1.NewHostClassesClient(b.connection),
		hubCache:          b.hubCache,
		dnsServer:         b.dnsServer,
		dnsZone:           b.dnsZone,
		dnsZoneFq:         dnsZoneFq,
		dnsKeyName:        b.dnsKeyName,
		dnsKeyNameFq:      dnsKeyNameFq,
		dnsKeySecret:      b.dnsKeySecret,
		dnsClient:         dnsClient,
		jqTool:            jqTool,
	}
	result = object.run
	return
}

func (f *function) run(ctx context.Context, cluster *privatev1.Cluster) error {
	logger := f.logger.With(
		slog.String("cluster", cluster.GetId()),
	)
	oldCluster := proto.Clone(cluster).(*privatev1.Cluster)
	t := task{
		parent:  f,
		logger:  logger,
		cluster: cluster,
	}
	var err error
	if cluster.GetMetadata().HasDeletionTimestamp() {
		err = t.delete(ctx)
	} else {
		err = t.update(ctx)
	}
	if err != nil {
		return err
	}
	if !proto.Equal(cluster, oldCluster) {
		_, err = f.clustersClient.Update(ctx, privatev1.ClustersUpdateRequest_builder{
			Object: cluster,
		}.Build())
	}
	return err
}

func (t *task) update(ctx context.Context) error {
	// If there is no finalizer yet then add, set the defaults and return. That will trigger another reconciliation
	// and we will perform the rest of the update there.
	if !t.hasFinalizer() {
		t.addFinalizer()
		t.setDefaults()
		return nil
	}

	// Shortcuts for the cluster metadata and spec:
	clusterMeta := t.cluster.GetMetadata()
	clusterStatus := t.cluster.GetStatus()

	// Select the hub, and if no hubs are available, update the condition to inform the user that creation is
	// pending. Note that we don't want to disclose the existence of hubs to the user, as that is a internal
	// implementation detail, so keep the message generic enough to not reveal that information.
	err := t.selectHub(ctx)
	if err != nil {
		t.logger.ErrorContext(
			ctx,
			"Failed to select hub",
			slog.String("error", err.Error()),
		)
		t.updateCondition(
			privatev1.ClusterConditionType_CLUSTER_CONDITION_TYPE_PROGRESSING,
			sharedv1.ConditionStatus_CONDITION_STATUS_FALSE,
			"ResourcesUnavailable",
			"The cluster cannot be created because there are no resources available to fulfill the "+
				"request.",
		)
		return nil
	}

	// Save the selected hub in the private data of the cluster:
	clusterStatus.SetHub(t.hub.GetId())

	// Select a port number for the API service:
	err = t.selectPort(ctx)
	if err != nil {
		return err
	}

	// Select an alias for the cluster:
	err = t.selectAlias(ctx)
	if err != nil {
		return err
	}

	// Calculate and synchronize the DNS records::
	t.baseDomain = fmt.Sprintf("%s.%s", t.alias, t.parent.dnsZone)
	t.apiDomain = fmt.Sprintf("api.%s", t.baseDomain)
	t.apiIntDomain = fmt.Sprintf("api-int.%s", t.baseDomain)
	t.ingressDomain = fmt.Sprintf("*.apps.%s", t.baseDomain)
	err = t.syncDnsRecords(ctx)
	if err != nil {
		return err
	}

	// Ensure that the pull secret exists:
	t.pullSecret = &corev1.Secret{}
	t.pullSecret.SetNamespace(t.hub.GetNamespace())
	t.pullSecret.SetName(fmt.Sprintf("%s-pull", clusterMeta.GetName()))
	_, err = controllerutil.CreateOrPatch(ctx, t.hubClient, t.pullSecret, t.mutatePullSecret)
	if err != nil {
		return err
	}

	// Ensure that the SSH key secret exists:
	t.sshSecret = &corev1.Secret{}
	t.sshSecret.SetNamespace(t.hub.GetNamespace())
	t.sshSecret.SetName(fmt.Sprintf("%s-ssh", clusterMeta.GetName()))
	_, err = controllerutil.CreateOrPatch(ctx, t.hubClient, t.sshSecret, t.mutateSshSecret)
	if err != nil {
		return err
	}

	// Ensure that the hosted cluster exists:
	t.hostedCluster = &unstructured.Unstructured{}
	t.hostedCluster.SetGroupVersionKind(gvks.HostedCluster)
	t.hostedCluster.SetNamespace(t.hub.GetNamespace())
	t.hostedCluster.SetName(clusterMeta.GetName())
	_, err = controllerutil.CreateOrPatch(ctx, t.hubClient, t.hostedCluster, t.mutateHostedCluster)
	if err != nil {
		return err
	}

	// Ensure that there is a node pool for each node set:
	t.nodePools = map[string]*unstructured.Unstructured{}
	for nodeSetName := range t.cluster.GetSpec().GetNodeSets() {
		nodePool := &unstructured.Unstructured{}
		nodePool.SetGroupVersionKind(gvks.NodePool)
		nodePool.SetNamespace(t.hostedCluster.GetNamespace())
		nodePool.SetName(fmt.Sprintf("%s-%s", t.hostedCluster.GetName(), nodeSetName))
		t.nodePools[nodeSetName] = nodePool
		_, err = controllerutil.CreateOrPatch(ctx, t.hubClient, nodePool, func() error {
			return t.mutateNodePool(nodeSetName)
		})
		if err != nil {
			return err
		}
	}

	// Scale the node sets up or down as needed:
	err = t.scaleNodeSets(ctx)
	if err != nil {
		return err
	}

	// Synchronize the state of the from the hosted cluster:
	err = t.syncFromHostedCluster(ctx)
	if err != nil {
		return err
	}

	return nil
}

func (t *task) mutatePullSecret() error {
	t.mutateLabels(t.pullSecret)
	t.pullSecret.Type = corev1.SecretTypeDockerConfigJson
	if t.pullSecret.Data == nil {
		t.pullSecret.Data = make(map[string][]byte)
	}
	t.pullSecret.Data[corev1.DockerConfigJsonKey] = []byte(t.hub.GetPullSecret())
	return nil
}

func (t *task) mutateSshSecret() error {
	t.mutateLabels(t.sshSecret)
	t.sshSecret.Type = corev1.SecretTypeOpaque
	if t.sshSecret.StringData == nil {
		t.sshSecret.StringData = make(map[string]string)
	}
	t.sshSecret.StringData["id_rsa.pub"] = t.hub.GetSshPublicKey()
	return nil
}

func (t *task) mutateHostedCluster() error {
	t.mutateLabels(t.hostedCluster)
	t.mutateFinalizers(t.hostedCluster)
	spec, err := t.renderHostedClusterSpec()
	if err != nil {
		return err
	}
	err = unstructured.SetNestedMap(t.hostedCluster.Object, spec, "spec")
	if err != nil {
		return err
	}

	return nil
}

func (t *task) renderHostedClusterSpec() (result json.Object, err error) {
	dnsSpec, err := t.renderHostedClusterDns()
	if err != nil {
		return
	}
	etcdSpec, err := t.renderHostedClusterEtcd()
	if err != nil {
		return
	}
	networkingSpec, err := t.renderHostedClusterNetworking()
	if err != nil {
		return
	}
	platformSpec, err := t.renderHostedClusterPlatform()
	if err != nil {
		return
	}
	releaseSpec, err := t.renderRelease()
	if err != nil {
		return
	}
	servicesSpec, err := t.renderHostedClusterServices()
	if err != nil {
		return
	}
	pullSecretRef := json.Object{
		"name": t.pullSecret.GetName(),
	}
	sshKeyRef := json.Object{
		"name": t.sshSecret.GetName(),
	}
	result = json.Object{
		"clusterID":                        t.cluster.GetId(),
		"controllerAvailabilityPolicy":     "SingleReplica",
		"dns":                              dnsSpec,
		"etcd":                             etcdSpec,
		"infraID":                          t.cluster.GetId(),
		"infrastructureAvailabilityPolicy": "SingleReplica",
		"issuerURL":                        "https://kubernetes.default.svc",
		"networking":                       networkingSpec,
		"olmCatalogPlacement":              "guest",
		"platform":                         platformSpec,
		"pullSecret":                       pullSecretRef,
		"release":                          releaseSpec,
		"services":                         servicesSpec,
		"sshKey":                           sshKeyRef,
	}
	return
}

func (t *task) renderHostedClusterDns() (result json.Object, err error) {
	result = json.Object{
		"baseDomain": t.parent.dnsZone,
	}
	return
}

func (t *task) renderHostedClusterEtcd() (result json.Object, err error) {
	result = json.Object{
		"managed": json.Object{
			"storage": json.Object{
				"type": "PersistentVolume",
				"persistentVolume": json.Object{
					"size": "8Gi",
				},
			},
		},
		"managementType": "Managed",
	}
	return
}

func (t *task) renderHostedClusterNetworking() (result json.Object, err error) {
	result = json.Object{
		"clusterNetwork": json.List{
			json.Object{
				"cidr": "10.132.0.0/14",
			},
		},
		"networkType": "OVNKubernetes",
		"serviceNetwork": json.List{
			json.Object{
				"cidr": "172.31.0.0/16",
			},
		},
	}
	return
}

func (t *task) renderHostedClusterPlatform() (result json.Object, err error) {
	result = json.Object{
		"agent": json.Object{
			"agentNamespace": t.hub.GetNamespace(),
		},
		"type": "Agent",
	}
	return
}

func (t *task) renderHostedClusterServices() (result json.List, err error) {
	result = json.List{
		json.Object{
			"service": "APIServer",
			"servicePublishingStrategy": json.Object{
				"type": "NodePort",
				"nodePort": json.Object{
					"address": t.apiDomain,
					"port":    int64(t.cluster.GetStatus().GetPort()),
				},
			},
		},
		json.Object{
			"service": "OAuthServer",
			"servicePublishingStrategy": json.Object{
				"type": "Route",
			},
		},
		json.Object{
			"service": "OIDC",
			"servicePublishingStrategy": json.Object{
				"type": "Route",
			},
		},
		json.Object{
			"service": "Konnectivity",
			"servicePublishingStrategy": json.Object{
				"type": "Route",
			},
		},
		json.Object{
			"service": "Ignition",
			"servicePublishingStrategy": json.Object{
				"type": "Route",
			},
		},
	}
	return
}

func (t *task) mutateNodePool(nodeSetName string) error {
	t.mutateLabels(t.nodePools[nodeSetName])
	nodeSet := t.cluster.GetSpec().GetNodeSets()[nodeSetName]
	if nodeSet == nil {
		return fmt.Errorf("node set '%s' not found", nodeSetName)
	}
	spec, err := t.renderNodePoolSpec(nodeSet)
	if err != nil {
		return err
	}
	nodePool := t.nodePools[nodeSetName]
	err = unstructured.SetNestedMap(nodePool.Object, spec, "spec")
	if err != nil {
		return err
	}

	return nil
}

func (t *task) renderNodePoolSpec(nodeSet *privatev1.ClusterNodeSet) (result json.Object, err error) {
	platformSpec, err := t.renderNodePoolPlatform()
	if err != nil {
		return
	}
	managementSpec, err := t.renderNodePoolManagement()
	if err != nil {
		return
	}
	releaseSpec, err := t.renderRelease()
	if err != nil {
		return
	}
	result = json.Object{
		"arch":        "amd64",
		"clusterName": t.hostedCluster.GetName(),
		"management":  managementSpec,
		"platform":    platformSpec,
		"release":     releaseSpec,
		"replicas":    int64(nodeSet.GetSize()),
	}
	return
}

func (t *task) renderNodePoolPlatform() (result json.Object, err error) {
	result = json.Object{
		"type": "Agent",
		"agent": json.Object{
			"agentLabelSelector": json.Object{
				"matchLabels": json.Object{
					labels.ClusterId: t.cluster.GetId(),
				},
			},
		},
	}
	return
}

func (t *task) renderNodePoolManagement() (result json.Object, err error) {
	result = json.Object{
		"upgradeType": "Replace",
	}
	return
}

func (t *task) renderRelease() (result json.Object, err error) {
	result = json.Object{
		"image": "quay.io/openshift-release-dev/ocp-release:4.19.18-x86_64",
	}
	return
}

func (t *task) mutateLabels(object clnt.Object) {
	values := object.GetLabels()
	if values == nil {
		values = map[string]string{}
	}
	values[labels.ClusterId] = t.cluster.GetId()
	values[labels.ClusterName] = t.cluster.GetMetadata().GetName()
	object.SetLabels(values)
}

func (t *task) mutateFinalizers(object clnt.Object) {
	list := object.GetFinalizers()
	if !slices.Contains(list, finalizers.Controller) {
		list = append(list, finalizers.Controller)
		object.SetFinalizers(list)
	}
}

// syncDnsRecords synchronizes the DNS records for the cluster. It calculates the desired records, compares then with
// the current records and sends the necessary updates to the DNS server.
func (t *task) syncDnsRecords(ctx context.Context) error {
	// Get the desired and current records and calculate the difference:
	desiredRecords, err := t.desiredDnsRecords(ctx)
	if err != nil {
		return err
	}
	currentRecords, err := t.currentDnsRecords(ctx)
	if err != nil {
		return err
	}
	addedRecords, removedRecords, err := t.diffDnsRecords(ctx, currentRecords, desiredRecords)
	if err != nil {
		return err
	}

	// If there are no changes, don't send an update:
	if len(addedRecords) == 0 && len(removedRecords) == 0 {
		t.logger.DebugContext(
			ctx,
			"No DNS changes detected, skipping update",
		)
		return nil
	}

	// Prepare the update message:
	request := &dns.Msg{}
	request.SetUpdate(t.parent.dnsZoneFq)
	request.Insert(addedRecords)
	request.Remove(removedRecords)

	// Sign the message with TSIG if configured:
	if t.parent.dnsKeyName != "" && t.parent.dnsKeySecret != "" {
		request.SetTsig(
			t.parent.dnsKeyNameFq,
			dns.HmacSHA256,
			defaultDnsTtl,
			time.Now().Unix(),
		)
	}

	// Send the message to the DNS server:
	t.logger.DebugContext(
		ctx,
		"Sending DNS update request",
		slog.String("server", t.parent.dnsServer),
		slog.Any("request", request),
	)
	response, _, err := t.parent.dnsClient.Exchange(request, t.parent.dnsServer)
	if err != nil {
		return err
	}
	t.logger.DebugContext(
		ctx,
		"Received DNS update response",
		slog.String("server", t.parent.dnsServer),
		slog.Any("response", response),
	)
	if response.Rcode != dns.RcodeSuccess {
		t.logger.ErrorContext(
			ctx,
			"Failed to update DNS",
			slog.String("server", t.parent.dnsServer),
			slog.Int("rcode", int(response.Rcode)),
		)
		return fmt.Errorf(
			"failed to send DNS update to '%s', response code is %d",
			t.parent.dnsServer, response.Rcode,
		)
	}

	return nil
}

// desiredDnsRecords calculates the DNS records for the cluster.
func (t *task) desiredDnsRecords(ctx context.Context) (result []dns.RR, err error) {
	// Get the IP addresses of the nodes of the cluster:
	var hostIds []string
	for _, nodeSet := range t.cluster.GetStatus().GetNodeSets() {
		hostIds = append(hostIds, nodeSet.GetHosts()...)
	}
	for i, hostId := range hostIds {
		hostIds[i] = strconv.Quote(hostId)
	}
	hostsFilter := fmt.Sprintf("this.id in [%s]", strings.Join(hostIds, ","))
	hostsResponse, err := t.parent.hostsClient.List(ctx, privatev1.HostsListRequest_builder{
		Filter: &hostsFilter,
		Limit:  proto.Int32(int32(len(hostIds))),
	}.Build())
	if err != nil {
		return
	}
	var hostIps []string
	for _, host := range hostsResponse.GetItems() {
		hostIp := host.GetSpec().GetBootIp()
		if hostIp == "" {
			continue
		}
		if !slices.Contains(hostIps, hostIp) {
			hostIps = append(hostIps, hostIp)
		}
	}

	// We will collect the DNS records here:
	records := []dns.RR{}

	// Calculate A records for each IP address in the ingress domain:
	ingressDomainFq := dns.Fqdn(t.ingressDomain)
	for _, hostIp := range hostIps {
		parsedIp := net.ParseIP(hostIp)
		if parsedIp == nil {
			t.logger.WarnContext(
				ctx,
				"Skipping invalid host IP",
				slog.String("ip", hostIp),
			)
			continue
		}
		ingressRecord := &dns.A{
			Hdr: dns.RR_Header{
				Name:   ingressDomainFq,
				Ttl:    defaultDnsTtl,
				Class:  dns.ClassINET,
				Rrtype: dns.TypeA,
			},
			A: parsedIp,
		}
		records = append(records, ingressRecord)
	}

	// Calculate A records for the hub IP in the API domains:
	hubIp := net.ParseIP(t.hub.GetIp())
	if hubIp == nil {
		err = fmt.Errorf("invalid hub IP address: '%s'", t.hub.GetIp())
		return
	}
	apiDomainFq := dns.Fqdn(t.apiDomain)
	apiRecord := &dns.A{
		Hdr: dns.RR_Header{
			Name:   apiDomainFq,
			Ttl:    defaultDnsTtl,
			Class:  dns.ClassINET,
			Rrtype: dns.TypeA,
		},
		A: hubIp,
	}
	records = append(records, apiRecord)
	apiIntDomainFq := dns.Fqdn(t.apiIntDomain)
	apiIntRecord := &dns.A{
		Hdr: dns.RR_Header{
			Name:   apiIntDomainFq,
			Ttl:    defaultDnsTtl,
			Class:  dns.ClassINET,
			Rrtype: dns.TypeA,
		},
		A: hubIp,
	}
	records = append(records, apiIntRecord)

	// Return the records:
	result = records
	return
}

// currentDnsRecords gets the DNS records for the given domain from the DNS server.
//
// TODO: This uses a complete zone transfer and then discards anything that isn't a sub-domain of the domain that we
// are interested in. This is expensive, should be replaced by something better.
func (t *task) currentDnsRecords(ctx context.Context) (records []dns.RR, err error) {
	// Start the zone transfer:
	baseDomainFq := dns.Fqdn(t.baseDomain)
	t.logger.DebugContext(
		ctx,
		"Attempting zone transfer",
		slog.String("server", t.parent.dnsServer),
		slog.String("zone", t.parent.dnsZoneFq),
		slog.String("domain", baseDomainFq),
	)
	msg := &dns.Msg{}
	msg.SetAxfr(t.parent.dnsZoneFq)
	if t.parent.dnsKeyName != "" && t.parent.dnsKeySecret != "" {
		msg.SetTsig(t.parent.dnsKeyNameFq, dns.HmacSHA256, 300, time.Now().Unix())
	}
	transfer := &dns.Transfer{
		TsigSecret: t.parent.dnsClient.TsigSecret,
	}
	channel, err := transfer.In(msg, t.parent.dnsServer)
	if err != nil {
		err = fmt.Errorf("failed to initiate zone transfer for domain '%s': %w", baseDomainFq, err)
		return
	}

	// Process the incoming records, discarding anything that is not a sub-domain of the domain that we are
	// interested in:
	for envelope := range channel {
		if envelope.Error != nil {
			err = fmt.Errorf("zone transfer error for domain '%s': %w", baseDomainFq, envelope.Error)
			return
		}
		t.logger.DebugContext(
			ctx,
			"Received zone transfer envelope",
			slog.Any("records", envelope.RR),
		)
		for _, record := range envelope.RR {
			name := record.Header().Name
			if strings.HasSuffix(name, baseDomainFq) {
				records = append(records, record)
			}
		}
	}

	return
}

func (t *task) diffDnsRecords(ctx context.Context, current, desired []dns.RR) (add, remove []dns.RR, err error) {
	// Calculate the keys to compare the records:
	currentSet := map[string]dns.RR{}
	for _, record := range current {
		currentSet[t.dnsRecordDiffKey(record)] = record
	}
	desiredSet := map[string]dns.RR{}
	for _, record := range desired {
		desiredSet[t.dnsRecordDiffKey(record)] = record
	}

	// Find records that need to be added:
	for key, record := range desiredSet {
		_, ok := currentSet[key]
		if !ok {
			add = append(add, record)
		}
	}

	// Find records that need to be removed:
	for key, record := range currentSet {
		_, ok := desiredSet[key]
		if !ok {
			remove = append(remove, record)
		}
	}

	// Return the results:
	t.logger.DebugContext(
		ctx,
		"Calculated DNS diff",
		slog.Any("add", add),
		slog.Any("remove", remove),
	)

	return
}

// dnsRecordDiffKey returns a normalized string key for a DNS record, ignoring TTL, to use it as the key for
// calculating the difference between two lists of records.
func (t *task) dnsRecordDiffKey(record dns.RR) string {
	// We make a copy and set the TTL to 0 to ignore it in comparisons.
	copy := dns.Copy(record)
	copy.Header().Ttl = 0
	return copy.String()
}

func (t *task) syncFromHostedCluster(ctx context.Context) error {
	hostedClusterKey := clnt.ObjectKeyFromObject(t.hostedCluster)
	err := t.hubClient.Get(ctx, hostedClusterKey, t.hostedCluster)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get hosted cluster: %w", err)
	}
	err = t.syncState()
	if err != nil {
		return err
	}
	if t.cluster.GetStatus().GetState() == privatev1.ClusterState_CLUSTER_STATE_READY {
		err = t.syncApiUrl()
		if err != nil {
			return err
		}
		err = t.syncConsoleUrl()
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *task) syncState() error {
	var availableStatus corev1.ConditionStatus
	err := t.parent.jqTool.Evaluate(
		`.status.conditions[] | select(.type == "Available") | .status`,
		t.hostedCluster.Object,
		&availableStatus,
	)
	if err != nil {
		return err
	}
	var state privatev1.ClusterState
	if availableStatus == corev1.ConditionTrue {
		state = privatev1.ClusterState_CLUSTER_STATE_READY
		t.cluster.GetStatus().SetState(state)
	}
	return nil
}

func (t *task) syncApiUrl() error {
	type controlPlaneEndpointData struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	var controlPlaneEndpoint controlPlaneEndpointData
	err := t.parent.jqTool.Evaluate(
		`.status.controlPlaneEndpoint | {
			host: .host,
			port: .port,
		}`,
		t.hostedCluster.Object,
		&controlPlaneEndpoint,
	)
	if err != nil {
		return err
	}
	if controlPlaneEndpoint.Host == "" || controlPlaneEndpoint.Port == 0 {
		return nil
	}
	url := fmt.Sprintf(
		"https://%s:%d",
		controlPlaneEndpoint.Host, controlPlaneEndpoint.Port,
	)
	t.cluster.GetStatus().SetApiUrl(url)
	return nil
}

func (t *task) syncConsoleUrl() error {
	var dnsBaseDomain string
	err := t.parent.jqTool.Evaluate(
		`.spec.dns.baseDomain`,
		t.hostedCluster.Object,
		&dnsBaseDomain,
	)
	if err != nil {
		return err
	}
	if dnsBaseDomain == "" {
		return nil
	}
	url := fmt.Sprintf(
		"https://console-openshift-console.apps.%s.%s",
		t.alias, dnsBaseDomain,
	)
	t.cluster.GetStatus().SetConsoleUrl(url)
	return nil
}

func (t *task) setDefaults() {
	status := t.cluster.GetStatus()
	if status == nil {
		status = privatev1.ClusterStatus_builder{}.Build()
		t.cluster.SetStatus(status)
	}
	if status.GetState() == privatev1.ClusterState_CLUSTER_STATE_UNSPECIFIED {
		status.SetState(privatev1.ClusterState_CLUSTER_STATE_PROGRESSING)
	}
	for value := range privatev1.ClusterConditionType_name {
		if value != 0 {
			t.setConditionDefaults(privatev1.ClusterConditionType(value))
		}
	}
	nodeSets := status.GetNodeSets()
	if nodeSets == nil {
		nodeSets = map[string]*privatev1.ClusterNodeSet{}
		status.SetNodeSets(nodeSets)
	}
	for nodeSetName, nodeSet := range t.cluster.GetSpec().GetNodeSets() {
		statusNodeSet := nodeSets[nodeSetName]
		if statusNodeSet == nil {
			statusNodeSet = privatev1.ClusterNodeSet_builder{
				HostClass: nodeSet.GetHostClass(),
			}.Build()
			nodeSets[nodeSetName] = statusNodeSet
		}
	}
}

func (t *task) setConditionDefaults(value privatev1.ClusterConditionType) {
	exists := false
	for _, current := range t.cluster.GetStatus().GetConditions() {
		if current.GetType() == value {
			exists = true
			break
		}
	}
	if !exists {
		conditions := t.cluster.GetStatus().GetConditions()
		conditions = append(conditions, privatev1.ClusterCondition_builder{
			Type:   value,
			Status: sharedv1.ConditionStatus_CONDITION_STATUS_FALSE,
		}.Build())
		t.cluster.GetStatus().SetConditions(conditions)
	}
}

func (t *task) delete(ctx context.Context) error {
	// Select the hub where the cluster was created so we can delete the resources:
	err := t.selectHub(ctx)
	if err != nil {
		t.logger.ErrorContext(
			ctx,
			"Failed to select hub for deletion",
			slog.Any("error", err),
		)
		return err
	}

	// Try fetch the hosted cluster:
	clusterMeta := t.cluster.GetMetadata()
	t.hostedCluster = &unstructured.Unstructured{}
	t.hostedCluster.SetGroupVersionKind(gvks.HostedCluster)
	t.hostedCluster.SetNamespace(t.hub.GetNamespace())
	t.hostedCluster.SetName(clusterMeta.GetName())
	hostedClusterKey := clnt.ObjectKeyFromObject(t.hostedCluster)
	err = t.hubClient.Get(ctx, hostedClusterKey, t.hostedCluster)
	if clnt.IgnoreNotFound(err) != nil {
		return err
	}

	// If the hosted cluster doesn't exist then we can assume that HyperShift already completed its cleanup
	// so we can remove our finalizer.
	if apierrors.IsNotFound(err) {
		t.logger.InfoContext(
			ctx,
			"Hosted cluster not found, deleting finalizer",
			slog.Any("error", err),
		)
		t.removeFinalizer()
		return nil
	}

	// If the hosted cluster exists and it hasn't the deletion timestamp, then we need to delete it:
	hostedClusterDeleted := t.hostedCluster.GetDeletionTimestamp() != nil
	if !hostedClusterDeleted {
		err = t.hubClient.Delete(ctx, t.hostedCluster)
		if err != nil {
			return err
		}
		t.logger.InfoContext(
			ctx,
			"Deleted hosted cluster",
		)
		return nil
	}

	// If the hosted cluster exists, it has been deleted and it doesn't have the HyperShift finalizer, then we can
	// also assume that Hypershift already completed its cleanup, so we can remove our finalizer from both the
	// cluster and the hosted cluster.
	if !controllerutil.ContainsFinalizer(t.hostedCluster, "hypershift.openshift.io/finalizer") {
		t.logger.InfoContext(
			ctx,
			"Hosted cluster has been deleted and doesn't have the HyperShift finalizer, removing finalizers",
			slog.Any("error", err),
		)
		t.removeFinalizer()
		hostedClusterPatch := clnt.MergeFrom(t.hostedCluster.DeepCopy())
		controllerutil.RemoveFinalizer(t.hostedCluster, finalizers.Controller)
		return t.hubClient.Patch(ctx, t.hostedCluster, hostedClusterPatch)
	}

	return nil
}

func (t *task) scaleNodeSets(ctx context.Context) error {
	for nodeSetName, nodeSetSpec := range t.cluster.GetSpec().GetNodeSets() {
		nodeSetStatus := t.cluster.GetStatus().GetNodeSets()[nodeSetName]
		err := t.scaleNodeSet(ctx, nodeSetName, nodeSetSpec, nodeSetStatus)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *task) scaleNodeSet(ctx context.Context, nodeSetName string, nodeSetSpec *privatev1.ClusterNodeSet,
	nodeSetStatus *privatev1.ClusterNodeSet) error {
	// Get the desired and current number of hosts:
	desiredSize := nodeSetSpec.GetSize()
	currentSize := nodeSetStatus.GetSize()
	deltaSize := desiredSize - currentSize
	t.logger.DebugContext(
		ctx,
		"Node set scaling delta",
		slog.String("set", nodeSetName),
		slog.Int("desired", int(desiredSize)),
		slog.Int("current", int(currentSize)),
		slog.Int("delta", int(deltaSize)),
	)

	// Scale the node set up or down as needed:
	var assignedHostIds, unassignedHostIds []string
	switch {
	case deltaSize > 0:
		var err error
		assignedHostIds, err = t.scaleNodeSetUp(ctx, nodeSetName, nodeSetSpec, nodeSetStatus)
		if err != nil {
			return err
		}
	case deltaSize < 0:
		var err error
		unassignedHostIds, err = t.scaleNodeSetDown(ctx, nodeSetName, nodeSetSpec, nodeSetStatus)
		if err != nil {
			return err
		}
	}

	// Update the status of the node set:
	hostIds := nodeSetStatus.GetHosts()
	for _, hostId := range assignedHostIds {
		if !slices.Contains(hostIds, hostId) {
			hostIds = append(hostIds, hostId)
		}
	}
	for _, hostId := range unassignedHostIds {
		hostIds = slices.DeleteFunc(hostIds, func(item string) bool {
			return item == hostId
		})
	}
	sort.Strings(hostIds)
	nodeSetStatus.SetHosts(hostIds)
	nodeSetStatus.SetSize(int32(len(hostIds)))

	return nil
}

// scaleNodeSetUp scales up a node set by assigning a random selection of hosts, and returns the identifiers of the
// assigned hosts.
func (t *task) scaleNodeSetUp(ctx context.Context, nodeSetName string, nodeSetSpec *privatev1.ClusterNodeSet,
	nodeSetStatus *privatev1.ClusterNodeSet) (result []string, err error) {
	// Add the node set name to the logger:
	logger := t.logger.With(
		slog.String("set", nodeSetName),
	)

	// Find all the available hosts with the matching host class:
	hostDelta := nodeSetSpec.GetSize() - nodeSetStatus.GetSize()
	hostFilter := fmt.Sprintf(
		"!has(this.metadata.deletion_timestamp) && "+
			"this.spec.class == '%s' && "+
			"!has(this.status.cluster)",
		nodeSetSpec.GetHostClass(),
	)
	hostListResponse, err := t.parent.hostsClient.List(ctx, privatev1.HostsListRequest_builder{
		Filter: &hostFilter,
		Limit:  proto.Int32(hostDelta),
	}.Build())
	if err != nil {
		return
	}
	hostDelta = min(hostDelta, hostListResponse.GetSize())
	hostList := hostListResponse.GetItems()
	hostIds := make([]string, 0, hostDelta)
	for _, host := range hostList {
		hostIds = append(hostIds, host.GetId())
	}
	sort.Strings(hostIds)
	logger.DebugContext(
		ctx,
		"Selected hosts to assign",
		slog.Int("count", int(hostDelta)),
		slog.Any("hosts", hostIds),
	)

	// Update the hosts to reflect that they are assigned to the cluster:
	for _, hostId := range hostIds {
		_, err = t.parent.hostsClient.Update(ctx, privatev1.HostsUpdateRequest_builder{
			Object: privatev1.Host_builder{
				Id: hostId,
				Status: privatev1.HostStatus_builder{
					Cluster: t.cluster.GetId(),
				}.Build(),
			}.Build(),
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{
					"status.cluster",
				},
			},
		}.Build())
		if err != nil {
			return err
		}
		logger.DebugContext(
			ctx,
			"Assigned host",
			slog.String("host", hostId),
		)
	}

	// Return the identifiers of the assigned hosts:
	result = hostIds
	return
}

// scaleNodeSetDown scales down a node set by unassigning a random selection of hosts, and returns the identifiers of
// the unassigned hosts.
func (t *task) scaleNodeSetDown(ctx context.Context, nodeSetName string, nodeSetSpec *privatev1.ClusterNodeSet,
	nodeSetStatus *privatev1.ClusterNodeSet) (result []string, err error) {
	// Add the node set name to the logger:
	logger := t.logger.With(
		slog.String("set", nodeSetName),
	)

	// Randomize the list of hosts of the cluster, so that we can randomly select the hosts to unassign.
	hostIds := slices.Clone(nodeSetStatus.GetHosts())
	rand.Shuffle(len(hostIds), func(i, j int) {
		hostIds[i], hostIds[j] = hostIds[j], hostIds[i]
	})

	// Select the first hosts from the list:
	hostDelta := nodeSetStatus.GetSize() - nodeSetSpec.GetSize()
	hostDelta = min(hostDelta, int32(len(hostIds)))
	hostIds = hostIds[:hostDelta]
	sort.Strings(hostIds)
	logger.DebugContext(
		ctx,
		"Selected hosts to unassign",
		slog.String("set", nodeSetName),
		slog.Int("count", int(hostDelta)),
		slog.Any("hosts", hostIds),
	)

	// Remove the reference to the cluster from the selected hosts:
	for _, hostId := range hostIds {
		_, err = t.parent.hostsClient.Update(ctx, privatev1.HostsUpdateRequest_builder{
			Object: privatev1.Host_builder{
				Id: hostId,
				Status: privatev1.HostStatus_builder{
					Cluster: "",
				}.Build(),
			}.Build(),
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{
					"status.cluster",
				},
			},
		}.Build())
		if err != nil {
			return
		}
		logger.DebugContext(
			ctx,
			"Unassigned host",
			slog.String("host", hostId),
		)
	}

	// Return the identifiers of the unassigned hosts:
	result = hostIds
	return
}

func (t *task) selectHub(ctx context.Context) error {
	hub := t.cluster.GetStatus().GetHub()
	if hub == "" {
		response, err := t.parent.hubsClient.List(ctx, privatev1.HubsListRequest_builder{}.Build())
		if err != nil {
			return err
		}
		if len(response.Items) == 0 {
			return errors.New("there are no hubs")
		}
		hub = response.Items[rand.IntN(len(response.Items))].GetId()
		t.logger.DebugContext(
			ctx,
			"Selected hub",
			slog.String("name", t.hub.GetMetadata().GetName()),
			slog.String("id", t.hub.GetId()),
		)
	}
	entry, err := t.parent.hubCache.Get(ctx, hub)
	if err != nil {
		return err
	}
	t.hub = entry.Hub
	t.hubClient = entry.Client
	return nil
}

func (t *task) selectPort(ctx context.Context) error {
	// If the port is already set, do nothing:
	port := t.cluster.GetStatus().GetPort()
	if port != 0 {
		return nil
	}

	// Otherwise we need to find all the existing clusters and select a port that is not in use:
	ports := map[int32]struct{}{}
	clusters, err := t.parent.clustersClient.List(ctx, privatev1.ClustersListRequest_builder{}.Build())
	if err != nil {
		return err
	}
	for _, cluster := range clusters.Items {
		ports[cluster.GetStatus().GetPort()] = struct{}{}
	}
	for candidate := int32(30000); candidate < 32768; candidate++ {
		_, used := ports[candidate]
		if used {
			t.logger.DebugContext(
				ctx,
				"Candidate port in use",
				slog.Int("port", int(candidate)),
			)
			continue
		}
		t.logger.DebugContext(
			ctx,
			"Candidate port available",
			slog.Int("port", int(candidate)),
		)
		t.cluster.GetStatus().SetPort(candidate)
		return nil
	}

	// If we are here then we didn't find a port that is not in use:
	return errors.New("no available port found")
}

func (t *task) selectAlias(ctx context.Context) error {
	// If the alias is already set, do nothing:
	t.alias = t.cluster.GetStatus().GetAlias()
	if t.alias != "" {
		return nil
	}

	// Try to use the name of the cluster as the alias:
	t.alias = t.cluster.GetMetadata().GetName()
	if t.alias != "" {
		t.cluster.GetStatus().SetAlias(t.alias)
		return nil
	}

	// Try to use the identifier of the cluster as the alias:
	t.alias = t.cluster.GetId()
	t.cluster.GetStatus().SetAlias(t.alias)
	return nil
}

// updateCondition updates or creates a condition with the specified type, status, reason, and message.
func (t *task) updateCondition(conditionType privatev1.ClusterConditionType, status sharedv1.ConditionStatus,
	reason string, message string) {
	conditions := t.cluster.GetStatus().GetConditions()
	updated := false
	for i, condition := range conditions {
		if condition.GetType() == conditionType {
			conditions[i] = privatev1.ClusterCondition_builder{
				Type:    conditionType,
				Status:  status,
				Reason:  &reason,
				Message: &message,
			}.Build()
			updated = true
			break
		}
	}
	if !updated {
		conditions = append(conditions, privatev1.ClusterCondition_builder{
			Type:    conditionType,
			Status:  status,
			Reason:  &reason,
			Message: &message,
		}.Build())
	}
	t.cluster.GetStatus().SetConditions(conditions)
}

func (t *task) hasFinalizer() bool {
	list := t.cluster.GetMetadata().GetFinalizers()
	return slices.Contains(list, finalizers.Controller)
}

func (t *task) addFinalizer() {
	if t.hasFinalizer() {
		return
	}
	list := t.cluster.GetMetadata().GetFinalizers()
	list = append(list, finalizers.Controller)
	t.cluster.GetMetadata().SetFinalizers(list)
}

func (t *task) removeFinalizer() {
	if !t.hasFinalizer() {
		return
	}
	list := t.cluster.GetMetadata().GetFinalizers()
	list = slices.DeleteFunc(list, func(item string) bool {
		return item == finalizers.Controller
	})
	t.cluster.GetMetadata().SetFinalizers(list)
}

// defaultDnsTtl is the default time to live for DNS records.
const defaultDnsTtl = 10

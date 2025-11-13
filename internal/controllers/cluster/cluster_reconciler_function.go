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
	"sort"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clnt "sigs.k8s.io/controller-runtime/pkg/client"

	privatev1 "github.com/innabox/fulfillment-service/internal/api/private/v1"
	sharedv1 "github.com/innabox/fulfillment-service/internal/api/shared/v1"
	"github.com/innabox/fulfillment-service/internal/controllers"
	"github.com/innabox/fulfillment-service/internal/kubernetes/gvks"
	"github.com/innabox/fulfillment-service/internal/kubernetes/labels"
	"github.com/innabox/fulfillment-service/internal/utils"
)

// objectPrefix is the prefix that will be used in the `generateName` field of the resources created in the hub.
const objectPrefix = "order-"

// FunctionBuilder contains the data and logic needed to build a function that reconciles clustes.
type FunctionBuilder struct {
	logger     *slog.Logger
	connection *grpc.ClientConn
	hubCache   *controllers.HubCache
}

type function struct {
	logger            *slog.Logger
	hubCache          *controllers.HubCache
	clustersClient    privatev1.ClustersClient
	hubsClient        privatev1.HubsClient
	hostClassesClient privatev1.HostClassesClient
	hostsClient       privatev1.HostsClient
}

type task struct {
	r            *function
	cluster      *privatev1.Cluster
	hubId        string
	hubNamespace string
	hubClient    clnt.Client
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

	// Create and populate the object:
	object := &function{
		logger:            b.logger,
		clustersClient:    privatev1.NewClustersClient(b.connection),
		hubsClient:        privatev1.NewHubsClient(b.connection),
		hostsClient:       privatev1.NewHostsClient(b.connection),
		hostClassesClient: privatev1.NewHostClassesClient(b.connection),
		hubCache:          b.hubCache,
	}
	result = object.run
	return
}

func (r *function) run(ctx context.Context, cluster *privatev1.Cluster) error {
	oldCluster := proto.Clone(cluster).(*privatev1.Cluster)
	t := task{
		r:       r,
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
		_, err = r.clustersClient.Update(ctx, privatev1.ClustersUpdateRequest_builder{
			Object: cluster,
		}.Build())
	}
	return err
}

func (t *task) update(ctx context.Context) error {
	// Set the default values:
	t.setDefaults()

	// Do nothing if the order isn't progressing:
	if t.cluster.GetStatus().GetState() != privatev1.ClusterState_CLUSTER_STATE_PROGRESSING {
		return nil
	}

	// Check if hosts need to be assigned or unassigned:
	err := t.checkHostAssignment(ctx)
	if err != nil {
		return err
	}

	// TODO: Skip the rest of the logic for now, as we just want to test the hosts assignment.
	if true {
		return nil
	}

	// Select the hub, and if no hubs are available, update the condition to inform the user that creation is
	// pending. Note that we don't want to disclose the existence of hubs to the user, as that is a internal
	// implementation detail, so keep the message generic enough to not reveal that information.
	err = t.selectHub(ctx)
	if err != nil {
		t.r.logger.ErrorContext(
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
	t.cluster.GetStatus().SetHub(t.hubId)

	// Get the K8S object:
	object, err := t.getKubeObject(ctx)
	if err != nil {
		return err
	}

	// Prepare the changes to the spec:
	nodeRequests := t.prepareNodeRequests()
	templateParameters, err := utils.ConvertTemplateParametersToJSON(t.cluster.GetSpec().GetTemplateParameters())
	if err != nil {
		return err
	}
	spec := map[string]any{
		"templateID":         t.cluster.GetSpec().GetTemplate(),
		"templateParameters": templateParameters,
		"nodeRequests":       nodeRequests,
	}

	// Create or update the Kubernetes object:
	if object == nil {
		object := &unstructured.Unstructured{}
		object.SetGroupVersionKind(gvks.ClusterOrder)
		object.SetNamespace(t.hubNamespace)
		object.SetGenerateName(objectPrefix)
		object.SetLabels(map[string]string{
			labels.ClusterOrderUuid: t.cluster.GetId(),
		})
		err = unstructured.SetNestedField(object.Object, spec, "spec")
		if err != nil {
			return err
		}
		err = t.hubClient.Create(ctx, object)
		if err != nil {
			return err
		}
		t.r.logger.DebugContext(
			ctx,
			"Created cluster order",
			slog.String("namespace", object.GetNamespace()),
			slog.String("name", object.GetName()),
		)
	} else {
		update := object.DeepCopy()
		err = unstructured.SetNestedField(update.Object, spec, "spec")
		if err != nil {
			return err
		}
		err = t.hubClient.Patch(ctx, update, clnt.MergeFrom(object))
		if err != nil {
			return err
		}
		t.r.logger.DebugContext(
			ctx,
			"Updated cluster order",
			slog.String("namespace", object.GetNamespace()),
			slog.String("name", object.GetName()),
		)
	}

	return err
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

func (t *task) prepareNodeRequests() any {
	var nodeRequests []any
	for _, nodeSet := range t.cluster.GetSpec().GetNodeSets() {
		nodeRequest := t.prepareNodeRequest(nodeSet)
		nodeRequests = append(nodeRequests, nodeRequest)
	}
	return nodeRequests
}

func (t *task) prepareNodeRequest(nodeSet *privatev1.ClusterNodeSet) any {
	return map[string]any{
		"resourceClass": nodeSet.GetHostClass(),
		"numberOfNodes": int64(nodeSet.GetSize()),
	}
}

func (t *task) delete(ctx context.Context) error {
	// Unassign all the hosts from the cluster:
	for _, nodeSet := range t.cluster.GetSpec().GetNodeSets() {
		err := t.unassignHosts(ctx, nodeSet.GetHostClass(), int(nodeSet.GetSize()))
		if err != nil {
			return err
		}
	}

	// TODO: Skip the rest of the logic for now, as we just want to test the hosts assignment.
	if true {
		return nil
	}

	// Do nothing if we don't know the hub yet:
	t.hubId = t.cluster.GetStatus().GetHub()
	if t.hubId == "" {
		return nil
	}
	err := t.getHub(ctx)
	if err != nil {
		return err
	}

	// Delete the K8S object:
	object, err := t.getKubeObject(ctx)
	if err != nil {
		return err
	}
	if object == nil {
		t.r.logger.DebugContext(
			ctx,
			"Cluster order doesn't exist",
			slog.String("id", t.cluster.GetId()),
		)
		return nil
	}
	err = t.hubClient.Delete(ctx, object)
	if err != nil {
		return err
	}
	t.r.logger.DebugContext(
		ctx,
		"Deleted cluster order",
		slog.String("namespace", object.GetNamespace()),
		slog.String("name", object.GetName()),
	)

	return err
}

func (t *task) checkHostAssignment(ctx context.Context) error {
	for nodeSetName, nodeSet := range t.cluster.GetSpec().GetNodeSets() {
		err := t.checkNodeSetHostAssignment(ctx, nodeSetName, nodeSet)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *task) checkNodeSetHostAssignment(ctx context.Context, nodeSetName string, nodeSet *privatev1.ClusterNodeSet) error {
	// The user may have specified the host class by identifier or by name, so we need to look it up to make sure
	// that we have the identifier, as that is what is used internally to avoid ambiguity.
	desiredClass, err := t.lookupHostClass(ctx, nodeSet.GetHostClass())
	if err != nil {
		return err
	}

	// Get the desired number of hosts for this node set:
	desiredCount := int(nodeSet.GetSize())

	// Find the number of hots already assigned to this cluster. Note tha twe set the limit to zero don't care
	// about the items, we only want the count.
	assignedFilter := fmt.Sprintf(
		"!has(this.metadata.deletion_timestamp) && "+
			"this.spec.class == '%s' && "+
			"has(this.status.cluster) && this.status.cluster == '%s'",
		desiredClass, t.cluster.GetId(),
	)
	assignedResponse, err := t.r.hostsClient.List(ctx, privatev1.HostsListRequest_builder{
		Filter: &assignedFilter,
		Limit:  proto.Int32(0),
	}.Build())
	if err != nil {
		return err
	}
	assignedCount := int(assignedResponse.GetTotal())

	// Calculate the delta, which can be zero, positive or negative:
	deltaCount := desiredCount - assignedCount
	t.r.logger.DebugContext(
		ctx,
		"Host assignment delta",
		slog.String("node_set", nodeSetName),
		slog.Int("desired", desiredCount),
		slog.Int("assigned", assignedCount),
		slog.Int("delta", deltaCount),
	)

	// If the delta is positive, we need to assign new hosts, if it is negative, we need to unassign hosts.
	switch {
	case deltaCount > 0:
		err = t.assignHosts(ctx, desiredClass, deltaCount)
		if err != nil {
			return err
		}
	case deltaCount < 0:
		err = t.unassignHosts(ctx, desiredClass, -deltaCount)
		if err != nil {
			return err
		}
	}

	// If the delta wasn't zero, then check again the hosts assigned to the cluster, as the above code should have
	// changed it. Note that this time we do want' the complete list of hosts, so that we can get the identifiers
	// and add them to the details of the node set.
	assignedResponse, err = t.r.hostsClient.List(ctx, privatev1.HostsListRequest_builder{
		Filter: &assignedFilter,
		Limit:  proto.Int32(int32(deltaCount)),
	}.Build())
	if err != nil {
		return err
	}
	assignedHosts := assignedResponse.GetItems()
	assignedCount = len(assignedHosts)
	assignedIds := make([]string, assignedCount)
	for i, assignedHost := range assignedHosts {
		assignedIds[i] = assignedHost.GetId()
	}
	sort.Strings(assignedIds)

	// Update the status of the cluster to reflect the new number and identifiers of the hosts actually assigned to
	// the cluster. Note that we don't need to check or create the node set in the status, becauase we always check
	// and set it in the method that sets the defaults.
	statusNodeSet := t.cluster.GetStatus().GetNodeSets()[nodeSetName]
	statusNodeSet.SetSize(int32(assignedCount))
	statusNodeSet.SetHosts(assignedIds)

	return nil
}

func (t *task) assignHosts(ctx context.Context, hostClass string, hostCount int) error {
	// Find available hosts with matching class:
	availableFilter := fmt.Sprintf(
		"!has(this.metadata.deletion_timestamp) && "+
			"this.spec.class == '%s' && "+
			"!has(this.status.cluster)",
		hostClass,
	)
	availableResponse, err := t.r.hostsClient.List(ctx, privatev1.HostsListRequest_builder{
		Filter: &availableFilter,
		Limit:  proto.Int32(int32(hostCount)),
	}.Build())
	if err != nil {
		return err
	}
	availableHosts := availableResponse.GetItems()

	// Assign the hosts:
	for _, host := range availableHosts {
		host.GetStatus().SetCluster(t.cluster.GetId())
		_, err = t.r.hostsClient.Update(ctx, privatev1.HostsUpdateRequest_builder{
			Object: host,
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{
					"status.cluster",
				},
			},
		}.Build())
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *task) unassignHosts(ctx context.Context, hostClass string, hostCount int) error {
	// Find the hosts that are assigned to this cluster and have the matching class:
	assignedFilter := fmt.Sprintf(
		"!has(this.metadata.deletion_timestamp) && "+
			"this.spec.class == '%s' && "+
			"has(this.status.cluster) && this.status.cluster == '%s'",
		hostClass, t.cluster.GetId(),
	)
	assignedResponse, err := t.r.hostsClient.List(ctx, privatev1.HostsListRequest_builder{
		Filter: &assignedFilter,
		Limit:  proto.Int32(int32(hostCount)),
	}.Build())
	if err != nil {
		return err
	}
	assignedHosts := assignedResponse.GetItems()

	// Unassign the hosts:
	for _, assignedHost := range assignedHosts {
		assignedHost.GetStatus().SetCluster("")
		_, err = t.r.hostsClient.Update(ctx, privatev1.HostsUpdateRequest_builder{
			Object: assignedHost,
			UpdateMask: &fieldmaskpb.FieldMask{
				Paths: []string{
					"status.cluster",
				},
			},
		}.Build())
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *task) lookupHostClass(ctx context.Context, key string) (result string, err error) {
	filter := fmt.Sprintf("this.id == %[1]s || this.metadata.name == %[1]s", strconv.Quote(key))
	response, err := t.r.hostClassesClient.List(ctx, privatev1.HostClassesListRequest_builder{
		Filter: &filter,
		Limit:  proto.Int32(1),
	}.Build())
	if err != nil {
		return
	}
	total := response.GetTotal()
	switch {
	case total == 0:
		err = fmt.Errorf("there is no host class with identifier or name '%s'", key)
	case total == 1:
		result = response.GetItems()[0].GetId()
	default:
		err = fmt.Errorf("there are multiple host classes with identifier or name '%s'", key)
	}
	return
}

func (t *task) selectHub(ctx context.Context) error {
	t.hubId = t.cluster.GetStatus().GetHub()
	if t.hubId == "" {
		response, err := t.r.hubsClient.List(ctx, privatev1.HubsListRequest_builder{}.Build())
		if err != nil {
			return err
		}
		if len(response.Items) == 0 {
			return errors.New("there are no hubs")
		}
		t.hubId = response.Items[rand.IntN(len(response.Items))].GetId()
	}
	t.r.logger.DebugContext(
		ctx,
		"Selected hub",
		slog.String("id", t.hubId),
	)
	hubEntry, err := t.r.hubCache.Get(ctx, t.hubId)
	if err != nil {
		return err
	}
	t.hubNamespace = hubEntry.Namespace
	t.hubClient = hubEntry.Client
	return nil
}

func (t *task) getHub(ctx context.Context) error {
	t.hubId = t.cluster.GetStatus().GetHub()
	hubEntry, err := t.r.hubCache.Get(ctx, t.hubId)
	if err != nil {
		return err
	}
	t.hubNamespace = hubEntry.Namespace
	t.hubClient = hubEntry.Client
	return nil
}

func (t *task) getKubeObject(ctx context.Context) (result *unstructured.Unstructured, err error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvks.ClusterOrderList)
	err = t.hubClient.List(
		ctx, list,
		clnt.InNamespace(t.hubNamespace),
		clnt.MatchingLabels{
			labels.ClusterOrderUuid: t.cluster.GetId(),
		},
	)
	if err != nil {
		return
	}
	items := list.Items
	count := len(items)
	if count > 1 {
		err = fmt.Errorf(
			"expected at most one cluster order with identifer '%s' but found %d",
			t.cluster.GetId(), count,
		)
		return
	}
	if count > 0 {
		result = &items[0]
	}
	return
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

/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package bcm

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/adrg/frontmatter"
	"github.com/gogo/protobuf/proto"
	"github.com/stmcginnis/gofish"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	privatev1 "github.com/innabox/fulfillment-service/internal/api/private/v1"
	"github.com/innabox/fulfillment-service/internal/work"
)

// SynchronizerBuilder is used to build synchronizers using the builder pattern.
type SynchronizerBuilder struct {
	logger    *slog.Logger
	bcmUrl    string
	bcmClient *Client
	grpcConn  *grpc.ClientConn
	interval  time.Duration
}

// Synchronizer periodically synchronizes inventory from BCM to the fulfillment service.
type Synchronizer struct {
	logger            *slog.Logger
	bcmUrl            string
	bcmClient         *Client
	hostsClient       privatev1.HostsClient
	hostClassesClient privatev1.HostClassesClient
	interval          time.Duration
}

// synchronizerTask contains the data and logic needed for an individual synchronization cycle.
type synchronizerTask struct {
	parent            *Synchronizer
	logger            *slog.Logger
	bcmClient         *Client
	hostsClient       privatev1.HostsClient
	hostClassesClient privatev1.HostClassesClient
	racksByUuid       map[string]*Rack
	networksByUuid    map[string]*Network
	categoriesByUuid  map[string]*Category
	devicesByUuid     map[string]*Device
}

// NewSynchronizer creates a new synchronizer builder.
func NewSynchronizer() *SynchronizerBuilder {
	return &SynchronizerBuilder{}
}

// SetLogger sets the logger for the synchronizer.
func (b *SynchronizerBuilder) SetLogger(value *slog.Logger) *SynchronizerBuilder {
	b.logger = value
	return b
}

// SetBcmUrl sets the BCM URL.
func (b *SynchronizerBuilder) SetBcmUrl(value string) *SynchronizerBuilder {
	b.bcmUrl = value
	return b
}

// SetBcmClient sets the BCM client.
func (b *SynchronizerBuilder) SetBcmClient(value *Client) *SynchronizerBuilder {
	b.bcmClient = value
	return b
}

// SetGrpcClient sets the gRPC connection to the fulfillment service.
func (b *SynchronizerBuilder) SetGrpcClient(value *grpc.ClientConn) *SynchronizerBuilder {
	b.grpcConn = value
	return b
}

// SetInterval sets the synchronization interval.
func (b *SynchronizerBuilder) SetInterval(value time.Duration) *SynchronizerBuilder {
	b.interval = value
	return b
}

// Build creates the synchronizer.
func (b *SynchronizerBuilder) Build() (result *Synchronizer, err error) {
	// Check required parameters:
	if b.logger == nil {
		err = fmt.Errorf("logger is mandatory")
		return
	}
	if b.bcmClient == nil {
		err = fmt.Errorf("BCM client is mandatory")
		return
	}
	if b.grpcConn == nil {
		err = fmt.Errorf("gRPC client is mandatory")
		return
	}

	// Set defaults:
	interval := b.interval
	if interval == 0 {
		interval = 10 * time.Second
	}

	// Create and populate the object:
	result = &Synchronizer{
		logger:            b.logger,
		bcmUrl:            b.bcmUrl,
		bcmClient:         b.bcmClient,
		hostsClient:       privatev1.NewHostsClient(b.grpcConn),
		hostClassesClient: privatev1.NewHostClassesClient(b.grpcConn),
		interval:          interval,
	}
	return
}

// Run starts the synchronization loop.
func (s *Synchronizer) Run(ctx context.Context) error {
	s.logger.InfoContext(
		ctx,
		"Starting inventory synchronization loop",
		slog.Duration("interval", s.interval),
	)
	loop, err := work.NewLoop().
		SetLogger(s.logger).
		SetName("synchronize").
		SetInterval(s.interval).
		SetWorkFunc(s.sync).
		Build()
	if err != nil {
		return fmt.Errorf("failed to create synchronization loop: %w", err)
	}
	return loop.Run(ctx)
}

func (s *Synchronizer) sync(ctx context.Context) error {
	task := &synchronizerTask{
		parent:            s,
		logger:            s.logger,
		bcmClient:         s.bcmClient,
		hostsClient:       s.hostsClient,
		hostClassesClient: s.hostClassesClient,
		racksByUuid:       map[string]*Rack{},
		networksByUuid:    map[string]*Network{},
		categoriesByUuid:  map[string]*Category{},
		devicesByUuid:     map[string]*Device{},
	}
	return task.run(ctx)
}

func (t *synchronizerTask) run(ctx context.Context) error {
	// Take note of the start time:
	start := time.Now()
	t.logger.InfoContext(
		ctx,
		"Starting synchronization cycle",
	)

	// Load networks, racks, categories, and devices:
	t.loadCategories(ctx)
	t.loadDevices(ctx)
	t.loadNetworks(ctx)
	t.loadRacks(ctx)

	// Synchronize categories as host classes:
	err := t.syncCategories(ctx)
	if err != nil {
		t.logger.WarnContext(
			ctx,
			"Failed to synchronize categories",
			slog.Any("error", err),
		)
	}

	// Synchronize devices as hosts:
	err = t.syncDevices(ctx)
	if err != nil {
		t.logger.WarnContext(
			ctx,
			"Failed to synchronize devices",
			slog.Any("error", err),
		)
	}

	// Approve certificate requests:
	err = t.approveCertificateRequests(ctx)
	if err != nil {
		t.logger.WarnContext(
			ctx,
			"Failed to approve certificate requests",
			slog.Any("error", err),
		)
	}

	// Log the results:
	duration := time.Since(start)
	t.logger.InfoContext(
		ctx,
		"Synchronization cycle completed",
		slog.Int("categories", len(t.categoriesByUuid)),
		slog.Int("devices", len(t.devicesByUuid)),
		slog.Duration("duration", duration),
	)

	return nil
}

func (t *synchronizerTask) loadCategories(ctx context.Context) {
	categories, err := t.bcmClient.GetCategories(ctx)
	if err != nil {
		t.logger.WarnContext(
			ctx,
			"Failed to get categories from BCM",
			slog.Any("error", err),
		)
	}
	for _, category := range categories {
		t.categoriesByUuid[category.Uuid] = category
	}
	t.logger.InfoContext(
		ctx,
		"Retrieved categories from BCM",
		slog.Int("count", len(categories)),
	)
}

func (t *synchronizerTask) loadRacks(ctx context.Context) {
	racks, err := t.bcmClient.GetRacks(ctx)
	if err != nil {
		t.logger.WarnContext(
			ctx,
			"Failed to get racks from BCM",
			slog.Any("error", err),
		)
		return
	}
	for _, rack := range racks {
		t.racksByUuid[rack.Uuid] = rack
	}
	t.logger.InfoContext(
		ctx,
		"Retrieved racks from BCM",
		slog.Int("count", len(racks)),
	)
}

func (t *synchronizerTask) loadNetworks(ctx context.Context) {
	networks, err := t.bcmClient.GetNetworks(ctx)
	if err != nil {
		t.logger.WarnContext(
			ctx,
			"Failed to get networks from BCM",
			slog.Any("error", err),
		)
		return
	}
	for _, network := range networks {
		t.networksByUuid[network.Uuid] = network
	}
	t.logger.InfoContext(
		ctx,
		"Retrieved networks from BCM",
		slog.Int("count", len(networks)),
	)
}

func (t *synchronizerTask) loadDevices(ctx context.Context) {
	devices, err := t.bcmClient.GetDevices(ctx)
	if err != nil {
		t.logger.WarnContext(
			ctx,
			"Failed to get devices from BCM",
			slog.Any("error", err),
		)
		return
	}
	for _, device := range devices {
		t.devicesByUuid[device.Uuid] = device
	}
	t.logger.InfoContext(
		ctx,
		"Retrieved devices from BCM",
		slog.Int("count", len(devices)),
	)
}

func (t *synchronizerTask) syncDevices(ctx context.Context) error {
	for _, device := range t.devicesByUuid {
		if device.ChildType != "LiteNode" {
			continue
		}
		err := t.syncDevice(ctx, device)
		if err != nil {
			t.logger.WarnContext(
				ctx,
				"Failed to synchronize device",
				slog.String("hostname", device.Hostname),
				slog.Any("error", err),
			)
		}
	}
	return nil
}

func (t *synchronizerTask) syncDevice(ctx context.Context, device *Device) error {
	// Extract the title and description from the notes. Note that we don't have a place to store the host title
	// and description yet, so we just ignore them.
	type Matter struct {
		HostClass string `yaml:"host_class"`
	}
	var matter Matter
	_, _ = t.parseNotes(device.Notes, &matter)

	// Try to find a host that matches the name or identifier of the device. If we find one then we will use it,
	// otherwise we will create a new host with the same identifier than the device.
	hostFilter := fmt.Sprintf(
		"this.id == %s || this.metadata.name == %s",
		strconv.Quote(device.Uuid),
		strconv.Quote(device.Hostname),
	)
	hostsResponse, err := t.hostsClient.List(ctx, privatev1.HostsListRequest_builder{
		Filter: proto.String(hostFilter),
		Limit:  proto.Int32(1),
	}.Build())
	if err != nil {
		return fmt.Errorf("failed to find hosts: %w", err)
	}
	var host *privatev1.Host
	if hostsResponse.GetSize() == 1 {
		host = hostsResponse.GetItems()[0]
	} else {
		host = privatev1.Host_builder{
			Id: device.Uuid,
		}.Build()
	}
	hostMeta := host.GetMetadata()
	if hostMeta == nil {
		hostMeta = &privatev1.Metadata{}
		host.SetMetadata(hostMeta)
	}
	hostSpec := host.GetSpec()
	if hostSpec == nil {
		hostSpec = &privatev1.HostSpec{}
		host.SetSpec(hostSpec)
	}

	// Set the host name:
	hostMeta.SetName(device.Hostname)

	// If the matter species a host class then use it, otherwise use the device category:
	hostClassId := matter.HostClass
	if hostClassId == "" {
		hostClassId = device.Category
	}
	hostSpec.SetClass(hostClassId)

	// Set the boot MAC address:
	hostSpec.SetBootMac(device.Mac)

	// Set the BMC details:
	hostBmc := hostSpec.GetBmc()
	if hostBmc == nil {
		hostBmc = &privatev1.BMC{}
		hostSpec.SetBmc(hostBmc)
	}
	hostBmc.SetUser(device.BmcSettings.UserName)
	hostBmc.SetPassword(device.BmcSettings.Password)
	hostBmc.SetInsecure(true)

	// Find the BMC network interface in order to build the BMC URL:
	for _, iface := range device.Interfaces {
		if iface.ChildType == "NetworkBmcInterface" {
			switch {
			case redfishInterfaceRegex.MatchString(iface.Name):
				url, err := t.buildRedfishUrl(
					ctx,
					iface.IP,
					device.BmcSettings.UserName,
					device.BmcSettings.Password,
					device.Hostname,
				)
				if err != nil {
					t.logger.ErrorContext(
						ctx,
						"Failed to build Redfish URL",
						slog.String("interface", iface.Name),
						slog.String("ip", iface.IP),
						slog.Any("error", err),
					)
				} else {
					hostBmc.SetUrl(url)
				}
			default:
				t.logger.ErrorContext(
					ctx,
					"Unsupported BMC interface type",
					slog.String("interface", iface.Name),
					slog.String("ip", iface.IP),
				)
			}
		}
	}

	// Find the boot network interface in order to extract the boot IP address:
	for _, iface := range device.Interfaces {
		if iface.ChildType == "NetworkPhysicalInterface" && iface.Name == "BOOTIF" {
			hostSpec.SetBootIp(iface.IP)
			break
		}
	}

	// Set the rack:
	rack := t.racksByUuid[device.RackPosition.Rack]
	if rack != nil {
		hostSpec.SetRack(rack.Name)
	}

	// Set the BCM link:
	hostSpec.SetBcmLink(fmt.Sprintf("%s/base-view/device/%s", t.parent.bcmUrl, device.Uuid))

	// Create or update the host:
	err = t.createOrUpdateHost(ctx, host)
	if err != nil {
		return fmt.Errorf("failed to create or update host: %w", err)
	}
	t.logger.DebugContext(
		ctx,
		"Synchronized device",
		slog.String("hostname", device.Hostname),
	)
	return nil
}

// buildRedfishUrl connects to a Redfish server, finds the system ID matching the device name, and builds a virtual
// media URL.
func (t *synchronizerTask) buildRedfishUrl(ctx context.Context, address, username, password, device string) (result string,
	err error) {
	// Connect to the server. Yes, using insecure mode because BCM doesn't contain the CA certificates to use.
	config := gofish.ClientConfig{
		Endpoint:  fmt.Sprintf("https://%s", address),
		Username:  username,
		Password:  password,
		BasicAuth: true,
		Insecure:  true,
	}
	client, err := gofish.ConnectContext(ctx, config)
	if err != nil {
		err = fmt.Errorf("failed to connect to Redfish server: %w", err)
		return
	}
	defer client.Logout()

	// Get the systems:
	systems, err := client.Service.Systems()
	if err != nil {
		err = fmt.Errorf("failed to get systems from Redfish server: %w", err)
		return
	}

	// Find the system matching the device name:
	var path string
	for _, system := range systems {
		if system.Name == device {
			path = system.ODataID
			break
		}
	}
	if path == "" {
		err = fmt.Errorf("system with name '%s' not found", device)
		return
	}

	// Build the virtual media URL:
	result = fmt.Sprintf("redfish-virtualmedia://%s%s", address, path)
	return
}

func (t *synchronizerTask) createOrUpdateHost(ctx context.Context, host *privatev1.Host) error {
	// Try to create the host:
	_, err := t.hostsClient.Create(ctx, privatev1.HostsCreateRequest_builder{
		Object: host,
	}.Build())
	if err == nil {
		t.logger.InfoContext(
			ctx,
			"Created new host",
			slog.String("id", host.GetId()),
		)
		return nil
	}
	if grpcstatus.Code(err) != grpccodes.AlreadyExists {
		return fmt.Errorf("failed to create host: %w", err)
	}

	// Try to update the host:
	_, err = t.hostsClient.Update(ctx, &privatev1.HostsUpdateRequest{
		Object: host,
		UpdateMask: &fieldmaskpb.FieldMask{
			Paths: []string{
				"metadata.name",
				"spec",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update host: %w", err)
	}
	t.logger.InfoContext(
		ctx,
		"Updated existing host",
		slog.String("id", host.GetId()),
	)

	return nil
}

func (t *synchronizerTask) syncCategories(ctx context.Context) error {
	for _, category := range t.categoriesByUuid {
		err := t.syncCategory(ctx, category)
		if err != nil {
			t.logger.WarnContext(
				ctx,
				"Failed to synchronize category",
				slog.String("id", category.Uuid),
				slog.String("name", category.Name),
				slog.Any("error", err),
			)
			continue
		}
	}
	return nil
}

func (t *synchronizerTask) syncCategory(ctx context.Context, category *Category) error {
	// Extract the title and description from the notes:
	type Matter struct {
		HostClass string `yaml:"host_class"`
	}
	var matter Matter
	hostClassTitle, hostClassDescription := t.parseNotes(category.Notes, &matter)
	if hostClassTitle == "" {
		hostClassTitle = fmt.Sprintf(
			"BCM `%s` category",
			category.Name,
		)
	}
	if hostClassDescription == "" {
		hostClassDescription = fmt.Sprintf(
			"# %s\n\nExtracted from BCM device category `%s`.",
			hostClassTitle, category.Name,
		)
	}

	// Use the category name as the host class identifier:
	hostClassId := category.Uuid

	// If the host class name is specified in the front matter then use it, otherwise use the category name:
	hostClassName := matter.HostClass
	if hostClassName == "" {
		hostClassName = category.Name
	}

	// Create or update the host class:
	hostClass := privatev1.HostClass_builder{
		Id: hostClassId,
		Metadata: privatev1.Metadata_builder{
			Name: hostClassName,
		}.Build(),
		Title:       hostClassTitle,
		Description: hostClassDescription,
	}.Build()
	err := t.createOrUpdateHostClass(ctx, hostClass)
	if err != nil {
		return fmt.Errorf("failed to create or update host class: %w", err)
	}
	t.logger.DebugContext(
		ctx,
		"Synchronized host class",
		slog.String("id", hostClassId),
		slog.String("name", hostClassName),
	)

	return nil
}

func (t *synchronizerTask) createOrUpdateHostClass(ctx context.Context, hostClass *privatev1.HostClass) error {
	// Try to create the host class:
	_, err := t.hostClassesClient.Create(ctx, &privatev1.HostClassesCreateRequest{
		Object: hostClass,
	})
	if err == nil {
		t.logger.InfoContext(
			ctx,
			"Created new host class",
			slog.String("id", hostClass.GetId()),
		)
		return nil
	}
	if grpcstatus.Code(err) != grpccodes.AlreadyExists {
		return fmt.Errorf("failed to create host class: %w", err)
	}

	// Try to update the host class:
	_, err = t.hostClassesClient.Update(ctx, &privatev1.HostClassesUpdateRequest{
		Object: hostClass,
		UpdateMask: &fieldmaskpb.FieldMask{
			Paths: []string{
				"metadata.name",
				"title",
				"description",
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update host class: %w", err)
	}
	t.logger.InfoContext(
		ctx,
		"Updated existing host class",
		slog.String("id", hostClass.GetId()),
	)
	return nil
}

func (t *synchronizerTask) parseNotes(notes string, matter any) (title, description string) {
	// If the notes are empty then there is nothing to do, matter title and description are left unchanged.
	if notes == "" {
		return
	}

	// Try to extract the from matter and the rest of the data from the notes. If this fails we will continue
	// assuming that there is no front matter and using the complete document as the description.
	var reader io.Reader
	reader = strings.NewReader(notes)
	data, err := frontmatter.Parse(reader, matter)
	if err != nil {
		t.logger.Error(
			"Failed to extract front matter from notes",
			slog.String("notes", notes),
			slog.Any("error", err),
		)
		data = []byte(notes)
	}

	// Try to find the first heading and use it, without the leading hash characters, as the title:
	reader = bytes.NewReader(data)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			break
		}
	}

	// Return the rest of the data as the description:
	description = string(data)
	return
}

func (t *synchronizerTask) approveCertificateRequests(ctx context.Context) error {
	certificateRequests, err := t.bcmClient.GetCertificateRequests(ctx)
	if err != nil {
		return fmt.Errorf("failed to get certificate requests: %w", err)
	}
	for _, certificateRequest := range certificateRequests {
		err := t.approveCertificateRequest(ctx, certificateRequest)
		if err != nil {
			t.logger.WarnContext(
				ctx,
				"Failed to approve certificate request",
				slog.String("id", certificateRequest.Uuid),
				slog.Any("error", err),
			)
		}
	}
	return nil
}

func (t *synchronizerTask) approveCertificateRequest(ctx context.Context, certificateRequest *CertificateRequest) error {
	issued, err := t.bcmClient.IssueCertificateRequest(
		ctx,
		certificateRequest.Uuid,
		&CertificateSubjectName{
			Entity: Entity{
				BaseType: "CertificateSubjectName",
			},
			CommonName: certificateRequest.CommonName,
			Days:       365,
			Profile:    "litenode",
		},
	)
	if err != nil {
		return fmt.Errorf("failed to approve certificate request: %w", err)
	}
	if !issued {
		return fmt.Errorf("certificate request for '%s' wasn't approved", certificateRequest.CommonName)
	}
	t.logger.InfoContext(
		ctx,
		"Approved certificate request",
		slog.String("id", certificateRequest.Uuid),
		slog.String("name", certificateRequest.CommonName),
	)
	return nil
}

var (
	redfishInterfaceRegex = regexp.MustCompile(`^rf\d+$`)
)

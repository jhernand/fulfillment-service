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
	"context"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	privateapi "github.com/innabox/fulfillment-service/internal/api/private/v1"
)

// SynchronizerBuilder is used to build synchronizers using the builder pattern.
type SynchronizerBuilder struct {
	logger    *slog.Logger
	bcmClient Client
	grpcConn  *grpc.ClientConn
	interval  time.Duration
}

// Synchronizer periodically synchronizes inventory from BCM to the fulfillment service.
type Synchronizer struct {
	logger            *slog.Logger
	bcmClient         Client
	hostsClient       privateapi.HostsClient
	hostClassesClient privateapi.HostClassesClient
	interval          time.Duration
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

// SetBcmClient sets the BCM client.
func (b *SynchronizerBuilder) SetBcmClient(value Client) *SynchronizerBuilder {
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
		bcmClient:         b.bcmClient,
		hostsClient:       privateapi.NewHostsClient(b.grpcConn),
		hostClassesClient: privateapi.NewHostClassesClient(b.grpcConn),
		interval:          interval,
	}
	return
}

// Run starts the synchronization loop.
func (s *Synchronizer) Run(ctx context.Context) error {
	s.logger.InfoContext(
		ctx,
		"Starting inventory synchronization",
		slog.Duration("interval", s.interval),
	)

	// Create a ticker for periodic synchronization:
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	// Run the first synchronization immediately:
	err := s.synchronize(ctx)
	if err != nil {
		s.logger.ErrorContext(
			ctx,
			"Initial synchronization failed",
			slog.Any("error", err),
		)
		// Continue despite the error
	}

	// Run periodic synchronization:
	for {
		select {
		case <-ctx.Done():
			s.logger.InfoContext(ctx, "Stopping inventory synchronization")
			return ctx.Err()
		case <-ticker.C:
			err := s.synchronize(ctx)
			if err != nil {
				s.logger.ErrorContext(
					ctx,
					"Synchronization failed",
					slog.Any("error", err),
				)
				// Continue despite the error
			}
		}
	}
}

// synchronize performs a single synchronization cycle.
func (s *Synchronizer) synchronize(ctx context.Context) error {
	s.logger.InfoContext(ctx, "Starting synchronization cycle")

	// Synchronize categories as host classes:
	err := s.synchronizeCategories(ctx)
	if err != nil {
		s.logger.WarnContext(
			ctx,
			"Failed to synchronize categories",
			slog.Any("error", err),
		)
		// Continue despite the error
	}

	// Get devices from BCM:
	devices, err := s.bcmClient.GetDevices(ctx)
	if err != nil {
		return fmt.Errorf("failed to get devices from BCM: %w", err)
	}

	s.logger.InfoContext(
		ctx,
		"Retrieved devices from BCM",
		slog.Int("count", len(devices)),
	)

	// Process each device:
	for _, device := range devices {
		// Skip non-physical nodes:
		if device.ChildType != "PhysicalNode" {
			continue
		}

		// Convert the device to a host:
		host, err := s.deviceToHost(ctx, &device)
		if err != nil {
			s.logger.WarnContext(
				ctx,
				"Failed to convert device to host",
				slog.String("hostname", device.Hostname),
				slog.Any("error", err),
			)
			continue
		}

		// Create or update the host:
		err = s.createOrUpdateHost(ctx, host)
		if err != nil {
			s.logger.ErrorContext(
				ctx,
				"Failed to create or update host",
				slog.String("hostname", device.Hostname),
				slog.Any("error", err),
			)
			continue
		}

		s.logger.DebugContext(
			ctx,
			"Synchronized host",
			slog.String("id", host.Id),
			slog.String("hostname", device.Hostname),
		)
	}

	s.logger.InfoContext(ctx, "Synchronization cycle completed")
	return nil
}

// deviceToHost converts a BCM device to a fulfillment service host.
func (s *Synchronizer) deviceToHost(ctx context.Context, device *Device) (
	*privateapi.Host, error) {
	// Prepare BMC information:
	bmcInfo := &privateapi.BMC{
		User:     device.BMCSettings.UserName,
		Password: device.BMCSettings.Password,
	}

	// Find the BMC network interface to get the IP address:
	for _, iface := range device.Interfaces {
		if iface.ChildType == "NetworkBmcInterface" {
			bmcInfo.Url = fmt.Sprintf("https://%s", iface.IP)
			break
		}
	}

	// Get the rack name if available:
	var rackName string
	if device.RackPosition.Rack != "" {
		racks, err := s.bcmClient.GetRacksByUuids(ctx, []string{device.RackPosition.Rack})
		if err != nil {
			s.logger.WarnContext(
				ctx,
				"Failed to get rack information",
				slog.String("rack_uuid", device.RackPosition.Rack),
				slog.Any("error", err),
			)
		} else if len(racks) > 0 {
			rackName = racks[0].Name
		}
	}

	// Create the host object:
	host := &privateapi.Host{
		Id: device.Uuid,
		Metadata: &privateapi.Metadata{
			Name: device.Hostname,
		},
		Spec: &privateapi.HostSpec{
			Bmc:  bmcInfo,
			Rack: rackName,
		},
	}

	// Set the class field from the category:
	if device.Category != "" {
		host.Spec.Class = device.Category
	}

	return host, nil
}

// createOrUpdateHost creates a new host or updates an existing one.
func (s *Synchronizer) createOrUpdateHost(ctx context.Context, host *privateapi.Host) error {
	// Try to get the existing host:
	getResp, err := s.hostsClient.Get(ctx, &privateapi.HostsGetRequest{
		Id: host.Id,
	})
	if err != nil {
		// Host doesn't exist, create it:
		s.logger.InfoContext(
			ctx,
			"Creating new host",
			slog.String("id", host.Id),
		)
		_, err = s.hostsClient.Create(ctx, &privateapi.HostsCreateRequest{
			Object: host,
		})
		if err != nil {
			return fmt.Errorf("failed to create host: %w", err)
		}
		return nil
	}

	// Host exists, update it:
	s.logger.InfoContext(
		ctx,
		"Updating existing host",
		slog.String("id", host.Id),
	)

	// Preserve the existing status and only update the spec:
	existingHost := getResp.Object
	existingHost.Spec = host.Spec

	_, err = s.hostsClient.Update(ctx, &privateapi.HostsUpdateRequest{
		Object: existingHost,
		UpdateMask: &fieldmaskpb.FieldMask{
			Paths: []string{"spec"},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update host: %w", err)
	}

	return nil
}

// synchronizeCategories synchronizes categories from BCM as host classes.
func (s *Synchronizer) synchronizeCategories(ctx context.Context) error {
	s.logger.InfoContext(ctx, "Starting category synchronization")

	// Get categories from BCM:
	categories, err := s.bcmClient.GetCategories(ctx)
	if err != nil {
		return fmt.Errorf("failed to get categories from BCM: %w", err)
	}

	s.logger.InfoContext(
		ctx,
		"Retrieved categories from BCM",
		slog.Int("count", len(categories)),
	)

	// Process each category:
	for _, category := range categories {
		// Convert the category to a host class:
		hostClass := &privateapi.HostClass{
			Id: category.Uuid,
			Metadata: &privateapi.Metadata{
				Name: category.Name,
			},
			Title:       fmt.Sprintf("BCM `%s` category", category.Name),
			Description: fmt.Sprintf("BCM `%s` host category extracted from BCM.", category.Name),
		}

		// Create or update the host class:
		err = s.createOrUpdateHostClass(ctx, hostClass)
		if err != nil {
			s.logger.WarnContext(
				ctx,
				"Failed to create or update host class",
				slog.String("id", category.Uuid),
				slog.String("name", category.Name),
				slog.Any("error", err),
			)
			continue
		}

		s.logger.DebugContext(
			ctx,
			"Synchronized host class",
			slog.String("id", category.Uuid),
			slog.String("name", category.Name),
		)
	}

	s.logger.InfoContext(ctx, "Category synchronization completed")
	return nil
}

// createOrUpdateHostClass creates a new host class or updates an existing one.
func (s *Synchronizer) createOrUpdateHostClass(ctx context.Context, hostClass *privateapi.HostClass) error {
	// Try to get the existing host class:
	getResp, err := s.hostClassesClient.Get(ctx, &privateapi.HostClassesGetRequest{
		Id: hostClass.Id,
	})
	if err != nil {
		// Host class doesn't exist, create it:
		s.logger.InfoContext(
			ctx,
			"Creating new host class",
			slog.String("id", hostClass.Id),
		)
		_, err = s.hostClassesClient.Create(ctx, &privateapi.HostClassesCreateRequest{
			Object: hostClass,
		})
		if err != nil {
			return fmt.Errorf("failed to create host class: %w", err)
		}
		return nil
	}

	// Host class exists, update it:
	s.logger.InfoContext(
		ctx,
		"Updating existing host class",
		slog.String("id", hostClass.Id),
	)

	// Update the existing host class:
	existingHostClass := getResp.Object
	existingHostClass.Metadata = hostClass.Metadata
	existingHostClass.Title = hostClass.Title
	existingHostClass.Description = hostClass.Description

	_, err = s.hostClassesClient.Update(ctx, &privateapi.HostClassesUpdateRequest{
		Object: existingHostClass,
		UpdateMask: &fieldmaskpb.FieldMask{
			Paths: []string{"metadata", "title", "description"},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update host class: %w", err)
	}

	return nil
}

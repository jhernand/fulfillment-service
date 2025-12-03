/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package servers

import (
	"context"
	"errors"
	"log/slog"

	privatev1 "github.com/innabox/fulfillment-service/internal/api/private/v1"
	"github.com/innabox/fulfillment-service/internal/auth"
	"github.com/innabox/fulfillment-service/internal/database"
)

// PrivateTenantsServerBuilder contains the data and logic needed to create a new PrivateTenantsServer.
type PrivateTenantsServerBuilder struct {
	logger           *slog.Logger
	notifier         *database.Notifier
	attributionLogic auth.AttributionLogic
	tenancyLogic     auth.TenancyLogic
}

var _ privatev1.TenantsServer = (*PrivateTenantsServer)(nil)

// PrivateTenantsServer is the implementation of the private Tenants gRPC service.
type PrivateTenantsServer struct {
	privatev1.UnimplementedTenantsServer
	logger  *slog.Logger
	generic *GenericServer[*privatev1.Tenant]
}

// NewPrivateTenantsServer creates a builder that can then be used to configure and create a new PrivateTenantsServer.
func NewPrivateTenantsServer() *PrivateTenantsServerBuilder {
	return &PrivateTenantsServerBuilder{}
}

// SetLogger sets the logger. This is mandatory.
func (b *PrivateTenantsServerBuilder) SetLogger(value *slog.Logger) *PrivateTenantsServerBuilder {
	b.logger = value
	return b
}

// SetNotifier sets the database notifier.
func (b *PrivateTenantsServerBuilder) SetNotifier(value *database.Notifier) *PrivateTenantsServerBuilder {
	b.notifier = value
	return b
}

// SetAttributionLogic sets the attribution logic.
func (b *PrivateTenantsServerBuilder) SetAttributionLogic(value auth.AttributionLogic) *PrivateTenantsServerBuilder {
	b.attributionLogic = value
	return b
}

// SetTenancyLogic sets the tenancy logic. This is mandatory.
func (b *PrivateTenantsServerBuilder) SetTenancyLogic(value auth.TenancyLogic) *PrivateTenantsServerBuilder {
	b.tenancyLogic = value
	return b
}

// Build uses the configuration stored in the builder to create a new PrivateTenantsServer.
func (b *PrivateTenantsServerBuilder) Build() (result *PrivateTenantsServer, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.tenancyLogic == nil {
		err = errors.New("tenancy logic is mandatory")
		return
	}

	// Create the generic server:
	generic, err := NewGenericServer[*privatev1.Tenant]().
		SetLogger(b.logger).
		SetService(privatev1.Tenants_ServiceDesc.ServiceName).
		SetTable("tenants").
		SetNotifier(b.notifier).
		SetAttributionLogic(b.attributionLogic).
		SetTenancyLogic(b.tenancyLogic).
		Build()
	if err != nil {
		return
	}

	// Create and populate the object:
	result = &PrivateTenantsServer{
		logger:  b.logger,
		generic: generic,
	}
	return
}

// List returns a list of tenants.
func (s *PrivateTenantsServer) List(ctx context.Context,
	request *privatev1.TenantsListRequest) (response *privatev1.TenantsListResponse, err error) {
	err = s.generic.List(ctx, request, &response)
	return
}

// Get returns a specific tenant by its identifier.
func (s *PrivateTenantsServer) Get(ctx context.Context,
	request *privatev1.TenantsGetRequest) (response *privatev1.TenantsGetResponse, err error) {
	err = s.generic.Get(ctx, request, &response)
	return
}

// Create creates a new tenant.
func (s *PrivateTenantsServer) Create(ctx context.Context,
	request *privatev1.TenantsCreateRequest) (response *privatev1.TenantsCreateResponse, err error) {
	err = s.generic.Create(ctx, request, &response)
	return
}

// Update updates an existing tenant.
func (s *PrivateTenantsServer) Update(ctx context.Context,
	request *privatev1.TenantsUpdateRequest) (response *privatev1.TenantsUpdateResponse, err error) {
	err = s.generic.Update(ctx, request, &response)
	return
}

// Delete deletes an existing tenant.
func (s *PrivateTenantsServer) Delete(ctx context.Context,
	request *privatev1.TenantsDeleteRequest) (response *privatev1.TenantsDeleteResponse, err error) {
	err = s.generic.Delete(ctx, request, &response)
	return
}

// Signal indicates that something changed in the object or the system that may require reconciling the object.
func (s *PrivateTenantsServer) Signal(ctx context.Context,
	request *privatev1.TenantsSignalRequest) (response *privatev1.TenantsSignalResponse, err error) {
	err = s.generic.Signal(ctx, request, &response)
	return
}

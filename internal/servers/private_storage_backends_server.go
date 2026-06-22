/*
Copyright (c) 2026 Red Hat Inc.

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

	"github.com/prometheus/client_golang/prometheus"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
	"github.com/osac-project/fulfillment-service/internal/auth"
	"github.com/osac-project/fulfillment-service/internal/events"
)

type PrivateStorageBackendsServerBuilder struct {
	logger            *slog.Logger
	notifier          events.Notifier
	attributionLogic  auth.AttributionLogic
	tenancyLogic      auth.TenancyLogic
	metricsRegisterer prometheus.Registerer
}

var _ privatev1.StorageBackendsServer = (*PrivateStorageBackendsServer)(nil)

type PrivateStorageBackendsServer struct {
	privatev1.UnimplementedStorageBackendsServer

	logger  *slog.Logger
	generic *GenericServer[*privatev1.StorageBackend]
}

func NewPrivateStorageBackendsServer() *PrivateStorageBackendsServerBuilder {
	return &PrivateStorageBackendsServerBuilder{}
}

func (b *PrivateStorageBackendsServerBuilder) SetLogger(value *slog.Logger) *PrivateStorageBackendsServerBuilder {
	b.logger = value
	return b
}

func (b *PrivateStorageBackendsServerBuilder) SetNotifier(value events.Notifier) *PrivateStorageBackendsServerBuilder {
	b.notifier = value
	return b
}

func (b *PrivateStorageBackendsServerBuilder) SetAttributionLogic(value auth.AttributionLogic) *PrivateStorageBackendsServerBuilder {
	b.attributionLogic = value
	return b
}

func (b *PrivateStorageBackendsServerBuilder) SetTenancyLogic(value auth.TenancyLogic) *PrivateStorageBackendsServerBuilder {
	b.tenancyLogic = value
	return b
}

func (b *PrivateStorageBackendsServerBuilder) SetMetricsRegisterer(value prometheus.Registerer) *PrivateStorageBackendsServerBuilder {
	b.metricsRegisterer = value
	return b
}

func (b *PrivateStorageBackendsServerBuilder) Build() (result *PrivateStorageBackendsServer, err error) {
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.tenancyLogic == nil {
		err = errors.New("tenancy logic is mandatory")
		return
	}

	generic, err := NewGenericServer[*privatev1.StorageBackend]().
		SetLogger(b.logger).
		SetService(privatev1.StorageBackends_ServiceDesc.ServiceName).
		SetNotifier(b.notifier).
		SetAttributionLogic(b.attributionLogic).
		SetTenancyLogic(b.tenancyLogic).
		SetMetricsRegisterer(b.metricsRegisterer).
		Build()
	if err != nil {
		return
	}

	result = &PrivateStorageBackendsServer{
		logger:  b.logger,
		generic: generic,
	}
	return
}

func (s *PrivateStorageBackendsServer) List(ctx context.Context,
	request *privatev1.StorageBackendsListRequest) (response *privatev1.StorageBackendsListResponse, err error) {
	err = s.generic.List(ctx, request, &response)
	return
}

func (s *PrivateStorageBackendsServer) Get(ctx context.Context,
	request *privatev1.StorageBackendsGetRequest) (response *privatev1.StorageBackendsGetResponse, err error) {
	err = s.generic.Get(ctx, request, &response)
	return
}

func (s *PrivateStorageBackendsServer) Create(ctx context.Context,
	request *privatev1.StorageBackendsCreateRequest) (response *privatev1.StorageBackendsCreateResponse, err error) {
	err = s.validateStorageBackendCreate(ctx, request.GetObject())
	if err != nil {
		return
	}

	sb := request.GetObject()
	if sb.Status == nil {
		sb.SetStatus(&privatev1.StorageBackendStatus{})
	}
	sb.GetStatus().SetState(privatev1.StorageBackendState_STORAGE_BACKEND_STATE_READY)

	sb.SetId("")

	// StorageBackend is platform-scoped; force tenant to "shared" so all authenticated users can see it.
	if sb.GetMetadata() == nil {
		sb.SetMetadata(&privatev1.Metadata{})
	}
	sb.GetMetadata().SetTenant(auth.SharedTenant)

	err = s.generic.Create(ctx, request, &response)
	return
}

func (s *PrivateStorageBackendsServer) Update(ctx context.Context,
	request *privatev1.StorageBackendsUpdateRequest) (response *privatev1.StorageBackendsUpdateResponse, err error) {
	id := request.GetObject().GetId()
	if id == "" {
		err = grpcstatus.Errorf(grpccodes.InvalidArgument, "object identifier is mandatory")
		return
	}

	getRequest := &privatev1.StorageBackendsGetRequest{}
	getRequest.SetId(id)
	var getResponse *privatev1.StorageBackendsGetResponse
	err = s.generic.Get(ctx, getRequest, &getResponse)
	if err != nil {
		return
	}

	existingSB := getResponse.GetObject()

	err = s.validateStorageBackendUpdate(ctx, request.GetObject(), existingSB)
	if err != nil {
		return
	}

	err = s.generic.Update(ctx, request, &response)
	return
}

func (s *PrivateStorageBackendsServer) Delete(ctx context.Context,
	request *privatev1.StorageBackendsDeleteRequest) (response *privatev1.StorageBackendsDeleteResponse, err error) {
	err = s.generic.Delete(ctx, request, &response)
	return
}

func (s *PrivateStorageBackendsServer) validateStorageBackendCreate(_ context.Context,
	sb *privatev1.StorageBackend) error {

	if sb == nil {
		return grpcstatus.Errorf(grpccodes.InvalidArgument, "storage backend is mandatory")
	}
	if sb.GetProvider() == "" {
		return grpcstatus.Errorf(grpccodes.InvalidArgument, "field 'provider' is required")
	}
	if sb.GetEndpoint() == "" {
		return grpcstatus.Errorf(grpccodes.InvalidArgument, "field 'endpoint' is required")
	}
	if sb.GetCredentials() == nil || sb.GetCredentials().GetUsername() == "" {
		return grpcstatus.Errorf(grpccodes.InvalidArgument, "field 'credentials.username' is required")
	}
	if sb.GetCredentials().GetPassword() == "" {
		return grpcstatus.Errorf(grpccodes.InvalidArgument, "field 'credentials.password' is required")
	}
	return nil
}

func (s *PrivateStorageBackendsServer) validateStorageBackendUpdate(_ context.Context,
	newSB *privatev1.StorageBackend, existingSB *privatev1.StorageBackend) error {

	if newSB.GetProvider() != "" && newSB.GetProvider() != existingSB.GetProvider() {
		return grpcstatus.Errorf(grpccodes.InvalidArgument,
			"field 'provider' is immutable and cannot be changed from '%s' to '%s'",
			existingSB.GetProvider(), newSB.GetProvider())
	}
	return nil
}

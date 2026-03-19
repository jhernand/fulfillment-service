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

	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
	"github.com/osac-project/fulfillment-service/internal/auth"
	"github.com/osac-project/fulfillment-service/internal/database"
	"github.com/osac-project/fulfillment-service/internal/database/dao"
)

type PrivateClusterTemplatesServerBuilder struct {
	logger           *slog.Logger
	notifier         *database.Notifier
	attributionLogic auth.AttributionLogic
	tenancyLogic     auth.TenancyLogic
}

var _ privatev1.ClusterTemplatesServer = (*PrivateClusterTemplatesServer)(nil)

type PrivateClusterTemplatesServer struct {
	privatev1.UnimplementedClusterTemplatesServer
	logger         *slog.Logger
	generic        *GenericServer[*privatev1.ClusterTemplate]
	templateHelper *TemplateHelper[*privatev1.ClusterTemplate, *privatev1.ClusterTemplateParameterDefinition]
}

func NewPrivateClusterTemplatesServer() *PrivateClusterTemplatesServerBuilder {
	return &PrivateClusterTemplatesServerBuilder{}
}

func (b *PrivateClusterTemplatesServerBuilder) SetLogger(value *slog.Logger) *PrivateClusterTemplatesServerBuilder {
	b.logger = value
	return b
}

func (b *PrivateClusterTemplatesServerBuilder) SetNotifier(
	value *database.Notifier) *PrivateClusterTemplatesServerBuilder {
	b.notifier = value
	return b
}

func (b *PrivateClusterTemplatesServerBuilder) SetAttributionLogic(value auth.AttributionLogic) *PrivateClusterTemplatesServerBuilder {
	b.attributionLogic = value
	return b
}

func (b *PrivateClusterTemplatesServerBuilder) SetTenancyLogic(value auth.TenancyLogic) *PrivateClusterTemplatesServerBuilder {
	b.tenancyLogic = value
	return b
}

func (b *PrivateClusterTemplatesServerBuilder) Build() (result *PrivateClusterTemplatesServer, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.tenancyLogic == nil {
		err = errors.New("tenancy logic is mandatory")
		return
	}

	// Create a DAO for looking up templates (used for parent validation and effective view resolution):
	templatesDao, err := dao.NewGenericDAO[*privatev1.ClusterTemplate]().
		SetLogger(b.logger).
		SetAttributionLogic(b.attributionLogic).
		SetTenancyLogic(b.tenancyLogic).
		Build()
	if err != nil {
		return
	}

	// Create the template helper:
	templateHelper, err := NewTemplateHelper[*privatev1.ClusterTemplate, *privatev1.ClusterTemplateParameterDefinition]().
		SetLogger(b.logger).
		SetDao(templatesDao).
		Build()
	if err != nil {
		return
	}

	// Create the generic server:
	generic, err := NewGenericServer[*privatev1.ClusterTemplate]().
		SetLogger(b.logger).
		SetService(privatev1.ClusterTemplates_ServiceDesc.ServiceName).
		SetNotifier(b.notifier).
		SetAttributionLogic(b.attributionLogic).
		SetTenancyLogic(b.tenancyLogic).
		Build()
	if err != nil {
		return
	}

	// Create and populate the object:
	result = &PrivateClusterTemplatesServer{
		logger:         b.logger,
		generic:        generic,
		templateHelper: templateHelper,
	}
	return
}

func (s *PrivateClusterTemplatesServer) List(ctx context.Context,
	request *privatev1.ClusterTemplatesListRequest) (response *privatev1.ClusterTemplatesListResponse, err error) {
	err = s.generic.List(ctx, request, &response)
	if err != nil {
		return
	}
	if request.GetFlatten() {
		for _, item := range response.GetItems() {
			err = s.templateHelper.Flatten(ctx, item)
			if err != nil {
				s.logger.ErrorContext(
					ctx,
					"Failed to flatten template",
					slog.String("id", item.GetId()),
					slog.Any("error", err),
				)
				err = grpcstatus.Errorf(grpccodes.Internal, "failed to flatten template '%s'", item.GetId())
				return
			}
		}
	}
	return
}

func (s *PrivateClusterTemplatesServer) Get(ctx context.Context,
	request *privatev1.ClusterTemplatesGetRequest) (response *privatev1.ClusterTemplatesGetResponse, err error) {
	err = s.generic.Get(ctx, request, &response)
	if err != nil {
		return
	}
	template := response.GetObject()
	if request.GetFlatten() {
		err = s.templateHelper.Flatten(ctx, template)
		if err != nil {
			s.logger.ErrorContext(
				ctx,
				"Failed to flatten template",
				slog.String("id", template.GetId()),
				slog.Any("error", err),
			)
			err = grpcstatus.Errorf(grpccodes.Internal, "failed to flatten template '%s'", template.GetId())
			return
		}
	}
	return
}

func (s *PrivateClusterTemplatesServer) Create(ctx context.Context,
	request *privatev1.ClusterTemplatesCreateRequest) (response *privatev1.ClusterTemplatesCreateResponse, err error) {
	template := request.GetObject()
	if template != nil {
		err = s.templateHelper.ValidateCreation(ctx, template)
		if err != nil {
			return
		}
		err = s.templateHelper.Normalize(ctx, template)
		if err != nil {
			return
		}
	}
	err = s.generic.Create(ctx, request, &response)
	return
}

func (s *PrivateClusterTemplatesServer) Update(ctx context.Context,
	request *privatev1.ClusterTemplatesUpdateRequest) (response *privatev1.ClusterTemplatesUpdateResponse, err error) {
	template := request.GetObject()
	if template != nil {
		err = s.templateHelper.ValidateUpdate(ctx, template)
		if err != nil {
			return
		}
		err = s.templateHelper.Normalize(ctx, template)
		if err != nil {
			return
		}
	}
	err = s.generic.Update(ctx, request, &response)
	return
}

func (s *PrivateClusterTemplatesServer) Delete(ctx context.Context,
	request *privatev1.ClusterTemplatesDeleteRequest) (response *privatev1.ClusterTemplatesDeleteResponse, err error) {
	err = s.templateHelper.ValidateDeletion(ctx, request.GetId())
	if err != nil {
		return
	}
	err = s.generic.Delete(ctx, request, &response)
	return
}

func (s *PrivateClusterTemplatesServer) Signal(ctx context.Context,
	request *privatev1.ClusterTemplatesSignalRequest) (response *privatev1.ClusterTemplatesSignalResponse, err error) {
	err = s.generic.Signal(ctx, request, &response)
	return
}

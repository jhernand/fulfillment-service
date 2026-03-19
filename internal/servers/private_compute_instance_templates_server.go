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

type PrivateComputeInstanceTemplatesServerBuilder struct {
	logger           *slog.Logger
	notifier         *database.Notifier
	attributionLogic auth.AttributionLogic
	tenancyLogic     auth.TenancyLogic
}

var _ privatev1.ComputeInstanceTemplatesServer = (*PrivateComputeInstanceTemplatesServer)(nil)

type PrivateComputeInstanceTemplatesServer struct {
	privatev1.UnimplementedComputeInstanceTemplatesServer
	logger         *slog.Logger
	generic        *GenericServer[*privatev1.ComputeInstanceTemplate]
	templateHelper *TemplateHelper[
		*privatev1.ComputeInstanceTemplate,
		*privatev1.ComputeInstanceTemplateParameterDefinition,
	]
}

func NewPrivateComputeInstanceTemplatesServer() *PrivateComputeInstanceTemplatesServerBuilder {
	return &PrivateComputeInstanceTemplatesServerBuilder{}
}

func (b *PrivateComputeInstanceTemplatesServerBuilder) SetLogger(value *slog.Logger) *PrivateComputeInstanceTemplatesServerBuilder {
	b.logger = value
	return b
}

func (b *PrivateComputeInstanceTemplatesServerBuilder) SetNotifier(value *database.Notifier) *PrivateComputeInstanceTemplatesServerBuilder {
	b.notifier = value
	return b
}

func (b *PrivateComputeInstanceTemplatesServerBuilder) SetAttributionLogic(value auth.AttributionLogic) *PrivateComputeInstanceTemplatesServerBuilder {
	b.attributionLogic = value
	return b
}

func (b *PrivateComputeInstanceTemplatesServerBuilder) SetTenancyLogic(value auth.TenancyLogic) *PrivateComputeInstanceTemplatesServerBuilder {
	b.tenancyLogic = value
	return b
}

func (b *PrivateComputeInstanceTemplatesServerBuilder) Build() (result *PrivateComputeInstanceTemplatesServer, err error) {
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
	templatesDao, err := dao.NewGenericDAO[*privatev1.ComputeInstanceTemplate]().
		SetLogger(b.logger).
		SetAttributionLogic(b.attributionLogic).
		SetTenancyLogic(b.tenancyLogic).
		Build()
	if err != nil {
		return
	}

	// Create the template helper:
	templateHelper, err := NewTemplateHelper[
		*privatev1.ComputeInstanceTemplate,
		*privatev1.ComputeInstanceTemplateParameterDefinition,
	]().
		SetLogger(b.logger).
		SetDao(templatesDao).
		Build()
	if err != nil {
		return
	}

	// Create the generic server:
	generic, err := NewGenericServer[*privatev1.ComputeInstanceTemplate]().
		SetLogger(b.logger).
		SetService(privatev1.ComputeInstanceTemplates_ServiceDesc.ServiceName).
		SetNotifier(b.notifier).
		SetAttributionLogic(b.attributionLogic).
		SetTenancyLogic(b.tenancyLogic).
		Build()
	if err != nil {
		return
	}

	// Create and populate the object:
	result = &PrivateComputeInstanceTemplatesServer{
		logger:         b.logger,
		generic:        generic,
		templateHelper: templateHelper,
	}
	return
}

func (s *PrivateComputeInstanceTemplatesServer) List(ctx context.Context,
	request *privatev1.ComputeInstanceTemplatesListRequest) (response *privatev1.ComputeInstanceTemplatesListResponse,
	err error) {
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
					slog.String("template", item.GetId()),
					slog.Any("error", err),
				)
				err = grpcstatus.Errorf(
					grpccodes.Internal,
					"failed to flatten template '%s'",
					item.GetId(),
				)
				return
			}
		}
	}
	return
}

func (s *PrivateComputeInstanceTemplatesServer) Get(ctx context.Context,
	request *privatev1.ComputeInstanceTemplatesGetRequest) (response *privatev1.ComputeInstanceTemplatesGetResponse,
	err error) {
	err = s.generic.Get(ctx, request, &response)
	if err != nil {
		return
	}
	if request.GetFlatten() {
		err = s.templateHelper.Flatten(ctx, response.GetObject())
		if err != nil {
			s.logger.ErrorContext(
				ctx,
				"Failed to flatten template",
				slog.String("template", response.GetObject().GetId()),
				slog.Any("error", err),
			)
			err = grpcstatus.Errorf(
				grpccodes.Internal,
				"failed to flatten template '%s'",
				response.GetObject().GetId(),
			)
			return
		}
	}
	return
}

func (s *PrivateComputeInstanceTemplatesServer) Create(ctx context.Context,
	request *privatev1.ComputeInstanceTemplatesCreateRequest) (
	response *privatev1.ComputeInstanceTemplatesCreateResponse, err error) {
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

func (s *PrivateComputeInstanceTemplatesServer) Update(ctx context.Context,
	request *privatev1.ComputeInstanceTemplatesUpdateRequest) (
	response *privatev1.ComputeInstanceTemplatesUpdateResponse, err error) {
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

func (s *PrivateComputeInstanceTemplatesServer) Delete(ctx context.Context,
	request *privatev1.ComputeInstanceTemplatesDeleteRequest) (response *privatev1.ComputeInstanceTemplatesDeleteResponse, err error) {
	err = s.templateHelper.ValidateDeletion(ctx, request.GetId())
	if err != nil {
		return
	}
	err = s.generic.Delete(ctx, request, &response)
	return
}

func (s *PrivateComputeInstanceTemplatesServer) Signal(ctx context.Context,
	request *privatev1.ComputeInstanceTemplatesSignalRequest) (response *privatev1.ComputeInstanceTemplatesSignalResponse, err error) {
	err = s.generic.Signal(ctx, request, &response)
	return
}

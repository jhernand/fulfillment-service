/*
Copyright 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in
compliance with the License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is
distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
implied. See the License for the specific language governing permissions and limitations under the
License.
*/

package servers

import (
	"context"
	"errors"
	"log/slog"

	api "github.com/innabox/fulfillment-service/internal/api/fulfillment/v1"
	"github.com/innabox/fulfillment-service/internal/database/dao"
	"github.com/innabox/fulfillment-service/internal/database/models"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/proto"
)

type ClusterTemplatesServerBuilder struct {
	logger *slog.Logger
	flags  *pflag.FlagSet
	daos   dao.Set
}

var _ api.ClusterTemplatesServer = (*ClusterTemplatesServer)(nil)

type ClusterTemplatesServer struct {
	api.UnimplementedClusterTemplatesServer

	logger *slog.Logger
	daos   dao.Set
}

func NewClusterTemplatesServer() *ClusterTemplatesServerBuilder {
	return &ClusterTemplatesServerBuilder{}
}

func (b *ClusterTemplatesServerBuilder) SetLogger(value *slog.Logger) *ClusterTemplatesServerBuilder {
	b.logger = value
	return b
}

func (b *ClusterTemplatesServerBuilder) SetDAOs(value dao.Set) *ClusterTemplatesServerBuilder {
	b.daos = value
	return b
}

func (b *ClusterTemplatesServerBuilder) SetFlags(value *pflag.FlagSet) *ClusterTemplatesServerBuilder {
	b.flags = value
	return b
}

func (b *ClusterTemplatesServerBuilder) Build() (result *ClusterTemplatesServer, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.daos == nil {
		err = errors.New("data access objects are mandatory")
		return
	}

	// Create and populate the object:
	result = &ClusterTemplatesServer{
		logger: b.logger,
		daos:   b.daos,
	}
	return
}

func (d *ClusterTemplatesServer) List(ctx context.Context,
	request *api.ClusterTemplatesListRequest) (response *api.ClusterTemplatesListResponse, err error) {
	models, err := d.daos.ClusterTemplate().List(ctx)
	if err != nil {
		return
	}
	items := make([]*api.ClusterTemplate, len(models))
	for i, model := range models {
		items[i] = &api.ClusterTemplate{}
		err = d.mapOut(model, items[i])
		if err != nil {
			return
		}
	}
	response = &api.ClusterTemplatesListResponse{
		Size:  proto.Int32(int32(len(items))),
		Total: proto.Int32(int32(len(items))),
		Items: items,
	}
	return
}

func (d *ClusterTemplatesServer) Get(ctx context.Context,
	request *api.ClusterTemplatesGetRequest) (response *api.ClusterTemplatesGetResponse, err error) {
	model, err := d.daos.ClusterTemplate().Get(ctx, request.TemplateId)
	if err != nil {
		return
	}
	if model == nil {
		return
	}
	item := &api.ClusterTemplate{}
	err = d.mapOut(model, item)
	if err != nil {
		return
	}
	response = &api.ClusterTemplatesGetResponse{
		Template: item,
	}
	return
}

func (d *ClusterTemplatesServer) mapOut(model *models.ClusterTemplate, api *api.ClusterTemplate) error {
	api.Id = model.ID
	api.Title = model.Title
	api.Description = model.Description
	return nil
}

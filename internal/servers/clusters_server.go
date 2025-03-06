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

type ClustersServerBuilder struct {
	logger *slog.Logger
	flags  *pflag.FlagSet
	daos   dao.Set
}

var _ api.ClustersServer = (*ClustersServer)(nil)

type ClustersServer struct {
	api.UnimplementedClustersServer

	logger *slog.Logger
	daos   dao.Set
}

func NewClustersServer() *ClustersServerBuilder {
	return &ClustersServerBuilder{}
}

func (b *ClustersServerBuilder) SetLogger(value *slog.Logger) *ClustersServerBuilder {
	b.logger = value
	return b
}

func (b *ClustersServerBuilder) SetDAOs(value dao.Set) *ClustersServerBuilder {
	b.daos = value
	return b
}

func (b *ClustersServerBuilder) SetFlags(value *pflag.FlagSet) *ClustersServerBuilder {
	b.flags = value
	return b
}

func (b *ClustersServerBuilder) Build() (result *ClustersServer, err error) {
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
	result = &ClustersServer{
		logger: b.logger,
		daos:   b.daos,
	}
	return
}

func (d *ClustersServer) List(ctx context.Context,
	request *api.ClustersListRequest) (response *api.ClustersListResponse, err error) {
	models, err := d.daos.Clusters().List(ctx)
	if err != nil {
		return
	}
	items := make([]*api.Cluster, len(models))
	for i, model := range models {
		items[i] = &api.Cluster{}
		err = d.mapOutbound(model, items[i])
		if err != nil {
			return
		}
	}
	response = &api.ClustersListResponse{
		Size:  proto.Int32(int32(len(items))),
		Total: proto.Int32(int32(len(items))),
		Items: items,
	}
	return
}

func (d *ClustersServer) Get(ctx context.Context,
	request *api.ClustersGetRequest) (response *api.ClustersGetResponse, err error) {
	model, err := d.daos.Clusters().Get(ctx, request.ClusterId)
	if err != nil {
		return
	}
	if model == nil {
		return
	}
	item := &api.Cluster{}
	err = d.mapOutbound(model, item)
	if err != nil {
		return
	}
	response = &api.ClustersGetResponse{
		Cluster: item,
	}
	return
}

func (d *ClustersServer) mapOutbound(from *models.Cluster, to *api.Cluster) error {
	to.Id = from.ID
	to.ApiUrl = from.APIURL
	to.ConsoleUrl = from.ConsoleURL
	return nil
}

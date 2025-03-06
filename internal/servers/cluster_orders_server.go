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
	"fmt"
	"log/slog"

	api "github.com/innabox/fulfillment-service/internal/api/fulfillment/v1"
	"github.com/innabox/fulfillment-service/internal/database/dao"
	"github.com/innabox/fulfillment-service/internal/database/models"
	"github.com/spf13/pflag"
	"google.golang.org/protobuf/proto"
)

type ClusterOrdersServerBuilder struct {
	logger *slog.Logger
	flags  *pflag.FlagSet
	daos   dao.Set
}

var _ api.ClusterOrdersServer = (*ClusterOrdersServer)(nil)

type ClusterOrdersServer struct {
	api.UnimplementedClusterOrdersServer

	logger *slog.Logger
	daos   dao.Set
}

func NewClusterOrdersServer() *ClusterOrdersServerBuilder {
	return &ClusterOrdersServerBuilder{}
}

func (b *ClusterOrdersServerBuilder) SetLogger(value *slog.Logger) *ClusterOrdersServerBuilder {
	b.logger = value
	return b
}

func (b *ClusterOrdersServerBuilder) SetDAOs(value dao.Set) *ClusterOrdersServerBuilder {
	b.daos = value
	return b
}

func (b *ClusterOrdersServerBuilder) SetFlags(value *pflag.FlagSet) *ClusterOrdersServerBuilder {
	b.flags = value
	return b
}

func (b *ClusterOrdersServerBuilder) Build() (result *ClusterOrdersServer, err error) {
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
	result = &ClusterOrdersServer{
		logger: b.logger,
		daos:   b.daos,
	}
	return
}

func (d *ClusterOrdersServer) List(ctx context.Context,
	request *api.ClusterOrdersListRequest) (response *api.ClusterOrdersListResponse, err error) {
	models, err := d.daos.ClusterOrders().List(ctx)
	if err != nil {
		return
	}
	items := make([]*api.ClusterOrder, len(models))
	for i, model := range models {
		items[i] = &api.ClusterOrder{}
		err = d.mapOutbound(model, items[i])
		if err != nil {
			return
		}
	}
	response = &api.ClusterOrdersListResponse{
		Size:  proto.Int32(int32(len(items))),
		Total: proto.Int32(int32(len(items))),
		Items: items,
	}
	return
}

func (d *ClusterOrdersServer) Get(ctx context.Context,
	request *api.ClusterOrdersGetRequest) (response *api.ClusterOrdersGetResponse, err error) {
	model, err := d.daos.ClusterOrders().Get(ctx, request.OrderId)
	if err != nil {
		return
	}
	if model == nil {
		return
	}
	item := &api.ClusterOrder{}
	err = d.mapOutbound(model, item)
	if err != nil {
		return
	}
	response = &api.ClusterOrdersGetResponse{
		Order: item,
	}
	return
}

func (d *ClusterOrdersServer) Place(ctx context.Context,
	request *api.ClusterOrdersPlaceRequest) (response *api.ClusterOrdersPlaceResponse, err error) {
	// Validate the request:
	if request.Order == nil {
		err = fmt.Errorf("order is mandatory")
		return
	}
	if request.Order.Id != "" {
		err = fmt.Errorf("order identifier isn't allowed")
		return
	}
	if request.Order.TemplateId == "" {
		err = fmt.Errorf("template identifier is mandatory")
		return
	}
	if request.Order.State != api.ClusterOrder_UNSPECIFIED {
		err = fmt.Errorf("state isn't allowed")
		return
	}
	if request.Order.ClusterId != "" {
		err = fmt.Errorf("cluster identifier isn't allowed")
		return
	}

	// Check that the requested template exists:
	template, err := d.daos.ClusterTemplates().Get(ctx, request.Order.TemplateId)
	if err != nil {
		return
	}
	if template == nil {
		err = fmt.Errorf("template with identifier '%s' doesn't exist", request.Order.TemplateId)
		return
	}

	// Insert the new order:
	order := &models.ClusterOrder{
		TemplateID: request.Order.TemplateId,
	}
	id, err := d.daos.ClusterOrders().Insert(ctx, order)
	if err != nil {
		return
	}

	// Fetch the result:
	order, err = d.daos.ClusterOrders().Get(ctx, id)
	if err != nil {
		return
	}
	item := &api.ClusterOrder{}
	err = d.mapOutbound(order, item)
	if err != nil {
		return
	}
	response = &api.ClusterOrdersPlaceResponse{
		Order: item,
	}
	return
}

func (d *ClusterOrdersServer) Cancel(ctx context.Context,
	request *api.ClusterOrdersCancelRequest) (response *api.ClusterOrdersCancelResponse, err error) {
	// Check that the requested order exists:
	ok, err := d.daos.ClusterOrders().Exists(ctx, request.OrderId)
	if err != nil {
		return
	}
	if !ok {
		err = fmt.Errorf("order with identifier '%s' doesn't exist", request.OrderId)
		return
	}

	// Update the state:
	err = d.daos.ClusterOrders().UpdateState(ctx, request.OrderId, models.ClusterOrderStateCanceled)
	if err != nil {
		return
	}
	response = &api.ClusterOrdersCancelResponse{}
	return
}

func (d *ClusterOrdersServer) mapOutbound(from *models.ClusterOrder, to *api.ClusterOrder) error {
	to.Id = from.ID
	to.TemplateId = from.TemplateID
	switch from.State {
	case models.ClusterOrderStateUnspecified:
		to.State = api.ClusterOrder_UNSPECIFIED
	case models.ClusterOrderStateAccepted:
		to.State = api.ClusterOrder_ACCEPTED
	case models.ClusterOrderStateRejected:
		to.State = api.ClusterOrder_REJECTED
	case models.ClusterOrderStateFulfilled:
		to.State = api.ClusterOrder_FULFILLED
	case models.ClusterOrderStateCanceled:
		to.State = api.ClusterOrder_CANCELED
	case models.ClusterOrderStateFailed:
		to.State = api.ClusterOrder_FAILED
	default:
		return fmt.Errorf("value '%s' doesn't correspond to any known cluster order state", from.State)
	}
	return nil
}

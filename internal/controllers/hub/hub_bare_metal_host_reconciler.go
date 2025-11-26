/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package hub

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clnt "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	privatev1 "github.com/innabox/fulfillment-service/internal/api/private/v1"
	"github.com/innabox/fulfillment-service/internal/kubernetes/gvks"
	"github.com/innabox/fulfillment-service/internal/kubernetes/labels"
)

// bareMetalHostReconciler is a reconciler for BareMetalHost resources.
type bareMetalHostReconciler struct {
	logger      *slog.Logger
	hubId       string
	connection  *grpc.ClientConn
	hubClient   clnt.Client
	hostsClient privatev1.HostsClient
}

// Reconcile handles a reconciliation request for a BareMetalHost resource.
func (r *bareMetalHostReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	// Create a logger with common fields:
	logger := r.logger.With(
		slog.String("hub", r.hubId),
		slog.String("namespace", req.Namespace),
		slog.String("name", req.Name),
	)

	logger.DebugContext(
		ctx,
		"Reconciling BareMetalHost resource",
	)

	// Get the BareMetalHost object from Kubernetes:
	bareMetalHost := &unstructured.Unstructured{}
	bareMetalHost.SetGroupVersionKind(gvks.BareMetalHost)
	err := r.hubClient.Get(ctx, req.NamespacedName, bareMetalHost)
	if err != nil {
		logger.ErrorContext(
			ctx,
			"Failed to get BareMetalHost",
			slog.Any("error", err),
		)
		return reconcile.Result{}, clnt.IgnoreNotFound(err)
	}

	// Check if the BareMetalHost has the host identifier label:
	hostLabels := bareMetalHost.GetLabels()
	if hostLabels == nil {
		logger.DebugContext(
			ctx,
			"BareMetalHost has no labels",
		)
		return reconcile.Result{}, nil
	}

	hostId, ok := hostLabels[labels.HostId]
	if !ok || hostId == "" {
		logger.DebugContext(
			ctx,
			"BareMetalHost does not have host identifier label",
		)
		return reconcile.Result{}, nil
	}

	// Add the host identifier to the logger:
	logger = logger.With(slog.String("host", hostId))

	// Extract the status.poweredOn field from the BareMetalHost:
	poweredOn, found, err := unstructured.NestedBool(bareMetalHost.Object, "status", "poweredOn")
	if err != nil {
		logger.ErrorContext(
			ctx,
			"Failed to extract status.poweredOn from BareMetalHost",
			slog.Any("error", err),
		)
		return reconcile.Result{}, err
	}

	if !found {
		logger.DebugContext(
			ctx,
			"BareMetalHost does not have status.poweredOn field",
		)
		return reconcile.Result{}, nil
	}

	// Determine the power state:
	var powerState privatev1.HostPowerState
	if poweredOn {
		powerState = privatev1.HostPowerState_HOST_POWER_STATE_ON
	} else {
		powerState = privatev1.HostPowerState_HOST_POWER_STATE_OFF
	}

	logger.InfoContext(
		ctx,
		"Updating host power state from BareMetalHost",
		slog.Bool("powered_on", poweredOn),
		slog.String("power_state", powerState.String()),
	)

	// Create an update mask to ensure only the power state is updated:
	updateMask := &fieldmaskpb.FieldMask{
		Paths: []string{"status.power_state"},
	}

	// Update the host via gRPC:
	_, err = r.hostsClient.Update(ctx, privatev1.HostsUpdateRequest_builder{
		Object: &privatev1.Host{
			Id: hostId,
			Status: &privatev1.HostStatus{
				PowerState: powerState,
			},
		},
		UpdateMask: updateMask,
	}.Build())
	if err != nil {
		logger.ErrorContext(
			ctx,
			"Failed to update host power state",
			slog.Any("error", err),
		)
		return reconcile.Result{}, err
	}

	logger.InfoContext(
		ctx,
		"Successfully updated host power state",
		slog.String("power_state", powerState.String()),
	)

	return reconcile.Result{}, nil
}

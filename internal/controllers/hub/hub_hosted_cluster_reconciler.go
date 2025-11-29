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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	clnt "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	privatev1 "github.com/innabox/fulfillment-service/internal/api/private/v1"
	"github.com/innabox/fulfillment-service/internal/kubernetes/gvks"
	"github.com/innabox/fulfillment-service/internal/kubernetes/labels"
)

// hostedClusterReconciler is a reconciler for HostedCluster resources.
type hostedClusterReconciler struct {
	logger         *slog.Logger
	connection     *grpc.ClientConn
	hubClient      clnt.Client
	clustersClient privatev1.ClustersClient
}

// Reconcile handles a reconciliation request for a HostedCluster resource.
func (r *hostedClusterReconciler) Reconcile(ctx context.Context, req reconcile.Request) (result reconcile.Result, err error) {
	// Create a logger with common fields:
	logger := r.logger.With(
		slog.String("namespace", req.Namespace),
		slog.String("name", req.Name),
	)

	// Get the Kubernetes object:
	hostedCluster := &unstructured.Unstructured{}
	hostedCluster.SetGroupVersionKind(gvks.HostedCluster)
	err = r.hubClient.Get(ctx, req.NamespacedName, hostedCluster)
	if apierrors.IsNotFound(err) {
		logger.DebugContext(
			ctx,
			"Hosted cluster not found, will ignore it",
		)
		err = nil
		return
	}
	if err != nil {
		logger.ErrorContext(
			ctx,
			"Failed to get hosted cluster",
			slog.Any("error", err),
		)
		return
	}

	// Check if the object has the cluster identifier label. If it doesn't then this cluster wasn't created by us,
	// so we don't need to reconcile it.
	clusterId := hostedCluster.GetLabels()[labels.ClusterId]
	if clusterId == "" {
		logger.DebugContext(
			ctx,
			"Hosted cluster doesn't have the identifier labels, will ignore it",
		)
		return
	}

	// Add the cluster identifier to the logger:
	logger = logger.With(slog.String("cluster", clusterId))

	// Signal the cluster to that it will be reconciled again:
	_, err = r.clustersClient.Signal(ctx, privatev1.ClustersSignalRequest_builder{
		Id: clusterId,
	}.Build())
	if err != nil {
		logger.ErrorContext(
			ctx,
			"Failed to signal cluster",
			slog.Any("error", err),
		)
		return
	}
	logger.DebugContext(
		ctx,
		"Successfully signaled cluster",
	)

	return
}

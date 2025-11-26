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
	"fmt"
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

// hostedClusterReconciler is a reconciler for HostedCluster resources.
type hostedClusterReconciler struct {
	logger         *slog.Logger
	hubId          string
	connection     *grpc.ClientConn
	hubClient      clnt.Client
	clustersClient privatev1.ClustersClient
}

// Reconcile handles a reconciliation request for a HostedCluster resource.
func (r *hostedClusterReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	// Create a logger with common fields:
	logger := r.logger.With(
		slog.String("hub", r.hubId),
		slog.String("namespace", req.Namespace),
		slog.String("name", req.Name),
	)

	logger.DebugContext(
		ctx,
		"Reconciling HostedCluster resource",
	)

	// Get the HostedCluster object from Kubernetes:
	hostedCluster := &unstructured.Unstructured{}
	hostedCluster.SetGroupVersionKind(gvks.HostedCluster)
	err := r.hubClient.Get(ctx, req.NamespacedName, hostedCluster)
	if err != nil {
		logger.ErrorContext(
			ctx,
			"Failed to get HostedCluster",
			slog.Any("error", err),
		)
		return reconcile.Result{}, clnt.IgnoreNotFound(err)
	}

	// Check if the HostedCluster has the cluster identifier label:
	clusterLabels := hostedCluster.GetLabels()
	if clusterLabels == nil {
		logger.DebugContext(
			ctx,
			"HostedCluster has no labels",
		)
		return reconcile.Result{}, nil
	}

	clusterId, ok := clusterLabels[labels.ClusterId]
	if !ok || clusterId == "" {
		logger.DebugContext(
			ctx,
			"HostedCluster does not have cluster identifier label",
		)
		return reconcile.Result{}, nil
	}

	// Add the cluster identifier to the logger:
	logger = logger.With(slog.String("cluster", clusterId))

	// Extract the status.controlPlaneEndpoint.host field from the HostedCluster:
	host, found, err := unstructured.NestedString(hostedCluster.Object, "status", "controlPlaneEndpoint", "host")
	if err != nil {
		logger.ErrorContext(
			ctx,
			"Failed to extract status.controlPlaneEndpoint.host from HostedCluster",
			slog.Any("error", err),
		)
		return reconcile.Result{}, err
	}

	if !found || host == "" {
		logger.DebugContext(
			ctx,
			"HostedCluster does not have status.controlPlaneEndpoint.host field or it is empty",
		)
		return reconcile.Result{}, nil
	}

	// Extract the status.controlPlaneEndpoint.port field from the HostedCluster:
	port, found, err := unstructured.NestedInt64(hostedCluster.Object, "status", "controlPlaneEndpoint", "port")
	if err != nil {
		logger.ErrorContext(
			ctx,
			"Failed to extract status.controlPlaneEndpoint.port from HostedCluster",
			slog.Any("error", err),
		)
		return reconcile.Result{}, err
	}

	if !found || port == 0 {
		logger.DebugContext(
			ctx,
			"HostedCluster does not have status.controlPlaneEndpoint.port field or it is zero",
		)
		return reconcile.Result{}, nil
	}

	// Construct the API URL:
	apiUrl := fmt.Sprintf("https://%s:%d", host, port)

	// Extract the spec.dns.baseDomain field from the HostedCluster:
	baseDomain, found, err := unstructured.NestedString(hostedCluster.Object, "spec", "dns", "baseDomain")
	if err != nil {
		logger.ErrorContext(
			ctx,
			"Failed to extract spec.dns.baseDomain from HostedCluster",
			slog.Any("error", err),
		)
		return reconcile.Result{}, err
	}

	if !found || baseDomain == "" {
		logger.DebugContext(
			ctx,
			"HostedCluster does not have spec.dns.baseDomain field or it is empty",
		)
		return reconcile.Result{}, nil
	}

	// Get the name from the HostedCluster metadata:
	clusterName := hostedCluster.GetName()

	// Construct the console URL:
	consoleUrl := fmt.Sprintf("https://console-openshift-console.apps.%s.%s", clusterName, baseDomain)

	logger.InfoContext(
		ctx,
		"Updating cluster API URL and console URL from HostedCluster",
		slog.String("host", host),
		slog.Int64("port", port),
		slog.String("api_url", apiUrl),
		slog.String("base_domain", baseDomain),
		slog.String("console_url", consoleUrl),
	)

	// Update the cluster via gRPC:
	_, err = r.clustersClient.Update(ctx, privatev1.ClustersUpdateRequest_builder{
		Object: &privatev1.Cluster{
			Id: clusterId,
			Status: &privatev1.ClusterStatus{
				ApiUrl:     apiUrl,
				ConsoleUrl: consoleUrl,
			},
		},
		UpdateMask: &fieldmaskpb.FieldMask{
			Paths: []string{
				"status.api_url",
				"status.console_url",
			},
		},
	}.Build())
	if err != nil {
		logger.ErrorContext(
			ctx,
			"Failed to update cluster API URL",
			slog.Any("error", err),
		)
		return reconcile.Result{}, err
	}

	logger.InfoContext(
		ctx,
		"Successfully updated cluster API URL and console URL",
		slog.String("api_url", apiUrl),
		slog.String("console_url", consoleUrl),
	)

	return reconcile.Result{}, nil
}

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
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	privatev1 "github.com/innabox/fulfillment-service/internal/api/private/v1"
	"github.com/innabox/fulfillment-service/internal/kubernetes/gvks"
)

// WatcherBuilder contains the data and logic needed to build a watcher for hub Kubernetes resources.
type WatcherBuilder struct {
	logger     *slog.Logger
	hubId      string
	restConfig *rest.Config
	connection *grpc.ClientConn
	hubClient  client.Client
}

// Watcher watches Kubernetes resources in a hub cluster.
type Watcher struct {
	logger     *slog.Logger
	hubId      string
	connection *grpc.ClientConn
	hubClient  client.Client
	manager    manager.Manager
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewWatcher creates a new builder that can then be used to create a watcher.
func NewWatcher() *WatcherBuilder {
	return &WatcherBuilder{}
}

// SetLogger sets the logger. This is mandatory.
func (b *WatcherBuilder) SetLogger(value *slog.Logger) *WatcherBuilder {
	b.logger = value
	return b
}

// SetHubId sets the hub identifier. This is mandatory.
func (b *WatcherBuilder) SetHubId(value string) *WatcherBuilder {
	b.hubId = value
	return b
}

// SetRestConfig sets the REST config for the hub cluster. This is mandatory.
func (b *WatcherBuilder) SetRestConfig(value *rest.Config) *WatcherBuilder {
	b.restConfig = value
	return b
}

// SetConnection sets the gRPC client connection. This is mandatory.
func (b *WatcherBuilder) SetConnection(value *grpc.ClientConn) *WatcherBuilder {
	b.connection = value
	return b
}

// SetHubClient sets the Kubernetes client for the hub. This is mandatory.
func (b *WatcherBuilder) SetHubClient(value client.Client) *WatcherBuilder {
	b.hubClient = value
	return b
}

// Build uses the information stored in the builder to create a new watcher.
func (b *WatcherBuilder) Build() (result *Watcher, err error) {
	// Check parameters:
	if b.logger == nil {
		err = errors.New("logger is mandatory")
		return
	}
	if b.hubId == "" {
		err = errors.New("hub id is mandatory")
		return
	}
	if b.restConfig == nil {
		err = errors.New("rest config is mandatory")
		return
	}
	if b.connection == nil {
		err = errors.New("connection is mandatory")
		return
	}
	if b.hubClient == nil {
		err = errors.New("hub client is mandatory")
		return
	}

	// Create a context for the watcher:
	ctx, cancel := context.WithCancel(context.Background())

	// Create the controller manager options:
	options := manager.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	}

	// Create the controller manager:
	mgr, err := manager.New(b.restConfig, options)
	if err != nil {
		cancel()
		return
	}

	// Create and populate the watcher object:
	result = &Watcher{
		logger:     b.logger,
		hubId:      b.hubId,
		connection: b.connection,
		hubClient:  b.hubClient,
		manager:    mgr,
		ctx:        ctx,
		cancel:     cancel,
	}

	// Set up the reconcilers for the different resource types:
	err = result.setupReconcilers()
	if err != nil {
		cancel()
		return
	}

	return
}

// Start starts the watcher. This method blocks until the context is cancelled.
func (w *Watcher) Start(ctx context.Context) error {
	// Create a context that will be cancelled when either the parent context is cancelled or when Stop is called:
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-w.ctx.Done()
		cancel()
	}()

	w.logger.InfoContext(
		ctx,
		"Starting Kubernetes resource watcher",
		slog.String("hub_id", w.hubId),
	)

	err := w.manager.Start(ctx)
	if err != nil {
		w.logger.ErrorContext(
			ctx,
			"Kubernetes resource watcher failed",
			slog.String("hub_id", w.hubId),
			slog.Any("error", err),
		)
		return err
	}

	w.logger.InfoContext(
		ctx,
		"Kubernetes resource watcher stopped",
		slog.String("hub_id", w.hubId),
	)

	return nil
}

// Stop stops the watcher.
func (w *Watcher) Stop() {
	w.logger.InfoContext(
		context.Background(),
		"Stopping Kubernetes resource watcher",
		slog.String("hub_id", w.hubId),
	)
	w.cancel()
}

// setupReconcilers sets up the reconcilers for the different resource types.
func (w *Watcher) setupReconcilers() error {
	// Create a reconciler for HostedCluster resources:
	hostedClusterReconciler := &hostedClusterReconciler{
		logger:     w.logger,
		hubId:      w.hubId,
		connection: w.connection,
		hubClient:  w.hubClient,
	}
	err := w.setupReconciler(hostedClusterReconciler, gvks.HostedCluster)
	if err != nil {
		return err
	}

	// Create a reconciler for NodePool resources:
	nodePoolReconciler := &nodePoolReconciler{
		logger:     w.logger,
		hubId:      w.hubId,
		connection: w.connection,
		hubClient:  w.hubClient,
	}
	err = w.setupReconciler(nodePoolReconciler, gvks.NodePool)
	if err != nil {
		return err
	}

	// Create a reconciler for BareMetalHost resources:
	bareMetalHostReconciler := &bareMetalHostReconciler{
		logger:      w.logger,
		hubId:       w.hubId,
		connection:  w.connection,
		hubClient:   w.hubClient,
		hostsClient: privatev1.NewHostsClient(w.connection),
	}
	err = w.setupReconciler(bareMetalHostReconciler, gvks.BareMetalHost)
	if err != nil {
		return err
	}

	return nil
}

// setupReconciler sets up a reconciler for a specific resource type.
func (w *Watcher) setupReconciler(r reconcile.Reconciler, gvk schema.GroupVersionKind) error {
	// Create an unstructured object for the resource type:
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	// Create the controller and configure it to watch the resource type:
	err := ctrl.NewControllerManagedBy(w.manager).
		For(obj).
		Complete(r)
	if err != nil {
		return err
	}

	// Alternative using builder API:
	// err := builder.ControllerManagedBy(w.manager).
	// 	For(obj).
	// 	Complete(r)

	w.logger.DebugContext(
		context.Background(),
		"Configured reconciler for resource type",
		slog.String("hub_id", w.hubId),
		slog.String("group", gvk.Group),
		slog.String("version", gvk.Version),
		slog.String("kind", gvk.Kind),
	)

	return nil
}

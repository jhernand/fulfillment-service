/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package cluster

import (
	"context"
	"errors"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
	"github.com/osac-project/fulfillment-service/internal/controllers"
	"github.com/osac-project/fulfillment-service/internal/controllers/finalizers"
	"github.com/osac-project/fulfillment-service/internal/kubernetes/annotations"
	"github.com/osac-project/fulfillment-service/internal/kubernetes/labels"
	osacv1alpha1 "github.com/osac-project/osac-operator/api/v1alpha1"
)

var _ = Describe("validateTenant", func() {
	It("should succeed when a tenant is assigned", func() {
		t := &task{
			cluster: privatev1.Cluster_builder{
				Id: "test-cluster",
				Metadata: privatev1.Metadata_builder{
					Tenant: "tenant-1",
				}.Build(),
			}.Build(),
		}

		err := t.validateTenant()
		Expect(err).ToNot(HaveOccurred())
	})

	It("should fail when tenant is empty", func() {
		t := &task{
			cluster: privatev1.Cluster_builder{
				Id: "test-cluster",
				Metadata: privatev1.Metadata_builder{
					Tenant: "",
				}.Build(),
			}.Build(),
		}

		err := t.validateTenant()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("tenant"))
	})

	It("should fail when metadata is missing", func() {
		t := &task{
			cluster: privatev1.Cluster_builder{
				Id: "test-cluster",
			}.Build(),
		}

		err := t.validateTenant()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("tenant"))
	})
})

var _ = Describe("update tenant annotation", func() {
	const (
		clusterID    = "test-cluster-id"
		tenantName   = "my-tenant"
		hubID        = "test-hub"
		hubNamespace = "test-ns"
	)

	var (
		ctx  context.Context
		ctrl *gomock.Controller
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		DeferCleanup(ctrl.Finish)
	})

	It("should set tenant annotation when creating a new ClusterOrder CR", func() {
		scheme := runtime.NewScheme()
		Expect(osacv1alpha1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		hubCache := controllers.NewMockHubCache(ctrl)
		hubCache.EXPECT().
			Get(gomock.Any(), hubID).
			Return(&controllers.HubEntry{
				Namespace: hubNamespace,
				Client:    fakeClient,
			}, nil)

		cluster := privatev1.Cluster_builder{
			Id: clusterID,
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{finalizers.Controller},
				Tenant:     tenantName,
			}.Build(),
			Spec: privatev1.ClusterSpec_builder{
				Template: "test-template",
			}.Build(),
			Status: privatev1.ClusterStatus_builder{
				State: privatev1.ClusterState_CLUSTER_STATE_PROGRESSING,
				Hub:   hubID,
			}.Build(),
		}.Build()

		t := &task{
			r: &function{
				logger:         logger,
				hubCache:       hubCache,
				maskCalculator: nil,
			},
			cluster: cluster,
		}

		err := t.update(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Verify the ClusterOrder CR was created with the tenant annotation
		list := &osacv1alpha1.ClusterOrderList{}
		err = fakeClient.List(ctx, list)
		Expect(err).ToNot(HaveOccurred())
		Expect(list.Items).To(HaveLen(1))

		createdCR := list.Items[0]
		Expect(createdCR.GetAnnotations()).To(HaveKeyWithValue(annotations.Tenant, tenantName))
		Expect(createdCR.GetLabels()).To(HaveKeyWithValue(labels.ClusterOrderUuid, clusterID))
	})

	It("should update ClusterOrder when node set size changes on a ready cluster", func() {
		// Create an existing ClusterOrder with size 3:
		existingOrder := &osacv1alpha1.ClusterOrder{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "order-abc",
				Namespace: hubNamespace,
				Labels: map[string]string{
					labels.ClusterOrderUuid: clusterID,
				},
				Annotations: map[string]string{
					annotations.Tenant: tenantName,
				},
			},
			Spec: osacv1alpha1.ClusterOrderSpec{
				TemplateID: "test-template",
				NodeRequests: []osacv1alpha1.NodeRequest{
					{
						ResourceClass: "gpu.gb200",
						NumberOfNodes: 3,
					},
				},
			},
		}

		scheme := runtime.NewScheme()
		Expect(osacv1alpha1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(existingOrder).
			Build()

		hubCache := controllers.NewMockHubCache(ctrl)
		hubCache.EXPECT().
			Get(gomock.Any(), hubID).
			Return(&controllers.HubEntry{
				Namespace: hubNamespace,
				Client:    fakeClient,
			}, nil)

		// Create a cluster in READY state with updated node set size (5):
		cluster := privatev1.Cluster_builder{
			Id: clusterID,
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{finalizers.Controller},
				Tenant:     tenantName,
			}.Build(),
			Spec: privatev1.ClusterSpec_builder{
				Template: "test-template",
				NodeSets: map[string]*privatev1.ClusterNodeSet{
					"gpu.gb200": privatev1.ClusterNodeSet_builder{
						HostType: "gpu.gb200",
						Size:     5,
					}.Build(),
				},
			}.Build(),
			Status: privatev1.ClusterStatus_builder{
				State: privatev1.ClusterState_CLUSTER_STATE_READY,
				Hub:   hubID,
			}.Build(),
		}.Build()

		t := &task{
			r: &function{
				logger:         logger,
				hubCache:       hubCache,
				maskCalculator: nil,
			},
			cluster: cluster,
		}

		err := t.update(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Verify the ClusterOrder was patched with the new size:
		list := &osacv1alpha1.ClusterOrderList{}
		err = fakeClient.List(ctx, list)
		Expect(err).ToNot(HaveOccurred())
		Expect(list.Items).To(HaveLen(1))

		updatedCR := list.Items[0]
		Expect(updatedCR.Spec.NodeRequests).To(HaveLen(1))
		Expect(updatedCR.Spec.NodeRequests[0].ResourceClass).To(Equal("gpu.gb200"))
		Expect(updatedCR.Spec.NodeRequests[0].NumberOfNodes).To(Equal(5))
	})

	It("should update ClusterOrder when node set size changes on a progressing cluster", func() {
		// Create an existing ClusterOrder with size 3:
		existingOrder := &osacv1alpha1.ClusterOrder{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "order-abc",
				Namespace: hubNamespace,
				Labels: map[string]string{
					labels.ClusterOrderUuid: clusterID,
				},
				Annotations: map[string]string{
					annotations.Tenant: tenantName,
				},
			},
			Spec: osacv1alpha1.ClusterOrderSpec{
				TemplateID: "test-template",
				NodeRequests: []osacv1alpha1.NodeRequest{
					{
						ResourceClass: "gpu.gb200",
						NumberOfNodes: 3,
					},
				},
			},
		}

		scheme := runtime.NewScheme()
		Expect(osacv1alpha1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(existingOrder).
			Build()

		hubCache := controllers.NewMockHubCache(ctrl)
		hubCache.EXPECT().
			Get(gomock.Any(), hubID).
			Return(&controllers.HubEntry{
				Namespace: hubNamespace,
				Client:    fakeClient,
			}, nil)

		// Create a cluster in PROGRESSING state with updated node set size (5):
		cluster := privatev1.Cluster_builder{
			Id: clusterID,
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{finalizers.Controller},
				Tenant:     tenantName,
			}.Build(),
			Spec: privatev1.ClusterSpec_builder{
				Template: "test-template",
				NodeSets: map[string]*privatev1.ClusterNodeSet{
					"gpu.gb200": privatev1.ClusterNodeSet_builder{
						HostType: "gpu.gb200",
						Size:     5,
					}.Build(),
				},
			}.Build(),
			Status: privatev1.ClusterStatus_builder{
				State: privatev1.ClusterState_CLUSTER_STATE_PROGRESSING,
				Hub:   hubID,
			}.Build(),
		}.Build()

		t := &task{
			r: &function{
				logger:         logger,
				hubCache:       hubCache,
				maskCalculator: nil,
			},
			cluster: cluster,
		}

		err := t.update(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Verify the ClusterOrder was patched with the new size:
		list := &osacv1alpha1.ClusterOrderList{}
		err = fakeClient.List(ctx, list)
		Expect(err).ToNot(HaveOccurred())
		Expect(list.Items).To(HaveLen(1))

		updatedCR := list.Items[0]
		Expect(updatedCR.Spec.NodeRequests).To(HaveLen(1))
		Expect(updatedCR.Spec.NodeRequests[0].ResourceClass).To(Equal("gpu.gb200"))
		Expect(updatedCR.Spec.NodeRequests[0].NumberOfNodes).To(Equal(5))
	})

	It("should not update ClusterOrder when cluster is in failed state", func() {
		// Create an existing ClusterOrder with size 3:
		existingOrder := &osacv1alpha1.ClusterOrder{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "order-abc",
				Namespace: hubNamespace,
				Labels: map[string]string{
					labels.ClusterOrderUuid: clusterID,
				},
				Annotations: map[string]string{
					annotations.Tenant: tenantName,
				},
			},
			Spec: osacv1alpha1.ClusterOrderSpec{
				TemplateID: "test-template",
				NodeRequests: []osacv1alpha1.NodeRequest{
					{
						ResourceClass: "gpu.gb200",
						NumberOfNodes: 3,
					},
				},
			},
		}

		scheme := runtime.NewScheme()
		Expect(osacv1alpha1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(existingOrder).
			Build()

		// No hubCache expectation — the reconciler should return before touching the hub.

		// Create a cluster in FAILED state with updated node set size (5):
		cluster := privatev1.Cluster_builder{
			Id: clusterID,
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{finalizers.Controller},
				Tenant:     tenantName,
			}.Build(),
			Spec: privatev1.ClusterSpec_builder{
				Template: "test-template",
				NodeSets: map[string]*privatev1.ClusterNodeSet{
					"gpu.gb200": privatev1.ClusterNodeSet_builder{
						HostType: "gpu.gb200",
						Size:     5,
					}.Build(),
				},
			}.Build(),
			Status: privatev1.ClusterStatus_builder{
				State: privatev1.ClusterState_CLUSTER_STATE_FAILED,
				Hub:   hubID,
			}.Build(),
		}.Build()

		t := &task{
			r: &function{
				logger:         logger,
				hubCache:       nil,
				maskCalculator: nil,
			},
			cluster: cluster,
		}

		err := t.update(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Verify the ClusterOrder was NOT patched — size should still be 3:
		list := &osacv1alpha1.ClusterOrderList{}
		err = fakeClient.List(ctx, list)
		Expect(err).ToNot(HaveOccurred())
		Expect(list.Items).To(HaveLen(1))

		unchangedCR := list.Items[0]
		Expect(unchangedCR.Spec.NodeRequests).To(HaveLen(1))
		Expect(unchangedCR.Spec.NodeRequests[0].NumberOfNodes).To(Equal(3))
	})

	It("should map explicit cluster fields to ClusterOrder CR spec", func() {
		scheme := runtime.NewScheme()
		Expect(osacv1alpha1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		hubCache := controllers.NewMockHubCache(ctrl)
		hubCache.EXPECT().
			Get(gomock.Any(), hubID).
			Return(&controllers.HubEntry{
				Namespace: hubNamespace,
				Client:    fakeClient,
			}, nil)

		pullSecret := "my-pull-secret"
		sshKey := "ssh-ed25519 AAAA..."
		releaseImage := "quay.io/openshift-release-dev/ocp-release:4.17.0-multi"
		podCIDR := "10.128.0.0/14"
		serviceCIDR := "172.30.0.0/16"

		cluster := privatev1.Cluster_builder{
			Id: clusterID,
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{finalizers.Controller},
				Tenant:     tenantName,
			}.Build(),
			Spec: privatev1.ClusterSpec_builder{
				Template:     "test-template",
				PullSecret:   &pullSecret,
				SshPublicKey: &sshKey,
				ReleaseImage: &releaseImage,
				Network: privatev1.ClusterNetwork_builder{
					PodCidr:     &podCIDR,
					ServiceCidr: &serviceCIDR,
				}.Build(),
			}.Build(),
			Status: privatev1.ClusterStatus_builder{
				State: privatev1.ClusterState_CLUSTER_STATE_PROGRESSING,
				Hub:   hubID,
			}.Build(),
		}.Build()

		t := &task{
			r: &function{
				logger:         logger,
				hubCache:       hubCache,
				maskCalculator: nil,
			},
			cluster: cluster,
		}

		err := t.update(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Verify the ClusterOrder CR spec contains the explicit fields
		list := &osacv1alpha1.ClusterOrderList{}
		err = fakeClient.List(ctx, list)
		Expect(err).ToNot(HaveOccurred())
		Expect(list.Items).To(HaveLen(1))

		createdCR := list.Items[0]
		Expect(createdCR.Spec.PullSecret).To(Equal(pullSecret))
		Expect(createdCR.Spec.SSHPublicKey).To(Equal(sshKey))
		Expect(createdCR.Spec.ReleaseImage).To(Equal(releaseImage))
		Expect(createdCR.Spec.Network).ToNot(BeNil())
		Expect(createdCR.Spec.Network.PodCIDR).To(Equal(podCIDR))
		Expect(createdCR.Spec.Network.ServiceCIDR).To(Equal(serviceCIDR))
	})
})

// newTaskForDelete creates a task configured for testing delete() with hub-dependent paths.
func newTaskForDelete(clusterID, hubID string, hubCache controllers.HubCache) *task {
	cluster := privatev1.Cluster_builder{
		Id: clusterID,
		Metadata: privatev1.Metadata_builder{
			Finalizers: []string{finalizers.Controller},
		}.Build(),
		Status: privatev1.ClusterStatus_builder{
			Hub: hubID,
		}.Build(),
	}.Build()

	f := &function{
		logger:   logger,
		hubCache: hubCache,
	}

	return &task{
		r:       f,
		cluster: cluster,
	}
}

// hasFinalizer checks if the fulfillment-controller finalizer is present on the cluster.
func hasFinalizer(cluster *privatev1.Cluster) bool {
	return slices.Contains(cluster.GetMetadata().GetFinalizers(), finalizers.Controller)
}

var _ = Describe("delete", func() {
	const (
		clusterID = "cluster-delete-id"
		hubID     = "test-hub"
	)

	var (
		ctx  context.Context
		ctrl *gomock.Controller
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		DeferCleanup(ctrl.Finish)
	})

	It("should remove finalizer when hub cache returns ErrHubNotFound", func() {
		// This test verifies the core behavior: when a hub is decommissioned/deleted,
		// the reconciler removes its finalizer to allow the cluster to be archived.
		hubCache := controllers.NewMockHubCache(ctrl)
		hubCache.EXPECT().
			Get(gomock.Any(), hubID).
			Return(nil, controllers.ErrHubNotFound)

		t := newTaskForDelete(clusterID, hubID, hubCache)
		Expect(hasFinalizer(t.cluster)).To(BeTrue())

		err := t.delete(ctx)
		// Should return nil (not propagate the error)
		Expect(err).ToNot(HaveOccurred())
		// Finalizer should be removed to allow archiving
		Expect(hasFinalizer(t.cluster)).To(BeFalse())
	})
})

var _ = Describe("OSAC-455: Hub Persistence Before CR Creation", func() {
	const (
		clusterID    = "osac-455-cluster"
		tenantName   = "test-tenant"
		hubID        = "test-hub-123"
		hubNamespace = "hub-123-ns"
	)

	var (
		ctx  context.Context
		ctrl *gomock.Controller
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		DeferCleanup(ctrl.Finish)
	})

	It("persists hub selection before creating ClusterOrder", func() {
		// Test 1: Hub Persistence Success
		// Verify that hub is persisted to database BEFORE ClusterOrder CR is created

		scheme := runtime.NewScheme()
		Expect(osacv1alpha1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		hubCache := controllers.NewMockHubCache(ctrl)
		hubCache.EXPECT().
			Get(gomock.Any(), hubID).
			Return(&controllers.HubEntry{
				Namespace: hubNamespace,
				Client:    fakeClient,
			}, nil).
			AnyTimes()

		// Mock the hubs list for selectHub
		hubsClient := controllers.NewMockHubsClient(ctrl)
		hubsClient.EXPECT().
			List(gomock.Any(), gomock.Any()).
			Return(&privatev1.HubsListResponse{
				Items: []*privatev1.Hub{
					privatev1.Hub_builder{Id: hubID}.Build(),
				},
			}, nil)

		// Track database update calls - expect two calls:
		// 1. First call: persist hub (field mask: status.hub only)
		// 2. Second call: persist other changes (field mask: status.conditions, status.hub)
		hubPersisted := false
		var firstUpdatedCluster *privatev1.Cluster
		clustersClient := NewMockClustersClient(ctrl)

		// First call: Hub persistence
		clustersClient.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, req *privatev1.ClustersUpdateRequest, opts ...grpc.CallOption) (*privatev1.ClustersUpdateResponse, error) {
				hubPersisted = true
				firstUpdatedCluster = req.GetObject()
				// Verify field mask only includes status.hub
				Expect(req.GetUpdateMask().GetPaths()).To(Equal([]string{"status.hub"}))
				return &privatev1.ClustersUpdateResponse{Object: req.GetObject()}, nil
			}).
			Times(1)

		// Second call: Final update (conditions, etc.)
		clustersClient.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, req *privatev1.ClustersUpdateRequest, opts ...grpc.CallOption) (*privatev1.ClustersUpdateResponse, error) {
				// Verify hub is still set in final update
				Expect(req.GetObject().GetStatus().GetHub()).To(Equal(hubID))
				return &privatev1.ClustersUpdateResponse{Object: req.GetObject()}, nil
			}).
			Times(1)

		cluster := privatev1.Cluster_builder{
			Id: clusterID,
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{finalizers.Controller},
				Tenant:     tenantName,
			}.Build(),
			Spec: privatev1.ClusterSpec_builder{
				Template: "test-template",
			}.Build(),
			Status: privatev1.ClusterStatus_builder{
				State: privatev1.ClusterState_CLUSTER_STATE_PROGRESSING,
				Hub:   "", // Empty - needs hub selection
			}.Build(),
		}.Build()

		f := &function{
			logger:         logger,
			hubCache:       hubCache,
			clustersClient: clustersClient,
			hubsClient:     hubsClient,
			maskCalculator: nil,
		}

		err := f.run(ctx, cluster)
		Expect(err).ToNot(HaveOccurred())

		// Verify hub was persisted to database in first Update call
		Expect(hubPersisted).To(BeTrue(), "Hub should have been persisted in first Update call")
		Expect(firstUpdatedCluster.GetStatus().GetHub()).To(Equal(hubID))

		// Verify ClusterOrder was created
		list := &osacv1alpha1.ClusterOrderList{}
		err = fakeClient.List(ctx, list)
		Expect(err).ToNot(HaveOccurred())
		Expect(list.Items).To(HaveLen(1))
	})

	It("does not create CR if hub persistence fails", func() {
		// Test 2: Hub Persistence Failure (Crash Scenario)
		// Verify that ClusterOrder is NOT created if database update fails

		scheme := runtime.NewScheme()
		Expect(osacv1alpha1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		hubCache := controllers.NewMockHubCache(ctrl)
		hubCache.EXPECT().
			Get(gomock.Any(), hubID).
			Return(&controllers.HubEntry{
				Namespace: hubNamespace,
				Client:    fakeClient,
			}, nil).
			AnyTimes()

		// Mock the hubs list for selectHub
		hubsClient := controllers.NewMockHubsClient(ctrl)
		hubsClient.EXPECT().
			List(gomock.Any(), gomock.Any()).
			Return(&privatev1.HubsListResponse{
				Items: []*privatev1.Hub{
					privatev1.Hub_builder{Id: hubID}.Build(),
				},
			}, nil)

		// Simulate database failure
		clustersClient := NewMockClustersClient(ctrl)
		clustersClient.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("database connection failed"))

		cluster := privatev1.Cluster_builder{
			Id: clusterID,
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{finalizers.Controller},
				Tenant:     tenantName,
			}.Build(),
			Spec: privatev1.ClusterSpec_builder{
				Template: "test-template",
			}.Build(),
			Status: privatev1.ClusterStatus_builder{
				State: privatev1.ClusterState_CLUSTER_STATE_PROGRESSING,
				Hub:   "", // Empty - needs hub selection
			}.Build(),
		}.Build()

		f := &function{
			logger:         logger,
			hubCache:       hubCache,
			clustersClient: clustersClient,
			hubsClient:     hubsClient,
			maskCalculator: nil,
		}

		err := f.run(ctx, cluster)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to persist hub selection"))

		// Verify ClusterOrder was NOT created
		list := &osacv1alpha1.ClusterOrderList{}
		err = fakeClient.List(ctx, list)
		Expect(err).ToNot(HaveOccurred())
		Expect(list.Items).To(BeEmpty(), "ClusterOrder should NOT be created when persistence fails")
	})

	It("skips hub selection if already set", func() {
		// Test 3: Hub Already Set (Idempotency)
		// Verify that hub selection is skipped if status.hub is already set

		scheme := runtime.NewScheme()
		Expect(osacv1alpha1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		hubCache := controllers.NewMockHubCache(ctrl)
		hubCache.EXPECT().
			Get(gomock.Any(), hubID).
			Return(&controllers.HubEntry{
				Namespace: hubNamespace,
				Client:    fakeClient,
			}, nil).
			AnyTimes()

		// Hub list should NOT be called since hub is already set
		hubsClient := controllers.NewMockHubsClient(ctrl)
		// No expectations - List should not be called

		// Database Update may be called for other fields (like conditions), but NOT for hub persistence
		// We verify by checking that any Update calls do NOT have status.hub in the field mask
		clustersClient := NewMockClustersClient(ctrl)
		clustersClient.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, req *privatev1.ClustersUpdateRequest, opts ...grpc.CallOption) (*privatev1.ClustersUpdateResponse, error) {
				// Verify that status.hub is NOT in the field mask (hub already persisted)
				Expect(req.GetUpdateMask().GetPaths()).ToNot(ContainElement("status.hub"))
				return &privatev1.ClustersUpdateResponse{Object: req.GetObject()}, nil
			}).
			AnyTimes()

		cluster := privatev1.Cluster_builder{
			Id: clusterID,
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{finalizers.Controller},
				Tenant:     tenantName,
			}.Build(),
			Spec: privatev1.ClusterSpec_builder{
				Template: "test-template",
			}.Build(),
			Status: privatev1.ClusterStatus_builder{
				State: privatev1.ClusterState_CLUSTER_STATE_PROGRESSING,
				Hub:   hubID, // Already set - should not re-select
			}.Build(),
		}.Build()

		f := &function{
			logger:         logger,
			hubCache:       hubCache,
			clustersClient: clustersClient,
			hubsClient:     hubsClient,
			maskCalculator: nil,
		}

		err := f.run(ctx, cluster)
		Expect(err).ToNot(HaveOccurred())

		// Verify ClusterOrder was created using existing hub
		list := &osacv1alpha1.ClusterOrderList{}
		err = fakeClient.List(ctx, list)
		Expect(err).ToNot(HaveOccurred())
		Expect(list.Items).To(HaveLen(1))
		Expect(list.Items[0].GetNamespace()).To(Equal(hubNamespace))
	})

	It("recovers from crash by using persisted hub", func() {
		// Test 4: Crash Recovery Scenario
		// Simulates: First reconcile creates CR and persists hub, then crashes
		// Second reconcile finds hub already set and uses it (no duplicate)

		scheme := runtime.NewScheme()
		Expect(osacv1alpha1.AddToScheme(scheme)).To(Succeed())

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		hubCache := controllers.NewMockHubCache(ctrl)
		hubCache.EXPECT().
			Get(gomock.Any(), hubID).
			Return(&controllers.HubEntry{
				Namespace: hubNamespace,
				Client:    fakeClient,
			}, nil).
			AnyTimes()

		hubsClient := controllers.NewMockHubsClient(ctrl)
		hubsClient.EXPECT().
			List(gomock.Any(), gomock.Any()).
			Return(&privatev1.HubsListResponse{
				Items: []*privatev1.Hub{
					privatev1.Hub_builder{Id: hubID}.Build(),
				},
			}, nil).
			AnyTimes()

		clustersClient := NewMockClustersClient(ctrl)
		clustersClient.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&privatev1.ClustersUpdateResponse{}, nil).
			AnyTimes()

		// First reconcile: empty hub, needs selection
		cluster1 := privatev1.Cluster_builder{
			Id: clusterID,
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{finalizers.Controller},
				Tenant:     tenantName,
			}.Build(),
			Spec: privatev1.ClusterSpec_builder{
				Template: "test-template",
			}.Build(),
			Status: privatev1.ClusterStatus_builder{
				State: privatev1.ClusterState_CLUSTER_STATE_PROGRESSING,
				Hub:   "", // Empty
			}.Build(),
		}.Build()

		f := &function{
			logger:         logger,
			hubCache:       hubCache,
			clustersClient: clustersClient,
			hubsClient:     hubsClient,
			maskCalculator: nil,
		}

		err := f.run(ctx, cluster1)
		Expect(err).ToNot(HaveOccurred())
		Expect(cluster1.GetStatus().GetHub()).To(Equal(hubID))

		// Verify first CR was created
		list1 := &osacv1alpha1.ClusterOrderList{}
		err = fakeClient.List(ctx, list1)
		Expect(err).ToNot(HaveOccurred())
		Expect(list1.Items).To(HaveLen(1))
		firstCRName := list1.Items[0].GetName()

		// Second reconcile: hub already set (simulating recovery after crash)
		cluster2 := privatev1.Cluster_builder{
			Id: clusterID,
			Metadata: privatev1.Metadata_builder{
				Finalizers: []string{finalizers.Controller},
				Tenant:     tenantName,
			}.Build(),
			Spec: privatev1.ClusterSpec_builder{
				Template: "test-template",
			}.Build(),
			Status: privatev1.ClusterStatus_builder{
				State: privatev1.ClusterState_CLUSTER_STATE_PROGRESSING,
				Hub:   hubID, // Already set from first reconcile
			}.Build(),
		}.Build()

		err = f.run(ctx, cluster2)
		Expect(err).ToNot(HaveOccurred())

		// Verify NO duplicate CR was created
		list2 := &osacv1alpha1.ClusterOrderList{}
		err = fakeClient.List(ctx, list2)
		Expect(err).ToNot(HaveOccurred())
		Expect(list2.Items).To(HaveLen(1), "Should still have exactly 1 ClusterOrder (no duplicate)")
		Expect(list2.Items[0].GetName()).To(Equal(firstCRName), "Should be the same CR, not a new one")
	})
})

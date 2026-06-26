/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package onboarding

import (
	"context"
	"errors"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	osacv1alpha1 "github.com/osac-project/osac-operator/api/v1alpha1"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clnt "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
	"github.com/osac-project/fulfillment-service/internal/controllers"
	"github.com/osac-project/fulfillment-service/internal/controllers/finalizers"
	"github.com/osac-project/fulfillment-service/internal/kubernetes/labels"
	"github.com/osac-project/fulfillment-service/internal/masks"
)

func hasFinalizer(tenant *privatev1.Tenant) bool {
	return slices.Contains(tenant.GetMetadata().GetFinalizers(), finalizers.Controller)
}

func newTenantCR(orgID, namespace, name string, deletionTimestamp *metav1.Time) *osacv1alpha1.Tenant {
	obj := &osacv1alpha1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels: map[string]string{
				labels.TenantUuid: orgID,
			},
		},
	}
	if deletionTimestamp != nil {
		obj.SetDeletionTimestamp(deletionTimestamp)
		obj.SetFinalizers([]string{"osac.openshift.io/tenant"})
	}
	return obj
}

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	Expect(osacv1alpha1.AddToScheme(scheme)).To(Succeed())
	return scheme
}

func newFunction(
	hubCache controllers.HubCache,
	hubsClient privatev1.HubsClient,
	orgClient privatev1.TenantsClient,
) *function {
	return &function{
		logger:         logger,
		hubCache:       hubCache,
		hubsClient:     hubsClient,
		tenantsClient:  orgClient,
		maskCalculator: masks.NewCalculator().Build(),
	}
}

var _ = Describe("addFinalizer", func() {
	It("adds finalizer when not present and creates metadata", func() {
		t := &task{
			tenant: privatev1.Tenant_builder{}.Build(),
		}

		added := t.addFinalizer()

		Expect(added).To(BeTrue())
		Expect(hasFinalizer(t.tenant)).To(BeTrue())
	})

	It("adds finalizer when not present but metadata exists", func() {
		t := &task{
			tenant: privatev1.Tenant_builder{
				Metadata: privatev1.Metadata_builder{
					Finalizers: []string{"other-finalizer"},
				}.Build(),
			}.Build(),
		}

		added := t.addFinalizer()

		Expect(added).To(BeTrue())
		Expect(hasFinalizer(t.tenant)).To(BeTrue())
		Expect(t.tenant.GetMetadata().GetFinalizers()).To(ContainElement("other-finalizer"))
	})

	It("does not add finalizer when already present", func() {
		t := &task{
			tenant: privatev1.Tenant_builder{
				Metadata: privatev1.Metadata_builder{
					Finalizers: []string{finalizers.Controller},
				}.Build(),
			}.Build(),
		}

		added := t.addFinalizer()

		Expect(added).To(BeFalse())
		finalizerList := t.tenant.GetMetadata().GetFinalizers()
		Expect(finalizerList).To(HaveLen(1))
		Expect(finalizerList[0]).To(Equal(finalizers.Controller))
	})
})

var _ = Describe("removeFinalizer", func() {
	It("removes the controller finalizer when present", func() {
		t := &task{
			tenant: privatev1.Tenant_builder{
				Metadata: privatev1.Metadata_builder{
					Finalizers: []string{finalizers.Controller, "other-finalizer"},
				}.Build(),
			}.Build(),
		}

		t.removeFinalizer()

		Expect(hasFinalizer(t.tenant)).To(BeFalse())
		Expect(t.tenant.GetMetadata().GetFinalizers()).To(ConsistOf("other-finalizer"))
	})

	It("does nothing when finalizer is not present", func() {
		t := &task{
			tenant: privatev1.Tenant_builder{
				Metadata: privatev1.Metadata_builder{
					Finalizers: []string{"other-finalizer"},
				}.Build(),
			}.Build(),
		}

		t.removeFinalizer()

		Expect(t.tenant.GetMetadata().GetFinalizers()).To(ConsistOf("other-finalizer"))
	})

	It("does nothing when metadata is missing", func() {
		t := &task{
			tenant: privatev1.Tenant_builder{}.Build(),
		}

		t.removeFinalizer()

		Expect(t.tenant.HasMetadata()).To(BeFalse())
	})
})

var _ = Describe("run", func() {
	const (
		orgID      = "org-123"
		tenantName = "my-tenant"
		hub1ID     = "hub-1"
		hub2ID     = "hub-2"
		namespace1 = "hub-1-ns"
		namespace2 = "hub-2-ns"
	)

	var (
		ctx          context.Context
		ctrl         *gomock.Controller
		mockHubCache *controllers.MockHubCache
		mockHubs     *controllers.MockHubsClient
		mockOrgs     *MockTenantsClient
		scheme       *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		ctrl = gomock.NewController(GinkgoT())
		mockHubCache = controllers.NewMockHubCache(ctrl)
		mockHubs = controllers.NewMockHubsClient(ctrl)
		mockOrgs = NewMockTenantsClient(ctrl)
		scheme = newScheme()
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("update (create/update path)", func() {
		When("tenant has no finalizer", func() {
			It("adds finalizer and returns early", func() {
				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Tenant: tenantName,
					}.Build(),
				}.Build()

				mockOrgs.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, req *privatev1.TenantsUpdateRequest, opts ...grpc.CallOption) (*privatev1.TenantsUpdateResponse, error) {
						return &privatev1.TenantsUpdateResponse{Object: req.GetObject()}, nil
					})

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).ToNot(HaveOccurred())
				Expect(hasFinalizer(org)).To(BeTrue())
			})
		})

		When("tenant is created and no Tenant CRDs exist", func() {
			It("creates Tenant CRD on each hub", func() {
				fakeClient1 := fake.NewClientBuilder().WithScheme(scheme).Build()
				fakeClient2 := fake.NewClientBuilder().WithScheme(scheme).Build()

				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(&privatev1.HubsListResponse{
						Size:  2,
						Total: 2,
						Items: []*privatev1.Hub{
							privatev1.Hub_builder{Id: hub1ID}.Build(),
							privatev1.Hub_builder{Id: hub2ID}.Build(),
						},
					}, nil)

				mockHubCache.EXPECT().
					Get(gomock.Any(), hub1ID).
					Return(&controllers.HubEntry{Namespace: namespace1, Client: fakeClient1}, nil)
				mockHubCache.EXPECT().
					Get(gomock.Any(), hub2ID).
					Return(&controllers.HubEntry{Namespace: namespace2, Client: fakeClient2}, nil)

				mockOrgs.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, req *privatev1.TenantsUpdateRequest, opts ...grpc.CallOption) (*privatev1.TenantsUpdateResponse, error) {
						return &privatev1.TenantsUpdateResponse{Object: req.GetObject()}, nil
					}).AnyTimes()

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Name:       orgID,
						Finalizers: []string{finalizers.Controller},
						Tenant:     tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).ToNot(HaveOccurred())

				list1 := &osacv1alpha1.TenantList{}
				Expect(fakeClient1.List(ctx, list1)).To(Succeed())
				Expect(list1.Items).To(HaveLen(1))
				Expect(list1.Items[0].Labels[labels.TenantUuid]).To(Equal(orgID))
				Expect(list1.Items[0].Namespace).To(Equal(namespace1))

				list2 := &osacv1alpha1.TenantList{}
				Expect(fakeClient2.List(ctx, list2)).To(Succeed())
				Expect(list2.Items).To(HaveLen(1))
				Expect(list2.Items[0].Labels[labels.TenantUuid]).To(Equal(orgID))
				Expect(list2.Items[0].Namespace).To(Equal(namespace2))
			})
		})

		When("Tenant CRD already exists on a hub", func() {
			It("does not create a duplicate", func() {
				existing := newTenantCR(orgID, namespace1, orgID, nil)
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithObjects(existing).
					Build()

				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(&privatev1.HubsListResponse{
						Size:  1,
						Total: 1,
						Items: []*privatev1.Hub{
							privatev1.Hub_builder{Id: hub1ID}.Build(),
						},
					}, nil)

				mockHubCache.EXPECT().
					Get(gomock.Any(), hub1ID).
					Return(&controllers.HubEntry{Namespace: namespace1, Client: fakeClient}, nil)

				mockOrgs.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, req *privatev1.TenantsUpdateRequest, opts ...grpc.CallOption) (*privatev1.TenantsUpdateResponse, error) {
						return &privatev1.TenantsUpdateResponse{Object: req.GetObject()}, nil
					}).AnyTimes()

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Name:       orgID,
						Finalizers: []string{finalizers.Controller},
						Tenant:     tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).ToNot(HaveOccurred())

				list := &osacv1alpha1.TenantList{}
				Expect(fakeClient.List(ctx, list)).To(Succeed())
				Expect(list.Items).To(HaveLen(1))
			})
		})

		When("listing hubs fails", func() {
			It("returns the error", func() {
				expectedErr := errors.New("hubs unavailable")
				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(nil, expectedErr)

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Finalizers: []string{finalizers.Controller},
						Tenant:     tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).To(MatchError(ContainSubstring("hubs unavailable")))
			})
		})

		When("hub cache returns transient error", func() {
			It("returns the error and does not skip the hub", func() {
				expectedErr := errors.New("cache temporarily unavailable")

				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(&privatev1.HubsListResponse{
						Size:  1,
						Total: 1,
						Items: []*privatev1.Hub{
							privatev1.Hub_builder{Id: hub1ID}.Build(),
						},
					}, nil)

				mockHubCache.EXPECT().
					Get(gomock.Any(), hub1ID).
					Return(nil, expectedErr)

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Name:       orgID,
						Finalizers: []string{finalizers.Controller},
						Tenant:     tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).To(MatchError(ContainSubstring("cache temporarily unavailable")))
			})
		})

		When("hub cache returns ErrHubNotFound during update", func() {
			It("skips the decommissioned hub and continues", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(&privatev1.HubsListResponse{
						Size:  2,
						Total: 2,
						Items: []*privatev1.Hub{
							privatev1.Hub_builder{Id: hub1ID}.Build(),
							privatev1.Hub_builder{Id: hub2ID}.Build(),
						},
					}, nil)

				mockHubCache.EXPECT().
					Get(gomock.Any(), hub1ID).
					Return(nil, controllers.ErrHubNotFound)
				mockHubCache.EXPECT().
					Get(gomock.Any(), hub2ID).
					Return(&controllers.HubEntry{Namespace: namespace2, Client: fakeClient}, nil)

				mockOrgs.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, req *privatev1.TenantsUpdateRequest, opts ...grpc.CallOption) (*privatev1.TenantsUpdateResponse, error) {
						return &privatev1.TenantsUpdateResponse{Object: req.GetObject()}, nil
					}).AnyTimes()

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Name:       orgID,
						Finalizers: []string{finalizers.Controller},
						Tenant:     tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).ToNot(HaveOccurred())

				list := &osacv1alpha1.TenantList{}
				Expect(fakeClient.List(ctx, list)).To(Succeed())
				Expect(list.Items).To(HaveLen(1))
				Expect(list.Items[0].Labels[labels.TenantUuid]).To(Equal(orgID))
			})
		})

		When("creating a Tenant CRD on a hub fails", func() {
			It("returns the error", func() {
				expectedErr := errors.New("create failed")
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Create: func(ctx context.Context, client clnt.WithWatch, obj clnt.Object, opts ...clnt.CreateOption) error {
							return expectedErr
						},
					}).
					Build()

				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(&privatev1.HubsListResponse{
						Size:  1,
						Total: 1,
						Items: []*privatev1.Hub{
							privatev1.Hub_builder{Id: hub1ID}.Build(),
						},
					}, nil)

				mockHubCache.EXPECT().
					Get(gomock.Any(), hub1ID).
					Return(&controllers.HubEntry{Namespace: namespace1, Client: fakeClient}, nil)

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Name:       orgID,
						Finalizers: []string{finalizers.Controller},
						Tenant:     tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).To(MatchError(ContainSubstring("create failed")))
			})
		})
	})

	Describe("delete path", func() {
		When("tenant is deleted and Tenant CRD exists on a hub", func() {
			It("issues delete and keeps finalizer until object is gone", func() {
				existing := newTenantCR(orgID, namespace1, orgID, nil)
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(&privatev1.HubsListResponse{
						Size:  1,
						Total: 1,
						Items: []*privatev1.Hub{
							privatev1.Hub_builder{Id: hub1ID}.Build(),
						},
					}, nil)

				mockHubCache.EXPECT().
					Get(gomock.Any(), hub1ID).
					Return(&controllers.HubEntry{Namespace: namespace1, Client: fakeClient}, nil)

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Name:              orgID,
						Finalizers:        []string{finalizers.Controller},
						DeletionTimestamp: timestamppb.Now(),
						Tenant:            tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).ToNot(HaveOccurred())
				Expect(hasFinalizer(org)).To(BeTrue())
			})
		})

		When("no Tenant CRDs exist on any hub", func() {
			It("removes finalizer immediately", func() {
				fakeClient1 := fake.NewClientBuilder().WithScheme(scheme).Build()
				fakeClient2 := fake.NewClientBuilder().WithScheme(scheme).Build()

				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(&privatev1.HubsListResponse{
						Size:  2,
						Total: 2,
						Items: []*privatev1.Hub{
							privatev1.Hub_builder{Id: hub1ID}.Build(),
							privatev1.Hub_builder{Id: hub2ID}.Build(),
						},
					}, nil)

				mockHubCache.EXPECT().
					Get(gomock.Any(), hub1ID).
					Return(&controllers.HubEntry{Namespace: namespace1, Client: fakeClient1}, nil)
				mockHubCache.EXPECT().
					Get(gomock.Any(), hub2ID).
					Return(&controllers.HubEntry{Namespace: namespace2, Client: fakeClient2}, nil)

				mockOrgs.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, req *privatev1.TenantsUpdateRequest, opts ...grpc.CallOption) (*privatev1.TenantsUpdateResponse, error) {
						return &privatev1.TenantsUpdateResponse{Object: req.GetObject()}, nil
					})

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Name:              orgID,
						Finalizers:        []string{finalizers.Controller},
						DeletionTimestamp: timestamppb.Now(),
						Tenant:            tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).ToNot(HaveOccurred())
				Expect(hasFinalizer(org)).To(BeFalse())
			})
		})

		When("Tenant CRD still has a deletion timestamp (K8s finalizers processing)", func() {
			It("keeps the finalizer and waits", func() {
				now := metav1.Now()
				existing := newTenantCR(orgID, namespace1, orgID, &now)
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(&privatev1.HubsListResponse{
						Size:  1,
						Total: 1,
						Items: []*privatev1.Hub{
							privatev1.Hub_builder{Id: hub1ID}.Build(),
						},
					}, nil)

				mockHubCache.EXPECT().
					Get(gomock.Any(), hub1ID).
					Return(&controllers.HubEntry{Namespace: namespace1, Client: fakeClient}, nil)

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Name:              orgID,
						Finalizers:        []string{finalizers.Controller},
						DeletionTimestamp: timestamppb.Now(),
						Tenant:            tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).ToNot(HaveOccurred())
				Expect(hasFinalizer(org)).To(BeTrue())
			})
		})

		When("hub cache returns ErrHubNotFound during delete", func() {
			It("removes finalizer for the decommissioned hub", func() {
				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(&privatev1.HubsListResponse{
						Size:  1,
						Total: 1,
						Items: []*privatev1.Hub{
							privatev1.Hub_builder{Id: hub1ID}.Build(),
						},
					}, nil)

				mockHubCache.EXPECT().
					Get(gomock.Any(), hub1ID).
					Return(nil, controllers.ErrHubNotFound)

				mockOrgs.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any()).
					DoAndReturn(func(ctx context.Context, req *privatev1.TenantsUpdateRequest, opts ...grpc.CallOption) (*privatev1.TenantsUpdateResponse, error) {
						return &privatev1.TenantsUpdateResponse{Object: req.GetObject()}, nil
					})

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Name:              orgID,
						Finalizers:        []string{finalizers.Controller},
						DeletionTimestamp: timestamppb.Now(),
						Tenant:            tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).ToNot(HaveOccurred())
				Expect(hasFinalizer(org)).To(BeFalse())
			})
		})

		When("hub cache returns transient error during delete", func() {
			It("returns the error and keeps finalizer", func() {
				expectedErr := errors.New("cache temporarily unavailable")

				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(&privatev1.HubsListResponse{
						Size:  1,
						Total: 1,
						Items: []*privatev1.Hub{
							privatev1.Hub_builder{Id: hub1ID}.Build(),
						},
					}, nil)

				mockHubCache.EXPECT().
					Get(gomock.Any(), hub1ID).
					Return(nil, expectedErr)

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Name:              orgID,
						Finalizers:        []string{finalizers.Controller},
						DeletionTimestamp: timestamppb.Now(),
						Tenant:            tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).To(MatchError(ContainSubstring("cache temporarily unavailable")))
				Expect(hasFinalizer(org)).To(BeTrue())
			})
		})

		When("listing hubs fails during delete", func() {
			It("returns the error", func() {
				expectedErr := errors.New("hubs unavailable")
				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(nil, expectedErr)

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Finalizers:        []string{finalizers.Controller},
						DeletionTimestamp: timestamppb.Now(),
						Tenant:            tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).To(MatchError(ContainSubstring("hubs unavailable")))
				Expect(hasFinalizer(org)).To(BeTrue())
			})
		})

		When("K8s Get operation fails during delete", func() {
			It("returns the error", func() {
				expectedErr := errors.New("get failed")
				fakeClient := fake.NewClientBuilder().
					WithScheme(scheme).
					WithInterceptorFuncs(interceptor.Funcs{
						Get: func(ctx context.Context, client clnt.WithWatch, key clnt.ObjectKey, obj clnt.Object, opts ...clnt.GetOption) error {
							return expectedErr
						},
					}).
					Build()

				mockHubs.EXPECT().
					List(gomock.Any(), gomock.Any()).
					Return(&privatev1.HubsListResponse{
						Size:  1,
						Total: 1,
						Items: []*privatev1.Hub{
							privatev1.Hub_builder{Id: hub1ID}.Build(),
						},
					}, nil)

				mockHubCache.EXPECT().
					Get(gomock.Any(), hub1ID).
					Return(&controllers.HubEntry{Namespace: namespace1, Client: fakeClient}, nil)

				org := privatev1.Tenant_builder{
					Id: orgID,
					Metadata: privatev1.Metadata_builder{
						Name:              orgID,
						Finalizers:        []string{finalizers.Controller},
						DeletionTimestamp: timestamppb.Now(),
						Tenant:            tenantName,
					}.Build(),
				}.Build()

				f := newFunction(mockHubCache, mockHubs, mockOrgs)
				err := f.run(ctx, org)

				Expect(err).To(MatchError(expectedErr))
				Expect(hasFinalizer(org)).To(BeTrue())
			})
		})
	})
})

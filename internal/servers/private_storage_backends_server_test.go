/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package servers

import (
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
	"github.com/osac-project/fulfillment-service/internal/auth"
)

var _ = Describe("Private storage backends server", func() {
	Describe("Creation", func() {
		It("Can be built if all the required parameters are set", func() {
			server, err := NewPrivateStorageBackendsServer().
				SetLogger(logger).
				SetAttributionLogic(attribution).
				SetTenancyLogic(tenancy).
				Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(server).ToNot(BeNil())
		})

		It("Fails if logger is not set", func() {
			server, err := NewPrivateStorageBackendsServer().
				SetAttributionLogic(attribution).
				SetTenancyLogic(tenancy).
				Build()
			Expect(err).To(MatchError("logger is mandatory"))
			Expect(server).To(BeNil())
		})

		It("Fails if tenancy logic is not set", func() {
			server, err := NewPrivateStorageBackendsServer().
				SetLogger(logger).
				SetAttributionLogic(attribution).
				Build()
			Expect(err).To(MatchError("tenancy logic is mandatory"))
			Expect(server).To(BeNil())
		})
	})

	Describe("Behaviour", func() {
		var server *PrivateStorageBackendsServer

		BeforeEach(func() {
			var err error
			server, err = NewPrivateStorageBackendsServer().
				SetLogger(logger).
				SetAttributionLogic(attribution).
				SetTenancyLogic(tenancy).
				Build()
			Expect(err).ToNot(HaveOccurred())
		})

		createStorageBackend := func() *privatev1.StorageBackend {
			response, err := server.Create(ctx, privatev1.StorageBackendsCreateRequest_builder{
				Object: privatev1.StorageBackend_builder{
					Metadata: privatev1.Metadata_builder{
						Name: "test-backend",
					}.Build(),
					Provider: "vast",
					Endpoint: "https://storage.example.com:8443",
					Credentials: privatev1.StorageBackendCredentials_builder{
						Username: "admin",
						Password: "secret",
					}.Build(),
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			return response.GetObject()
		}

		createStorageBackendWithName := func(name string) *privatev1.StorageBackend {
			response, err := server.Create(ctx, privatev1.StorageBackendsCreateRequest_builder{
				Object: privatev1.StorageBackend_builder{
					Metadata: privatev1.Metadata_builder{
						Name: name,
					}.Build(),
					Provider: "vast",
					Endpoint: "https://storage.example.com:8443",
					Credentials: privatev1.StorageBackendCredentials_builder{
						Username: "admin",
						Password: "secret",
					}.Build(),
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			return response.GetObject()
		}

		It("Creates and gets a storage backend", func() {
			created := createStorageBackend()

			Expect(created.GetId()).ToNot(BeEmpty())
			Expect(created.GetProvider()).To(Equal("vast"))
			Expect(created.GetEndpoint()).To(Equal("https://storage.example.com:8443"))
			Expect(created.GetCredentials().GetUsername()).To(Equal("admin"))
			Expect(created.GetCredentials().GetPassword()).To(Equal("secret"))
			Expect(created.GetStatus().GetState()).To(Equal(
				privatev1.StorageBackendState_STORAGE_BACKEND_STATE_READY))
			Expect(created.GetMetadata().GetTenant()).To(Equal(auth.SharedTenant))

			getResponse, err := server.Get(ctx, privatev1.StorageBackendsGetRequest_builder{
				Id: created.GetId(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			obj := getResponse.GetObject()
			Expect(obj.GetId()).To(Equal(created.GetId()))
			Expect(obj.GetProvider()).To(Equal("vast"))
			Expect(obj.GetEndpoint()).To(Equal("https://storage.example.com:8443"))
			Expect(obj.GetCredentials().GetUsername()).To(Equal("admin"))
		})

		It("List objects", func() {
			const count = 5
			for i := range count {
				createStorageBackendWithName(fmt.Sprintf("backend-%d", i))
			}

			response, err := server.List(ctx, privatev1.StorageBackendsListRequest_builder{}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetItems()).To(HaveLen(count))
		})

		It("List objects with limit", func() {
			const count = 5
			for i := range count {
				createStorageBackendWithName(fmt.Sprintf("backend-%d", i))
			}

			response, err := server.List(ctx, privatev1.StorageBackendsListRequest_builder{
				Limit: new(int32(2)),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetSize()).To(BeNumerically("==", 2))
		})

		It("List objects with offset", func() {
			const count = 5
			for i := range count {
				createStorageBackendWithName(fmt.Sprintf("backend-%d", i))
			}

			response, err := server.List(ctx, privatev1.StorageBackendsListRequest_builder{
				Offset: new(int32(2)),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetSize()).To(BeNumerically("==", count-2))
		})

		It("List objects with filter", func() {
			const count = 3
			var ids []string
			for i := range count {
				obj := createStorageBackendWithName(fmt.Sprintf("backend-%d", i))
				ids = append(ids, obj.GetId())
			}

			for _, id := range ids {
				response, err := server.List(ctx, privatev1.StorageBackendsListRequest_builder{
					Filter: new(fmt.Sprintf("this.id == '%s'", id)),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
				Expect(response.GetSize()).To(BeNumerically("==", 1))
				Expect(response.GetItems()[0].GetId()).To(Equal(id))
			}
		})

		It("List objects with order", func() {
			createStorageBackendWithName("aaa-backend")
			createStorageBackendWithName("zzz-backend")

			response, err := server.List(ctx, privatev1.StorageBackendsListRequest_builder{
				Order: new("metadata.name asc"),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetSize()).To(BeNumerically("==", 2))
			Expect(response.GetItems()[0].GetMetadata().GetName()).To(Equal("aaa-backend"))
			Expect(response.GetItems()[1].GetMetadata().GetName()).To(Equal("zzz-backend"))
		})

		It("Update applies partial changes via field mask", func() {
			created := createStorageBackend()

			updateResponse, err := server.Update(ctx, privatev1.StorageBackendsUpdateRequest_builder{
				Object: privatev1.StorageBackend_builder{
					Id:          created.GetId(),
					Description: "Updated description",
				}.Build(),
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description"}},
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(updateResponse.GetObject().GetDescription()).To(Equal("Updated description"))
			Expect(updateResponse.GetObject().GetProvider()).To(Equal("vast"))
		})

		It("Update endpoint", func() {
			created := createStorageBackend()

			updateResponse, err := server.Update(ctx, privatev1.StorageBackendsUpdateRequest_builder{
				Object: privatev1.StorageBackend_builder{
					Id:       created.GetId(),
					Endpoint: "https://new-storage.example.com:9443",
				}.Build(),
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"endpoint"}},
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(updateResponse.GetObject().GetEndpoint()).To(Equal("https://new-storage.example.com:9443"))
		})

		It("Update credentials", func() {
			created := createStorageBackend()

			updateResponse, err := server.Update(ctx, privatev1.StorageBackendsUpdateRequest_builder{
				Object: privatev1.StorageBackend_builder{
					Id: created.GetId(),
					Credentials: privatev1.StorageBackendCredentials_builder{
						Username: "new-admin",
						Password: "new-secret",
					}.Build(),
				}.Build(),
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"credentials"}},
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(updateResponse.GetObject().GetCredentials().GetUsername()).To(Equal("new-admin"))
			Expect(updateResponse.GetObject().GetCredentials().GetPassword()).To(Equal("new-secret"))
		})

		It("Delete removes the object", func() {
			created := createStorageBackend()

			_, err := server.Delete(ctx, privatev1.StorageBackendsDeleteRequest_builder{
				Id: created.GetId(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			_, err = server.Get(ctx, privatev1.StorageBackendsGetRequest_builder{
				Id: created.GetId(),
			}.Build())
			Expect(err).To(HaveOccurred())
			st, ok := status.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(codes.NotFound))
		})

		It("Generates UUID for id ignoring caller-provided value", func() {
			callerProvidedId := "my-custom-id"
			response, err := server.Create(ctx, privatev1.StorageBackendsCreateRequest_builder{
				Object: privatev1.StorageBackend_builder{
					Id: callerProvidedId,
					Metadata: privatev1.Metadata_builder{
						Name: "test-backend",
					}.Build(),
					Provider: "vast",
					Endpoint: "https://storage.example.com:8443",
					Credentials: privatev1.StorageBackendCredentials_builder{
						Username: "admin",
						Password: "secret",
					}.Build(),
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetObject().GetId()).ToNot(Equal(callerProvidedId))
			_, err = uuid.Parse(response.GetObject().GetId())
			Expect(err).ToNot(HaveOccurred())
		})

		It("Create always sets state to READY regardless of caller-provided state", func() {
			response, err := server.Create(ctx, privatev1.StorageBackendsCreateRequest_builder{
				Object: privatev1.StorageBackend_builder{
					Metadata: privatev1.Metadata_builder{
						Name: "test-backend",
					}.Build(),
					Provider: "vast",
					Endpoint: "https://storage.example.com:8443",
					Credentials: privatev1.StorageBackendCredentials_builder{
						Username: "admin",
						Password: "secret",
					}.Build(),
					Status: privatev1.StorageBackendStatus_builder{
						State: privatev1.StorageBackendState_STORAGE_BACKEND_STATE_UNSPECIFIED,
					}.Build(),
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetObject().GetStatus().GetState()).To(Equal(
				privatev1.StorageBackendState_STORAGE_BACKEND_STATE_READY))
		})

		It("Create forces tenant to shared", func() {
			response, err := server.Create(ctx, privatev1.StorageBackendsCreateRequest_builder{
				Object: privatev1.StorageBackend_builder{
					Metadata: privatev1.Metadata_builder{
						Name:   "test-backend",
						Tenant: "some-other-tenant",
					}.Build(),
					Provider: "vast",
					Endpoint: "https://storage.example.com:8443",
					Credentials: privatev1.StorageBackendCredentials_builder{
						Username: "admin",
						Password: "secret",
					}.Build(),
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetObject().GetMetadata().GetTenant()).To(Equal(auth.SharedTenant))
		})

		Describe("Validation", func() {
			It("Create without provider fails", func() {
				_, err := server.Create(ctx, privatev1.StorageBackendsCreateRequest_builder{
					Object: privatev1.StorageBackend_builder{
						Metadata: privatev1.Metadata_builder{
							Name: "test-backend",
						}.Build(),
						Endpoint: "https://storage.example.com:8443",
						Credentials: privatev1.StorageBackendCredentials_builder{
							Username: "admin",
							Password: "secret",
						}.Build(),
					}.Build(),
				}.Build())
				Expect(err).To(HaveOccurred())
				st, ok := status.FromError(err)
				Expect(ok).To(BeTrue())
				Expect(st.Code()).To(Equal(codes.InvalidArgument))
				Expect(st.Message()).To(ContainSubstring("provider"))
			})

			It("Create without endpoint fails", func() {
				_, err := server.Create(ctx, privatev1.StorageBackendsCreateRequest_builder{
					Object: privatev1.StorageBackend_builder{
						Metadata: privatev1.Metadata_builder{
							Name: "test-backend",
						}.Build(),
						Provider: "vast",
						Credentials: privatev1.StorageBackendCredentials_builder{
							Username: "admin",
							Password: "secret",
						}.Build(),
					}.Build(),
				}.Build())
				Expect(err).To(HaveOccurred())
				st, ok := status.FromError(err)
				Expect(ok).To(BeTrue())
				Expect(st.Code()).To(Equal(codes.InvalidArgument))
				Expect(st.Message()).To(ContainSubstring("endpoint"))
			})

			It("Create without credentials username fails", func() {
				_, err := server.Create(ctx, privatev1.StorageBackendsCreateRequest_builder{
					Object: privatev1.StorageBackend_builder{
						Metadata: privatev1.Metadata_builder{
							Name: "test-backend",
						}.Build(),
						Provider: "vast",
						Endpoint: "https://storage.example.com:8443",
						Credentials: privatev1.StorageBackendCredentials_builder{
							Password: "secret",
						}.Build(),
					}.Build(),
				}.Build())
				Expect(err).To(HaveOccurred())
				st, ok := status.FromError(err)
				Expect(ok).To(BeTrue())
				Expect(st.Code()).To(Equal(codes.InvalidArgument))
				Expect(st.Message()).To(ContainSubstring("credentials.username"))
			})

			It("Create without credentials password fails", func() {
				_, err := server.Create(ctx, privatev1.StorageBackendsCreateRequest_builder{
					Object: privatev1.StorageBackend_builder{
						Metadata: privatev1.Metadata_builder{
							Name: "test-backend",
						}.Build(),
						Provider: "vast",
						Endpoint: "https://storage.example.com:8443",
						Credentials: privatev1.StorageBackendCredentials_builder{
							Username: "admin",
						}.Build(),
					}.Build(),
				}.Build())
				Expect(err).To(HaveOccurred())
				st, ok := status.FromError(err)
				Expect(ok).To(BeTrue())
				Expect(st.Code()).To(Equal(codes.InvalidArgument))
				Expect(st.Message()).To(ContainSubstring("credentials.password"))
			})

			It("Create without credentials fails", func() {
				_, err := server.Create(ctx, privatev1.StorageBackendsCreateRequest_builder{
					Object: privatev1.StorageBackend_builder{
						Metadata: privatev1.Metadata_builder{
							Name: "test-backend",
						}.Build(),
						Provider: "vast",
						Endpoint: "https://storage.example.com:8443",
					}.Build(),
				}.Build())
				Expect(err).To(HaveOccurred())
				st, ok := status.FromError(err)
				Expect(ok).To(BeTrue())
				Expect(st.Code()).To(Equal(codes.InvalidArgument))
				Expect(st.Message()).To(ContainSubstring("credentials.username"))
			})
		})

		Describe("Immutability", func() {
			It("Update changing provider fails", func() {
				created := createStorageBackend()

				_, err := server.Update(ctx, privatev1.StorageBackendsUpdateRequest_builder{
					Object: privatev1.StorageBackend_builder{
						Id:       created.GetId(),
						Provider: "ceph",
					}.Build(),
					UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"provider"}},
				}.Build())
				Expect(err).To(HaveOccurred())
				st, ok := status.FromError(err)
				Expect(ok).To(BeTrue())
				Expect(st.Code()).To(Equal(codes.InvalidArgument))
				Expect(st.Message()).To(ContainSubstring("provider"))
				Expect(st.Message()).To(ContainSubstring("immutable"))
			})

			It("Update changing metadata.name fails at DB level", func() {
				created := createStorageBackend()

				_, err := server.Update(ctx, privatev1.StorageBackendsUpdateRequest_builder{
					Object: privatev1.StorageBackend_builder{
						Id: created.GetId(),
						Metadata: privatev1.Metadata_builder{
							Name: "new-name",
						}.Build(),
					}.Build(),
				}.Build())
				Expect(err).To(HaveOccurred())
			})
		})

		Describe("Name uniqueness", func() {
			It("Create with duplicate active name fails", func() {
				createStorageBackendWithName("unique-name")

				_, err := server.Create(ctx, privatev1.StorageBackendsCreateRequest_builder{
					Object: privatev1.StorageBackend_builder{
						Metadata: privatev1.Metadata_builder{
							Name: "unique-name",
						}.Build(),
						Provider: "ceph",
						Endpoint: "https://other.example.com:8443",
						Credentials: privatev1.StorageBackendCredentials_builder{
							Username: "admin",
							Password: "secret",
						}.Build(),
					}.Build(),
				}.Build())
				Expect(err).To(HaveOccurred())
				st, ok := status.FromError(err)
				Expect(ok).To(BeTrue())
				Expect(st.Code()).To(Equal(codes.AlreadyExists))
			})

			It("Create after delete of same name succeeds", func() {
				created := createStorageBackendWithName("reusable-name")

				_, err := server.Delete(ctx, privatev1.StorageBackendsDeleteRequest_builder{
					Id: created.GetId(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())

				second := createStorageBackendWithName("reusable-name")
				Expect(second.GetId()).ToNot(Equal(created.GetId()))
				Expect(second.GetMetadata().GetName()).To(Equal("reusable-name"))
			})
		})

		Describe("Optimistic locking", func() {
			It("Update with stale version and lock=true fails", func() {
				created := createStorageBackend()

				// First update succeeds:
				_, err := server.Update(ctx, privatev1.StorageBackendsUpdateRequest_builder{
					Object: privatev1.StorageBackend_builder{
						Id:          created.GetId(),
						Description: "first update",
					}.Build(),
					UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"description"}},
					Lock:       true,
				}.Build())
				Expect(err).ToNot(HaveOccurred())

				// Second update with the original version fails (version is now stale):
				_, err = server.Update(ctx, privatev1.StorageBackendsUpdateRequest_builder{
					Object: privatev1.StorageBackend_builder{
						Id:          created.GetId(),
						Description: "second update",
						Metadata: privatev1.Metadata_builder{
							Version: created.GetMetadata().GetVersion(),
						}.Build(),
					}.Build(),
					Lock: true,
				}.Build())
				Expect(err).To(HaveOccurred())
			})
		})

		It("Update without id fails", func() {
			_, err := server.Update(ctx, privatev1.StorageBackendsUpdateRequest_builder{
				Object: privatev1.StorageBackend_builder{
					Description: "updated",
				}.Build(),
			}.Build())
			Expect(err).To(HaveOccurred())
			st, ok := status.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(codes.InvalidArgument))
			Expect(st.Message()).To(ContainSubstring("identifier"))
		})
	})
})

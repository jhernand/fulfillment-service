/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package servers

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	publicv1 "github.com/osac-project/fulfillment-service/internal/api/osac/public/v1"
	"github.com/osac-project/fulfillment-service/internal/auth"
	"github.com/osac-project/fulfillment-service/internal/database"
	"github.com/osac-project/fulfillment-service/internal/database/dao"
)

var _ = Describe("Cluster templates server", func() {
	var (
		ctx context.Context
		tx  database.Tx
	)

	BeforeEach(func() {
		var err error

		// Create a context:
		ctx = context.Background()

		// Prepare the database pool:
		db := server.MakeDatabase()
		DeferCleanup(db.Close)
		pool, err := pgxpool.New(ctx, db.MakeURL())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(pool.Close)

		// Create the transaction manager:
		tm, err := database.NewTxManager().
			SetLogger(logger).
			SetPool(pool).
			Build()
		Expect(err).ToNot(HaveOccurred())

		// Start a transaction and add it to the context:
		tx, err = tm.Begin(ctx)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			err := tm.End(ctx, tx)
			Expect(err).ToNot(HaveOccurred())
		})
		ctx = database.TxIntoContext(ctx, tx)

		// Create the tables:
		err = dao.CreateTables[*publicv1.ClusterTemplate](ctx)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Creation", func() {
		It("Can be built if all the required parameters are set", func() {
			server, err := NewClusterTemplatesServer().
				SetLogger(logger).
				SetAttributionLogic(attribution).
				SetTenancyLogic(tenancy).
				Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(server).ToNot(BeNil())
		})

		It("Fails if logger is not set", func() {
			server, err := NewClusterTemplatesServer().
				SetAttributionLogic(attribution).
				SetTenancyLogic(tenancy).
				Build()
			Expect(err).To(MatchError("logger is mandatory"))
			Expect(server).To(BeNil())
		})

		It("Fails if attribution logic is not set", func() {
			server, err := NewClusterTemplatesServer().
				SetLogger(logger).
				SetTenancyLogic(tenancy).
				Build()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("attribution logic is mandatory"))
			Expect(server).To(BeNil())
		})

		It("Fails if tenancy logic is not set", func() {
			server, err := NewClusterTemplatesServer().
				SetLogger(logger).
				SetAttributionLogic(attribution).
				Build()
			Expect(err).To(MatchError("tenancy logic is mandatory"))
			Expect(server).To(BeNil())
		})
	})

	Describe("Behaviour", func() {
		var server *ClusterTemplatesServer

		BeforeEach(func() {
			var err error

			// Create the server:
			server, err = NewClusterTemplatesServer().
				SetLogger(logger).
				SetAttributionLogic(attribution).
				SetTenancyLogic(tenancy).
				Build()
			Expect(err).ToNot(HaveOccurred())
		})

		It("Creates object", func() {
			response, err := server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:          "my_template",
					Title:       "My template",
					Description: "My template is *nice*",
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response).ToNot(BeNil())
			object := response.GetObject()
			Expect(object).ToNot(BeNil())
			Expect(object.GetId()).To(Equal("my_template"))
		})

		It("List objects", func() {
			// Create a few objects:
			const count = 10
			for i := range count {
				_, err := server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
					Object: publicv1.ClusterTemplate_builder{
						Id:          fmt.Sprintf("my_template_%d", i),
						Title:       fmt.Sprintf("My template %d", i),
						Description: fmt.Sprintf("My template is *nice* %d", i),
					}.Build(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
			}

			// List the objects:
			response, err := server.List(ctx, publicv1.ClusterTemplatesListRequest_builder{}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response).ToNot(BeNil())
			items := response.GetItems()
			Expect(items).To(HaveLen(count))
		})

		It("List objects with limit", func() {
			// Create a few objects:
			const count = 10
			for i := range count {
				_, err := server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
					Object: publicv1.ClusterTemplate_builder{
						Id:          fmt.Sprintf("my_template_%d", i),
						Title:       fmt.Sprintf("My template %d", i),
						Description: fmt.Sprintf("My template is *nice* %d", i),
					}.Build(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
			}

			// List the objects:
			response, err := server.List(ctx, publicv1.ClusterTemplatesListRequest_builder{
				Limit: proto.Int32(1),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetSize()).To(BeNumerically("==", 1))
		})

		It("List objects with offset", func() {
			// Create a few objects:
			const count = 10
			for i := range count {
				_, err := server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
					Object: publicv1.ClusterTemplate_builder{
						Id:          fmt.Sprintf("my_template_%d", i),
						Title:       fmt.Sprintf("My template %d", i),
						Description: fmt.Sprintf("My template is *nice* %d", i),
					}.Build(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
			}

			// List the objects:
			response, err := server.List(ctx, publicv1.ClusterTemplatesListRequest_builder{
				Offset: proto.Int32(1),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetSize()).To(BeNumerically("==", count-1))
		})

		It("List objects with filter", func() {
			// Create a few objects:
			const count = 10
			var objects []*publicv1.ClusterTemplate
			for i := range count {
				response, err := server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
					Object: publicv1.ClusterTemplate_builder{
						Id:          fmt.Sprintf("my_template_%d", i),
						Title:       fmt.Sprintf("My template %d", i),
						Description: fmt.Sprintf("My template is *nice* %d", i),
					}.Build(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
				objects = append(objects, response.GetObject())
			}

			// List the objects:
			for _, object := range objects {
				response, err := server.List(ctx, publicv1.ClusterTemplatesListRequest_builder{
					Filter: proto.String(fmt.Sprintf("this.id == '%s'", object.GetId())),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
				Expect(response.GetSize()).To(BeNumerically("==", 1))
				Expect(response.GetItems()[0].GetId()).To(Equal(object.GetId()))
			}
		})

		It("Get object", func() {
			// Create the object:
			createResponse, err := server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:          "my_template",
					Title:       "My template",
					Description: "My template is *nice*",
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Get it:
			getResponse, err := server.Get(ctx, publicv1.ClusterTemplatesGetRequest_builder{
				Id: createResponse.GetObject().GetId(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(proto.Equal(createResponse.GetObject(), getResponse.GetObject())).To(BeTrue())
		})

		It("Update object", func() {
			// Create the object:
			createResponse, err := server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:          "my_template",
					Title:       "My template",
					Description: "My template is *nice*",
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			object := createResponse.GetObject()

			// Update the object:
			updateResponse, err := server.Update(ctx, publicv1.ClusterTemplatesUpdateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:          object.GetId(),
					Title:       "Still my template",
					Description: "But now it is _ugly_",
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(updateResponse.GetObject().GetTitle()).To(Equal("Still my template"))
			Expect(updateResponse.GetObject().GetDescription()).To(Equal("But now it is _ugly_"))

			// Get and verify:
			getResponse, err := server.Get(ctx, publicv1.ClusterTemplatesGetRequest_builder{
				Id: object.GetId(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(getResponse.GetObject().GetTitle()).To(Equal("Still my template"))
			Expect(getResponse.GetObject().GetDescription()).To(Equal("But now it is _ugly_"))
		})

		It("Delete object", func() {
			// Create the object:
			createResponse, err := server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:          "my_template",
					Title:       "My template",
					Description: "My template is *nice*",
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			object := createResponse.GetObject()

			// Add a finalizer, as otherwise the object will be immediatelly deleted and archived and it
			// won't be possible to verify the deletion timestamp. This can't be done using the server
			// because this is a public object, and public objects don't have the finalizers field.
			_, err = tx.Exec(
				ctx,
				`update cluster_templates set finalizers = '{"a"}' where id = $1`,
				object.GetId(),
			)
			Expect(err).ToNot(HaveOccurred())

			// Delete the object:
			_, err = server.Delete(ctx, publicv1.ClusterTemplatesDeleteRequest_builder{
				Id: object.GetId(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Get and verify:
			getResponse, err := server.Get(ctx, publicv1.ClusterTemplatesGetRequest_builder{
				Id: object.GetId(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			object = getResponse.GetObject()
			Expect(object.GetMetadata().GetDeletionTimestamp()).ToNot(BeNil())
		})
	})

	Describe("Template inheritance", func() {
		var server *ClusterTemplatesServer

		BeforeEach(func() {
			var err error

			server, err = NewClusterTemplatesServer().
				SetLogger(logger).
				SetAttributionLogic(attribution).
				SetTenancyLogic(tenancy).
				Build()
			Expect(err).ToNot(HaveOccurred())
		})

		It("Rejects creating a template with a non-existent parent", func() {
			_, err := server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:     "child_template",
					Title:  "Child",
					Parent: proto.String("non_existent_parent"),
				}.Build(),
			}.Build())
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Message()).To(ContainSubstring("non_existent_parent"))
		})

		It("Rejects creating a template with parameters not in the parent", func() {
			// Create the parent template with one parameter:
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())
			_, err = server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:    "parent_template",
					Title: "Parent",
					Parameters: []*publicv1.ClusterTemplateParameterDefinition{
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:    "version",
							Title:   "Version",
							Type:    "type.googleapis.com/google.protobuf.Int32Value",
							Default: versionDefault,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Attempt to create a child with a parameter that doesn't exist in the parent:
			_, err = server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:     "child_template",
					Title:  "Child",
					Parent: proto.String("parent_template"),
					Parameters: []*publicv1.ClusterTemplateParameterDefinition{
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:   "bogus_param",
							Sealed: true,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Message()).To(ContainSubstring("bogus_param"))
			Expect(st.Message()).To(ContainSubstring("does not exist in parent"))
		})

		It("Rejects creating a template with parameters not in a grandparent (recursive)", func() {
			// Create the root template:
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())
			_, err = server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:    "root_template",
					Title: "Root",
					Parameters: []*publicv1.ClusterTemplateParameterDefinition{
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:    "version",
							Title:   "Version",
							Type:    "type.googleapis.com/google.protobuf.Int32Value",
							Default: versionDefault,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Create a child template:
			childDefault, err := anypb.New(wrapperspb.Int32(43))
			Expect(err).ToNot(HaveOccurred())
			_, err = server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:     "child_template",
					Title:  "Child",
					Parent: proto.String("root_template"),
					Parameters: []*publicv1.ClusterTemplateParameterDefinition{
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:    "version",
							Default: childDefault,
							Sealed:  true,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Try creating a grandchild with a parameter that doesn't exist:
			_, err = server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:     "grandchild_template",
					Title:  "Grandchild",
					Parent: proto.String("child_template"),
					Parameters: []*publicv1.ClusterTemplateParameterDefinition{
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:   "nonexistent_param",
							Sealed: true,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Message()).To(ContainSubstring("nonexistent_param"))
			Expect(st.Message()).To(ContainSubstring("does not exist in parent"))
		})

		It("Rejects overriding a sealed parameter in a grandchild", func() {
			// Create the root:
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())
			_, err = server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:    "root_template",
					Title: "Root",
					Parameters: []*publicv1.ClusterTemplateParameterDefinition{
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:    "version",
							Title:   "Version",
							Type:    "type.googleapis.com/google.protobuf.Int32Value",
							Default: versionDefault,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Create a child that seals the parameter:
			childDefault, err := anypb.New(wrapperspb.Int32(43))
			Expect(err).ToNot(HaveOccurred())
			_, err = server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:     "child_template",
					Title:  "Child",
					Parent: proto.String("root_template"),
					Parameters: []*publicv1.ClusterTemplateParameterDefinition{
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:    "version",
							Default: childDefault,
							Sealed:  true,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Try to override the sealed parameter in a grandchild:
			grandchildDefault, err := anypb.New(wrapperspb.Int32(44))
			Expect(err).ToNot(HaveOccurred())
			_, err = server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:     "grandchild_template",
					Title:  "Grandchild",
					Parent: proto.String("child_template"),
					Parameters: []*publicv1.ClusterTemplateParameterDefinition{
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:    "version",
							Default: grandchildDefault,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Message()).To(ContainSubstring("sealed"))
		})

		It("Rejects creating a template using a parent that the user cannot see", func() {
			// Create the parent using the system server (all tenants visible):
			_, err := server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:    "hidden_parent",
					Title: "Hidden parent",
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Change the parent's tenant to something the guest server can't see:
			_, err = tx.Exec(ctx,
				`UPDATE cluster_templates SET tenants = '{"tenant_a"}' WHERE id = 'hidden_parent'`)
			Expect(err).ToNot(HaveOccurred())

			// Create a server with guest tenancy which can only see "guest" and "shared":
			guestTenancy, err := auth.NewGuestTenancyLogic().
				SetLogger(logger).
				Build()
			Expect(err).ToNot(HaveOccurred())
			guestServer, err := NewClusterTemplatesServer().
				SetLogger(logger).
				SetAttributionLogic(attribution).
				SetTenancyLogic(guestTenancy).
				Build()
			Expect(err).ToNot(HaveOccurred())

			// Attempt to create a child referencing the invisible parent:
			_, err = guestServer.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:     "child_of_hidden",
					Title:  "Child",
					Parent: proto.String("hidden_parent"),
				}.Build(),
			}.Build())
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Message()).To(ContainSubstring("hidden_parent"))
		})

		It("Creates a child template with valid parameters", func() {
			// Create the parent template:
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())
			selinuxDefault, err := anypb.New(wrapperspb.String("enabled"))
			Expect(err).ToNot(HaveOccurred())
			_, err = server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:    "parent_template",
					Title: "Parent",
					Parameters: []*publicv1.ClusterTemplateParameterDefinition{
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:    "version",
							Title:   "Version",
							Type:    "type.googleapis.com/google.protobuf.Int32Value",
							Default: versionDefault,
						}.Build(),
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:    "selinux",
							Title:   "SELinux",
							Type:    "type.googleapis.com/google.protobuf.StringValue",
							Default: selinuxDefault,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Create the child template that overrides version:
			childDefault, err := anypb.New(wrapperspb.Int32(43))
			Expect(err).ToNot(HaveOccurred())
			createResponse, err := server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:     "child_template",
					Title:  "Child",
					Parent: proto.String("parent_template"),
					Parameters: []*publicv1.ClusterTemplateParameterDefinition{
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:    "version",
							Default: childDefault,
							Sealed:  true,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(createResponse.GetObject().HasParent()).To(BeTrue())
		})

		It("Rejects deleting a template that is used as a parent", func() {
			// Create the parent template:
			_, err := server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:    "parent_template",
					Title: "Parent",
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Create a child referencing the parent:
			_, err = server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:     "child_template",
					Title:  "Child",
					Parent: proto.String("parent_template"),
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Try to delete the parent:
			_, err = server.Delete(ctx, publicv1.ClusterTemplatesDeleteRequest_builder{
				Id: "parent_template",
			}.Build())
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Message()).To(ContainSubstring("parent_template"))
			Expect(st.Message()).To(ContainSubstring("child_template"))
		})

		It("Returns flattened view with inherited parameters", func() {
			// Create the parent template:
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())
			selinuxDefault, err := anypb.New(wrapperspb.String("enabled"))
			Expect(err).ToNot(HaveOccurred())
			_, err = server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:    "parent_template",
					Title: "Parent",
					Parameters: []*publicv1.ClusterTemplateParameterDefinition{
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:    "version",
							Title:   "Version",
							Type:    "type.googleapis.com/google.protobuf.Int32Value",
							Default: versionDefault,
						}.Build(),
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:    "selinux",
							Title:   "SELinux",
							Type:    "type.googleapis.com/google.protobuf.StringValue",
							Default: selinuxDefault,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Create the child template:
			childDefault, err := anypb.New(wrapperspb.Int32(43))
			Expect(err).ToNot(HaveOccurred())
			_, err = server.Create(ctx, publicv1.ClusterTemplatesCreateRequest_builder{
				Object: publicv1.ClusterTemplate_builder{
					Id:     "child_template",
					Title:  "Child",
					Parent: proto.String("parent_template"),
					Parameters: []*publicv1.ClusterTemplateParameterDefinition{
						publicv1.ClusterTemplateParameterDefinition_builder{
							Name:    "version",
							Default: childDefault,
							Sealed:  true,
						}.Build(),
					},
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Get the child with flatten=true:
			getResponse, err := server.Get(ctx, publicv1.ClusterTemplatesGetRequest_builder{
				Id:      "child_template",
				Flatten: proto.Bool(true),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			flattenedParams := getResponse.GetObject().GetParameters()
			Expect(flattenedParams).To(HaveLen(2))

			paramsByName := map[string]*publicv1.ClusterTemplateParameterDefinition{}
			for _, p := range flattenedParams {
				paramsByName[p.GetName()] = p
			}
			Expect(paramsByName).To(HaveKey("version"))
			Expect(paramsByName).To(HaveKey("selinux"))
			Expect(paramsByName["version"].GetSealed()).To(BeTrue())
			Expect(paramsByName["selinux"].GetSealed()).To(BeFalse())
		})
	})
})

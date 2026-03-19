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
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	grpccodes "google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/osac-project/fulfillment-service/internal/annotations"
	privatev1 "github.com/osac-project/fulfillment-service/internal/api/osac/private/v1"
	"github.com/osac-project/fulfillment-service/internal/database"
	"github.com/osac-project/fulfillment-service/internal/database/dao"
)

var _ = Describe("Template helper", func() {
	var (
		ctx          context.Context
		helper       *TemplateHelper[*privatev1.ClusterTemplate, *privatev1.ClusterTemplateParameterDefinition]
		templatesDao *dao.GenericDAO[*privatev1.ClusterTemplate]
	)

	BeforeEach(func() {
		var err error

		ctx = context.Background()

		db := server.MakeDatabase()
		DeferCleanup(db.Close)
		pool, err := pgxpool.New(ctx, db.MakeURL())
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(pool.Close)

		tm, err := database.NewTxManager().
			SetLogger(logger).
			SetPool(pool).
			Build()
		Expect(err).ToNot(HaveOccurred())

		tx, err := tm.Begin(ctx)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			err := tm.End(ctx, tx)
			Expect(err).ToNot(HaveOccurred())
		})
		ctx = database.TxIntoContext(ctx, tx)

		err = dao.CreateTables[*privatev1.ClusterTemplate](ctx)
		Expect(err).ToNot(HaveOccurred())

		templatesDao, err = dao.NewGenericDAO[*privatev1.ClusterTemplate]().
			SetLogger(logger).
			SetAttributionLogic(attribution).
			SetTenancyLogic(tenancy).
			Build()
		Expect(err).ToNot(HaveOccurred())

		helper, err = NewTemplateHelper[*privatev1.ClusterTemplate]().
			SetLogger(logger).
			SetDao(templatesDao).
			Build()
		Expect(err).ToNot(HaveOccurred())
	})

	// createTemplate is a small shortcut used by the tests to insert a template via the DAO.
	createTemplate := func(template *privatev1.ClusterTemplate) {
		_, err := templatesDao.Create().
			SetObject(template).
			Do(ctx)
		ExpectWithOffset(1, err).ToNot(HaveOccurred())
	}

	Describe("Builder", func() {
		It("Builds successfully with all required parameters", func() {
			helper, err := NewTemplateHelper[*privatev1.ClusterTemplate]().
				SetLogger(logger).
				SetDao(templatesDao).
				Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(helper).ToNot(BeNil())
		})

		It("Fails if logger is not set", func() {
			helper, err := NewTemplateHelper[*privatev1.ClusterTemplate]().
				SetDao(templatesDao).
				Build()
			Expect(err).To(MatchError("logger is mandatory"))
			Expect(helper).To(BeNil())
		})
	})

	Describe("Validate creation", func() {
		It("Accepts a root template without parent", func() {
			template := privatev1.ClusterTemplate_builder{
				Id:    "root",
				Title: "Root",
			}.Build()
			err := helper.ValidateCreation(ctx, template)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Rejects a template with parent set to the empty string", func() {
			template := privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String(""),
			}.Build()
			err := helper.ValidateCreation(ctx, template)
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(grpccodes.InvalidArgument))
			Expect(st.Message()).To(ContainSubstring("parent must not be empty"))
		})

		It("Rejects a child referencing an ambiguous parent name", func() {
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "parent_a",
				Title: "Parent A",
				Metadata: privatev1.Metadata_builder{
					Name: "shared-name",
				}.Build(),
			}.Build())
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "parent_b",
				Title: "Parent B",
				Metadata: privatev1.Metadata_builder{
					Name: "shared-name",
				}.Build(),
			}.Build())

			child := privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("shared-name"),
			}.Build()
			err := helper.ValidateCreation(ctx, child)
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(grpccodes.InvalidArgument))
			Expect(st.Message()).To(ContainSubstring("multiple"))
			Expect(st.Message()).To(ContainSubstring("shared-name"))
		})

		It("Rejects a child referencing a non-existent parent", func() {
			template := privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("does_not_exist"),
			}.Build()
			err := helper.ValidateCreation(ctx, template)
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(grpccodes.NotFound))
			Expect(st.Message()).To(ContainSubstring("does_not_exist"))
		})

		It("Rejects a child with a parameter that doesn't exist in the parent", func() {
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "parent",
				Title: "Parent",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: versionDefault,
					}.Build(),
				},
			}.Build())

			child := privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("parent"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:   "bogus",
						Sealed: true,
					}.Build(),
				},
			}.Build()

			err = helper.ValidateCreation(ctx, child)
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(grpccodes.InvalidArgument))
			Expect(st.Message()).To(ContainSubstring("bogus"))
			Expect(st.Message()).To(ContainSubstring("does not exist in parent"))
		})

		It("Rejects overriding a sealed parameter", func() {
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "root",
				Title: "Root",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: versionDefault,
					}.Build(),
				},
			}.Build())

			childDefault, err := anypb.New(wrapperspb.Int32(43))
			Expect(err).ToNot(HaveOccurred())
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("root"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Default: childDefault,
						Sealed:  true,
					}.Build(),
				},
			}.Build())

			grandchild := privatev1.ClusterTemplate_builder{
				Id:     "grandchild",
				Title:  "Grandchild",
				Parent: proto.String("child"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:   "version",
						Sealed: true,
					}.Build(),
				},
			}.Build()

			err = helper.ValidateCreation(ctx, grandchild)
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(grpccodes.InvalidArgument))
			Expect(st.Message()).To(ContainSubstring("sealed"))
		})

		It("Rejects a child with parameters not in a grandparent (recursive check)", func() {
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "root",
				Title: "Root",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: versionDefault,
					}.Build(),
				},
			}.Build())

			childDefault, err := anypb.New(wrapperspb.Int32(43))
			Expect(err).ToNot(HaveOccurred())
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("root"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Default: childDefault,
					}.Build(),
				},
			}.Build())

			grandchild := privatev1.ClusterTemplate_builder{
				Id:     "grandchild",
				Title:  "Grandchild",
				Parent: proto.String("child"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:   "nonexistent",
						Sealed: true,
					}.Build(),
				},
			}.Build()

			err = helper.ValidateCreation(ctx, grandchild)
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(grpccodes.InvalidArgument))
			Expect(st.Message()).To(ContainSubstring("nonexistent"))
		})

		It("Accepts a valid child and normalizes the parent to its identifier", func() {
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "parent_id",
				Title: "Parent",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: versionDefault,
					}.Build(),
				},
			}.Build())

			childDefault, err := anypb.New(wrapperspb.Int32(43))
			Expect(err).ToNot(HaveOccurred())
			child := privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("parent_id"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Default: childDefault,
						Sealed:  true,
					}.Build(),
				},
			}.Build()

			err = helper.ValidateCreation(ctx, child)
			Expect(err).ToNot(HaveOccurred())
			Expect(child.GetParent()).To(Equal("parent_id"))
		})
	})

	Describe("Validate update", func() {
		It("Allows updating a root template that has no children", func() {
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "root",
				Title: "Root",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: versionDefault,
					}.Build(),
				},
			}.Build())

			newDefault, err := anypb.New(wrapperspb.Int32(99))
			Expect(err).ToNot(HaveOccurred())
			updated := privatev1.ClusterTemplate_builder{
				Id:    "root",
				Title: "Root updated",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: newDefault,
					}.Build(),
				},
			}.Build()

			err = helper.ValidateUpdate(ctx, updated)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Rejects updating a template to reference a non-existent parent", func() {
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "child",
				Title: "Child",
			}.Build())

			updated := privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("does_not_exist"),
			}.Build()

			err := helper.ValidateUpdate(ctx, updated)
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(grpccodes.NotFound))
		})

		It("Rejects updating a template with parameters not in the parent", func() {
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "parent",
				Title: "Parent",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: versionDefault,
					}.Build(),
				},
			}.Build())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("parent"),
			}.Build())

			updated := privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("parent"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:   "bogus",
						Sealed: true,
					}.Build(),
				},
			}.Build()

			err = helper.ValidateUpdate(ctx, updated)
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(grpccodes.InvalidArgument))
			Expect(st.Message()).To(ContainSubstring("bogus"))
		})

		It("Rejects removing a parameter that a child template references", func() {
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())
			selinuxDefault, err := anypb.New(wrapperspb.String("enabled"))
			Expect(err).ToNot(HaveOccurred())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "root",
				Title: "Root",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: versionDefault,
					}.Build(),
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "selinux",
						Title:   "SELinux",
						Type:    "type.googleapis.com/google.protobuf.StringValue",
						Default: selinuxDefault,
					}.Build(),
				},
			}.Build())

			childDefault, err := anypb.New(wrapperspb.String("permissive"))
			Expect(err).ToNot(HaveOccurred())
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("root"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "selinux",
						Default: childDefault,
					}.Build(),
				},
			}.Build())

			// Update the root to remove the "selinux" parameter that the child overrides:
			updated := privatev1.ClusterTemplate_builder{
				Id:    "root",
				Title: "Root",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: versionDefault,
					}.Build(),
				},
			}.Build()

			err = helper.ValidateUpdate(ctx, updated)
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(grpccodes.FailedPrecondition))
			Expect(st.Message()).To(ContainSubstring("child"))
			Expect(st.Message()).To(ContainSubstring("selinux"))
		})

		It("Rejects sealing a parameter that a child template overrides", func() {
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "root",
				Title: "Root",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: versionDefault,
					}.Build(),
				},
			}.Build())

			childDefault, err := anypb.New(wrapperspb.Int32(43))
			Expect(err).ToNot(HaveOccurred())
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("root"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Default: childDefault,
					}.Build(),
				},
			}.Build())

			// Update the root to seal the "version" parameter that the child overrides:
			updated := privatev1.ClusterTemplate_builder{
				Id:    "root",
				Title: "Root",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: versionDefault,
						Sealed:  true,
					}.Build(),
				},
			}.Build()

			err = helper.ValidateUpdate(ctx, updated)
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(grpccodes.FailedPrecondition))
			Expect(st.Message()).To(ContainSubstring("child"))
			Expect(st.Message()).To(ContainSubstring("version"))
			Expect(st.Message()).To(ContainSubstring("sealed"))
		})
	})

	Describe("Validate deletion", func() {
		It("Allows deleting a template that has no children", func() {
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "lonely",
				Title: "Lonely",
			}.Build())

			err := helper.ValidateDeletion(ctx, "lonely")
			Expect(err).ToNot(HaveOccurred())
		})

		It("Rejects deleting a template that is used as a parent", func() {
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "parent",
				Title: "Parent",
			}.Build())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("parent"),
			}.Build())

			err := helper.ValidateDeletion(ctx, "parent")
			Expect(err).To(HaveOccurred())
			st, ok := grpcstatus.FromError(err)
			Expect(ok).To(BeTrue())
			Expect(st.Code()).To(Equal(grpccodes.FailedPrecondition))
			Expect(st.Message()).To(ContainSubstring("parent"))
			Expect(st.Message()).To(ContainSubstring("child"))
		})
	})

	Describe("Template flattening", func() {
		It("Is a no-op for root templates", func() {
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())

			template := privatev1.ClusterTemplate_builder{
				Id:    "root",
				Title: "Root",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Default: versionDefault,
					}.Build(),
				},
			}.Build()

			err = helper.Flatten(ctx, template)
			Expect(err).ToNot(HaveOccurred())
			Expect(template.GetParameters()).To(HaveLen(1))
			Expect(template.GetParameters()[0].GetName()).To(Equal("version"))
		})

		It("Merges parameters from parent into child", func() {
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())
			selinuxDefault, err := anypb.New(wrapperspb.String("enabled"))
			Expect(err).ToNot(HaveOccurred())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "parent",
				Title: "Parent",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: versionDefault,
					}.Build(),
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "selinux",
						Title:   "SELinux",
						Type:    "type.googleapis.com/google.protobuf.StringValue",
						Default: selinuxDefault,
					}.Build(),
				},
			}.Build())

			childDefault, err := anypb.New(wrapperspb.Int32(43))
			Expect(err).ToNot(HaveOccurred())
			child := privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("parent"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Default: childDefault,
						Sealed:  true,
					}.Build(),
				},
			}.Build()

			err = helper.Flatten(ctx, child)
			Expect(err).ToNot(HaveOccurred())
			Expect(child.GetParameters()).To(HaveLen(2))

			paramsByName := map[string]*privatev1.ClusterTemplateParameterDefinition{}
			for _, p := range child.GetParameters() {
				paramsByName[p.GetName()] = p
			}

			Expect(paramsByName).To(HaveKey("version"))
			Expect(paramsByName).To(HaveKey("selinux"))
			Expect(paramsByName["version"].GetSealed()).To(BeTrue())
			Expect(paramsByName["version"].GetTitle()).To(Equal("Version"))
			Expect(paramsByName["selinux"].GetSealed()).To(BeFalse())
		})

		It("Merges through a three-level chain", func() {
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())
			selinuxDefault, err := anypb.New(wrapperspb.String("enabled"))
			Expect(err).ToNot(HaveOccurred())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "root",
				Title: "Root",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Title:   "Version",
						Type:    "type.googleapis.com/google.protobuf.Int32Value",
						Default: versionDefault,
					}.Build(),
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "selinux",
						Title:   "SELinux",
						Type:    "type.googleapis.com/google.protobuf.StringValue",
						Default: selinuxDefault,
					}.Build(),
				},
			}.Build())

			childDefault, err := anypb.New(wrapperspb.Int32(43))
			Expect(err).ToNot(HaveOccurred())
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("root"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "version",
						Default: childDefault,
						Sealed:  true,
					}.Build(),
				},
			}.Build())

			newSelinux, err := anypb.New(wrapperspb.String("permissive"))
			Expect(err).ToNot(HaveOccurred())
			grandchild := privatev1.ClusterTemplate_builder{
				Id:     "grandchild",
				Title:  "Grandchild",
				Parent: proto.String("child"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:    "selinux",
						Default: newSelinux,
					}.Build(),
				},
			}.Build()

			err = helper.Flatten(ctx, grandchild)
			Expect(err).ToNot(HaveOccurred())
			Expect(grandchild.GetParameters()).To(HaveLen(2))

			paramsByName := map[string]*privatev1.ClusterTemplateParameterDefinition{}
			for _, p := range grandchild.GetParameters() {
				paramsByName[p.GetName()] = p
			}

			Expect(paramsByName["version"].GetSealed()).To(BeTrue())
			Expect(paramsByName["selinux"].GetSealed()).To(BeFalse())
		})

		It("Allows a child to override title and description without changing type or default", func() {
			versionDefault, err := anypb.New(wrapperspb.Int32(42))
			Expect(err).ToNot(HaveOccurred())

			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "parent",
				Title: "Parent",
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:        "version",
						Title:       "Original title",
						Description: "Original description",
						Type:        "type.googleapis.com/google.protobuf.Int32Value",
						Default:     versionDefault,
					}.Build(),
				},
			}.Build())

			child := privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("parent"),
				Parameters: []*privatev1.ClusterTemplateParameterDefinition{
					privatev1.ClusterTemplateParameterDefinition_builder{
						Name:        "version",
						Title:       "Custom title",
						Description: "Custom description",
					}.Build(),
				},
			}.Build()

			err = helper.Flatten(ctx, child)
			Expect(err).ToNot(HaveOccurred())
			Expect(child.GetParameters()).To(HaveLen(1))

			param := child.GetParameters()[0]
			Expect(param.GetName()).To(Equal("version"))
			Expect(param.GetTitle()).To(Equal("Custom title"))
			Expect(param.GetDescription()).To(Equal("Custom description"))
			Expect(param.GetType()).To(Equal("type.googleapis.com/google.protobuf.Int32Value"))
			Expect(proto.Equal(param.GetDefault(), versionDefault)).To(BeTrue())
			Expect(param.GetSealed()).To(BeFalse())
		})
	})

	Describe("Calculate Ansible role", func() {
		It("Uses the annotation on the template itself", func() {
			template := privatev1.ClusterTemplate_builder{
				Id:    "my_template",
				Title: "My template",
				Metadata: privatev1.Metadata_builder{
					Annotations: map[string]string{
						annotations.AnsibleRole: "custom_role",
					},
				}.Build(),
			}.Build()

			role, err := helper.CalculateAnsibleRole(ctx, template)
			Expect(err).ToNot(HaveOccurred())
			Expect(role).To(Equal("custom_role"))
		})

		It("Uses the root annotation when the child has none", func() {
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "root",
				Title: "Root",
				Metadata: privatev1.Metadata_builder{
					Annotations: map[string]string{
						annotations.AnsibleRole: "root_role",
					},
				}.Build(),
			}.Build())

			child := privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("root"),
			}.Build()

			role, err := helper.CalculateAnsibleRole(ctx, child)
			Expect(err).ToNot(HaveOccurred())
			Expect(role).To(Equal("root_role"))
		})

		It("Falls back to the root name when no annotation exists", func() {
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "root_id",
				Title: "Root",
				Metadata: privatev1.Metadata_builder{
					Name: "root-name",
				}.Build(),
			}.Build())

			child := privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("root_id"),
			}.Build()

			role, err := helper.CalculateAnsibleRole(ctx, child)
			Expect(err).ToNot(HaveOccurred())
			Expect(role).To(Equal("root-name"))
		})

		It("Falls back to the root id when no annotation or name exists", func() {
			createTemplate(privatev1.ClusterTemplate_builder{
				Id:    "root_id",
				Title: "Root",
			}.Build())

			child := privatev1.ClusterTemplate_builder{
				Id:     "child",
				Title:  "Child",
				Parent: proto.String("root_id"),
			}.Build()

			role, err := helper.CalculateAnsibleRole(ctx, child)
			Expect(err).ToNot(HaveOccurred())
			Expect(role).To(Equal("root_id"))
		})

		It("Uses the id for a root template without annotation or name", func() {
			template := privatev1.ClusterTemplate_builder{
				Id:    "standalone",
				Title: "Standalone",
			}.Build()

			role, err := helper.CalculateAnsibleRole(ctx, template)
			Expect(err).ToNot(HaveOccurred())
			Expect(role).To(Equal("standalone"))
		})
	})
})

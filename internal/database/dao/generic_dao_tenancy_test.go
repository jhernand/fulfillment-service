/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package dao

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	testsv1 "github.com/osac-project/fulfillment-service/internal/api/osac/tests/v1"
	"github.com/osac-project/fulfillment-service/internal/auth"
	"github.com/osac-project/fulfillment-service/internal/collections"
	"github.com/osac-project/fulfillment-service/internal/database"
)

var _ = Describe("Tenancy logic", func() {
	// ctxWithTx creates a context with a fresh transaction whose tenancy logic is configured to return the given
	// set of visible tenants. The transaction is automatically ended when the test finishes.
	ctxWithTx := func(tenants collections.Set[string]) context.Context {
		var err error

		tenancy := auth.NewMockTenancyLogic(ctrl)
		tenancy.EXPECT().DetermineVisibleTenants(gomock.Any()).
			Return(tenants, nil).
			AnyTimes()

		tm, err = database.NewTxManager().
			SetLogger(logger).
			SetPool(pool).
			SetTenancyLogic(tenancy).
			Build()
		Expect(err).ToNot(HaveOccurred())

		ctx := context.Background()
		tx, err := tm.Begin(ctx)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			err := tx.End(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
		return database.TxIntoContext(ctx, tx)
	}

	It("Filters field based on user visibility", func() {
		dao, err := NewGenericDAO[*testsv1.Object]().
			SetLogger(logger).
			Build()
		Expect(err).ToNot(HaveOccurred())

		ctx := ctxWithTx(collections.NewSet("tenant_a", "tenant_c"))

		createResponse, err := dao.Create().
			SetObject(testsv1.Object_builder{
				Metadata: testsv1.Metadata_builder{
					Tenant: "tenant_a",
				}.Build(),
			}.Build()).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())
		object := createResponse.GetObject()
		Expect(object.GetMetadata().GetTenant()).To(Equal("tenant_a"))

		getResponse, err := dao.Get().
			SetId(object.GetId()).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())
		object = getResponse.GetObject()
		Expect(object.GetMetadata().GetTenant()).To(Equal("tenant_a"))

		listResponse, err := dao.List().
			SetFilter(fmt.Sprintf("this.id == %q", object.GetId())).
			SetLimit(1).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(listResponse.GetItems()).To(HaveLen(1))
		Expect(listResponse.GetItems()[0].GetMetadata().GetTenant()).To(Equal("tenant_a"))

		object.SetMyString("hello")
		object.GetMetadata().SetTenant("tenant_a")
		updateResponse, err := dao.Update().
			SetObject(object).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())
		object = updateResponse.GetObject()
		Expect(object.GetMetadata().GetTenant()).To(Equal("tenant_a"))
	})

	It("Shows all tenants when user has no tenant restrictions", func() {
		dao, err := NewGenericDAO[*testsv1.Object]().
			SetLogger(logger).
			Build()
		Expect(err).ToNot(HaveOccurred())

		ctx := ctxWithTx(collections.NewUniversalSet[string]())

		createResponse, err := dao.Create().
			SetObject(testsv1.Object_builder{
				Metadata: testsv1.Metadata_builder{
					Tenant: "tenant_a",
				}.Build(),
			}.Build()).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())
		object := createResponse.GetObject()
		Expect(object.GetMetadata().GetTenant()).To(Equal("tenant_a"))
	})

	It("Hides objects belonging to invisible tenants", func() {
		dao, err := NewGenericDAO[*testsv1.Object]().
			SetLogger(logger).
			Build()
		Expect(err).ToNot(HaveOccurred())

		// Create the object using universal access:
		adminCtx := ctxWithTx(collections.NewUniversalSet[string]())
		createResponse, err := dao.Create().
			SetObject(testsv1.Object_builder{
				Metadata: testsv1.Metadata_builder{
					Tenant: "tenant_y",
				}.Build(),
			}.Build()).
			Do(adminCtx)
		Expect(err).ToNot(HaveOccurred())
		object := createResponse.GetObject()

		// Use a restricted tenancy that cannot see tenant_y:
		restrictedCtx := ctxWithTx(collections.NewSet("tenant_x"))
		_, err = dao.Get().
			SetId(object.GetId()).
			Do(restrictedCtx)
		var notFoundErr *ErrNotFound
		Expect(err).To(HaveOccurred())
		Expect(err).To(BeAssignableToTypeOf(notFoundErr))

		listResponse, err := dao.List().
			Do(restrictedCtx)
		Expect(err).ToNot(HaveOccurred())
		Expect(listResponse.GetItems()).To(BeEmpty())
	})

	It("Allows a tenant to delete an object it created", func() {
		dao, err := NewGenericDAO[*testsv1.Object]().
			SetLogger(logger).
			Build()
		Expect(err).ToNot(HaveOccurred())

		ctx := ctxWithTx(collections.NewSet("tenant_a"))

		createResponse, err := dao.Create().
			SetObject(testsv1.Object_builder{
				Metadata: testsv1.Metadata_builder{
					Tenant: "tenant_a",
				}.Build(),
			}.Build()).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())
		object := createResponse.GetObject()

		_, err = dao.Delete().
			SetId(object.GetId()).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())

		_, err = dao.Get().
			SetId(object.GetId()).
			Do(ctx)
		var notFoundErr *ErrNotFound
		Expect(err).To(HaveOccurred())
		Expect(err).To(BeAssignableToTypeOf(notFoundErr))
	})

	It("Rejects deletion of an object belonging to an invisible tenant as not found", func() {
		dao, err := NewGenericDAO[*testsv1.Object]().
			SetLogger(logger).
			Build()
		Expect(err).ToNot(HaveOccurred())

		// Create the object using universal access:
		adminCtx := ctxWithTx(collections.NewUniversalSet[string]())
		createResponse, err := dao.Create().
			SetObject(testsv1.Object_builder{
				Metadata: testsv1.Metadata_builder{
					Tenant: "tenant_a",
				}.Build(),
			}.Build()).
			Do(adminCtx)
		Expect(err).ToNot(HaveOccurred())
		object := createResponse.GetObject()

		// Try to delete using a tenancy that cannot see tenant_a:
		restrictedCtx := ctxWithTx(collections.NewSet("tenant_b"))
		_, err = dao.Delete().
			SetId(object.GetId()).
			Do(restrictedCtx)
		var notFoundErr *ErrNotFound
		Expect(err).To(HaveOccurred())
		Expect(err).To(BeAssignableToTypeOf(notFoundErr))

		// Verify the object still exists:
		getResponse, err := dao.Get().
			SetId(object.GetId()).
			Do(adminCtx)
		Expect(err).ToNot(HaveOccurred())
		Expect(getResponse.GetObject().GetId()).To(Equal(object.GetId()))
	})

	It("Allows a tenant to update an object it created", func() {
		dao, err := NewGenericDAO[*testsv1.Object]().
			SetLogger(logger).
			Build()
		Expect(err).ToNot(HaveOccurred())

		ctx := ctxWithTx(collections.NewSet("tenant_a"))

		createResponse, err := dao.Create().
			SetObject(testsv1.Object_builder{
				Metadata: testsv1.Metadata_builder{
					Tenant: "tenant_a",
				}.Build(),
			}.Build()).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())
		object := createResponse.GetObject()

		object.SetMyString("updated")
		updateResponse, err := dao.Update().
			SetObject(object).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(updateResponse.GetObject().GetMyString()).To(Equal("updated"))

		getResponse, err := dao.Get().
			SetId(object.GetId()).
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())
		Expect(getResponse.GetObject().GetMyString()).To(Equal("updated"))
	})

	It("Rejects update of an object belonging to an invisible tenant as not found", func() {
		dao, err := NewGenericDAO[*testsv1.Object]().
			SetLogger(logger).
			Build()
		Expect(err).ToNot(HaveOccurred())

		// Create the object using universal access:
		adminCtx := ctxWithTx(collections.NewUniversalSet[string]())
		createResponse, err := dao.Create().
			SetObject(testsv1.Object_builder{
				Metadata: testsv1.Metadata_builder{
					Tenant: "tenant_a",
				}.Build(),
			}.Build()).
			Do(adminCtx)
		Expect(err).ToNot(HaveOccurred())
		object := createResponse.GetObject()

		// Try to update using a tenancy that cannot see tenant_a:
		restrictedCtx := ctxWithTx(collections.NewSet("tenant_b"))
		object.SetMyString("updated")
		_, err = dao.Update().
			SetObject(object).
			Do(restrictedCtx)
		var notFoundErr *ErrNotFound
		Expect(err).To(HaveOccurred())
		Expect(err).To(BeAssignableToTypeOf(notFoundErr))

		// Verify the object was not modified:
		getResponse, err := dao.Get().
			SetId(object.GetId()).
			Do(adminCtx)
		Expect(err).ToNot(HaveOccurred())
		Expect(getResponse.GetObject().GetMyString()).To(BeEmpty())
	})
})

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
	"google.golang.org/protobuf/proto"

	ffv1 "github.com/innabox/fulfillment-service/internal/api/fulfillment/v1"
	privatev1 "github.com/innabox/fulfillment-service/internal/api/private/v1"
	"github.com/innabox/fulfillment-service/internal/database"
)

var _ = Describe("Private cluster orders server", func() {
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

		// Create the templates table:
		_, err = tx.Exec(
			ctx,
			`
			create schema private;

			create table private.cluster_orders (
				id text not null primary key,
				creation_timestamp timestamp with time zone not null default now(),
				deletion_timestamp timestamp with time zone not null default 'epoch',
				public_data jsonb not null default '{}',
				private_data jsonb not null default '{}'
			);
			`,
		)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Creation", func() {
		It("Can be built if all the required parameters are set", func() {
			server, err := NewPrivateClusterOrdersServer().
				SetLogger(logger).
				Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(server).ToNot(BeNil())
		})

		It("Fails if logger is not set", func() {
			server, err := NewPrivateClusterOrdersServer().
				Build()
			Expect(err).To(MatchError("logger is mandatory"))
			Expect(server).To(BeNil())
		})
	})

	Describe("Behaviour", func() {
		var server *PrivateClusterOrdersServer

		BeforeEach(func() {
			var err error

			// Create the server:
			server, err = NewPrivateClusterOrdersServer().
				SetLogger(logger).
				Build()
			Expect(err).ToNot(HaveOccurred())
		})

		It("Creates object", func() {
			response, err := server.Create(ctx, privatev1.ClusterOrdersCreateRequest_builder{
				Public: ffv1.ClusterOrder_builder{
					Spec: ffv1.ClusterOrderSpec_builder{
						TemplateId: "my_template",
					}.Build(),
				}.Build(),
				Private: privatev1.ClusterOrder_builder{
					HubId: "my_hub",
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response).ToNot(BeNil())
			public := response.GetPublic()
			private := response.GetPrivate()
			Expect(public).ToNot(BeNil())
			Expect(public.GetId()).ToNot(BeEmpty())
			Expect(public.GetSpec().GetTemplateId()).To(Equal("my_template"))
			Expect(private).ToNot(BeNil())
			Expect(private.GetHubId()).To(Equal("my_hub"))
		})

		It("List objects", func() {
			// Create a few objects:
			const count = 10
			for i := range count {
				_, err := server.Create(ctx, privatev1.ClusterOrdersCreateRequest_builder{
					Public: ffv1.ClusterOrder_builder{
						Spec: ffv1.ClusterOrderSpec_builder{
							TemplateId: "my_template",
						}.Build(),
					}.Build(),
					Private: privatev1.ClusterOrder_builder{
						HubId: fmt.Sprintf("my_hub_%d", i),
					}.Build(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
			}

			// List the objects:
			response, err := server.List(ctx, privatev1.ClusterOrdersListRequest_builder{}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response).ToNot(BeNil())
			size := int(response.GetSize())
			public := response.GetPublic()
			private := response.GetPrivate()
			Expect(size).To(Equal(count))
			Expect(public).To(HaveLen(size))
			Expect(private).To(HaveLen(size))
			for i := range size {
				Expect(public[i].GetId()).ToNot(BeEmpty())
				Expect(public[i].GetSpec().GetTemplateId()).To(Equal("my_template"))
				Expect(private[i].GetHubId()).To(Equal(fmt.Sprintf("my_hub_%d", i)))
			}
		})

		It("List objects with limit", func() {
			// Create a few objects:
			const count = 10
			for i := range count {
				_, err := server.Create(ctx, privatev1.ClusterOrdersCreateRequest_builder{
					Private: privatev1.ClusterOrder_builder{
						HubId: fmt.Sprintf("my_hub_%d", i),
					}.Build(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
			}

			// List the objects:
			response, err := server.List(ctx, privatev1.ClusterOrdersListRequest_builder{
				Limit: proto.Int32(1),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetSize()).To(BeNumerically("==", 1))
		})

		It("List objects with offset", func() {
			// Create a few objects:
			const count = 10
			for i := range count {
				_, err := server.Create(ctx, privatev1.ClusterOrdersCreateRequest_builder{
					Private: privatev1.ClusterOrder_builder{
						HubId: fmt.Sprintf("my_hub_%d", i),
					}.Build(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
			}

			// List the objects:
			response, err := server.List(ctx, privatev1.ClusterOrdersListRequest_builder{
				Offset: proto.Int32(1),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetSize()).To(BeNumerically("==", count-1))
		})

		It("List objects with filter", func() {
			// Create a few objects:
			const count = 10
			var ids []string
			for i := range count {
				response, err := server.Create(ctx, privatev1.ClusterOrdersCreateRequest_builder{
					Private: privatev1.ClusterOrder_builder{
						HubId: fmt.Sprintf("my_hub_%d", i),
					}.Build(),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
				ids = append(ids, response.GetPublic().GetId())
			}

			// List the objects:
			for _, id := range ids {
				response, err := server.List(ctx, privatev1.ClusterOrdersListRequest_builder{
					Filter: proto.String(fmt.Sprintf("this.id == '%s'", id)),
				}.Build())
				Expect(err).ToNot(HaveOccurred())
				Expect(response.GetSize()).To(BeNumerically("==", 1))
				Expect(response.GetPublic()[0].GetId()).To(Equal(id))
			}
		})

		It("Get object", func() {
			// Create the object:
			createResponse, err := server.Create(ctx, privatev1.ClusterOrdersCreateRequest_builder{
				Private: privatev1.ClusterOrder_builder{
					HubId: "my_hub",
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Get it:
			getResponse, err := server.Get(ctx, privatev1.ClusterOrdersGetRequest_builder{
				Id: createResponse.GetPublic().GetId(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(proto.Equal(createResponse.GetPublic(), getResponse.GetPublic())).To(BeTrue())
		})

		It("Update object", func() {
			// Create the object:
			createResponse, err := server.Create(ctx, privatev1.ClusterOrdersCreateRequest_builder{
				Private: privatev1.ClusterOrder_builder{
					HubId: "my_hub",
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			public := createResponse.GetPublic()

			// Update the object:
			updateResponse, err := server.Update(ctx, privatev1.ClusterOrdersUpdateRequest_builder{
				Public: ffv1.ClusterOrder_builder{
					Id: public.GetId(),
				}.Build(),
				Private: privatev1.ClusterOrder_builder{
					HubId: "your_hub",
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(updateResponse.GetPrivate().GetHubId()).To(Equal("your_hub"))

			// Get and verify:
			getResponse, err := server.Get(ctx, privatev1.ClusterOrdersGetRequest_builder{
				Id: public.GetId(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(getResponse.GetPrivate().GetHubId()).To(Equal("your_hub"))
		})

		It("Delete object", func() {
			// Create the object:
			createResponse, err := server.Create(ctx, privatev1.ClusterOrdersCreateRequest_builder{
				Private: privatev1.ClusterOrder_builder{
					HubId: "my_hub",
				}.Build(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			public := createResponse.GetPublic()

			// Delete the object:
			_, err = server.Delete(ctx, privatev1.ClusterOrdersDeleteRequest_builder{
				Id: public.GetId(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())

			// Get and verify:
			getResponse, err := server.Get(ctx, privatev1.ClusterOrdersGetRequest_builder{
				Id: public.GetId(),
			}.Build())
			Expect(err).ToNot(HaveOccurred())
			Expect(getResponse.GetPublic().GetMetadata().GetDeletionTimestamp()).ToNot(BeNil())
		})
	})
})

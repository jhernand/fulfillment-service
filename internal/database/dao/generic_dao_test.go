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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/wrapperspb"

	api "github.com/innabox/fulfillment-service/internal/api/fulfillment/v1"
	"github.com/innabox/fulfillment-service/internal/database"
)

var _ = Describe("Generic DAO", func() {
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
	})

	Describe("Creation", func() {
		It("Can be built if all the required parameters are set", func() {
			generic, err := NewGenericDAO[*api.Cluster]().
				SetLogger(logger).
				SetTable("clusters").
				Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(generic).ToNot(BeNil())
		})

		It("Fails if logger is not set", func() {
			generic, err := NewGenericDAO[*api.Cluster]().
				SetTable("clusters").
				Build()
			Expect(err).To(MatchError("logger is mandatory"))
			Expect(generic).To(BeNil())
		})

		It("Fails if table is not set", func() {
			generic, err := NewGenericDAO[*api.Cluster]().
				SetLogger(logger).
				Build()
			Expect(err).To(MatchError("table is mandatory"))
			Expect(generic).To(BeNil())
		})

		It("Fails if object doesn't have identifier", func() {
			generic, err := NewGenericDAO[*wrapperspb.Int32Value]().
				SetLogger(logger).
				SetTable("integers").
				Build()
			Expect(err).To(MatchError(
				"object of type '*wrapperspb.Int32Value' doesn't have an identifier field",
			))
			Expect(generic).To(BeNil())
		})
	})

	Describe("Behaviour", func() {
		var generic *GenericDAO[*api.Cluster]

		BeforeEach(func() {
			// Create the table:
			_, err := tx.Exec(
				ctx,
				`
				create table clusters (
					id uuid not null primary key,
					data jsonb not null
				)
				`,
			)
			Expect(err).ToNot(HaveOccurred())

			// Create the DAO:
			generic, err = NewGenericDAO[*api.Cluster]().
				SetLogger(logger).
				SetTable("clusters").
				Build()
			Expect(err).ToNot(HaveOccurred())
		})

		It("Inserts object", func() {
			// Insert the object:
			object := &api.Cluster{}
			id, err := generic.Insert(ctx, object)
			Expect(err).ToNot(HaveOccurred())

			// Check the database:
			row := tx.QueryRow(ctx, `select data from clusters where id = $1`, id)
			var data []byte
			err = row.Scan(&data)
			Expect(err).ToNot(HaveOccurred())
			Expect(data).ToNot(BeNil())
		})

		It("Gets object", func() {
			// Insert the row:
			id, err := generic.Insert(ctx, &api.Cluster{})
			Expect(err).ToNot(HaveOccurred())

			// Try to get it:
			object, err := generic.Get(ctx, id)
			Expect(err).ToNot(HaveOccurred())
			Expect(object).ToNot(BeNil())
		})

		It("Lists objects", func() {
			// Insert a couple of rows:
			const count = 2
			for range count {
				_, err := generic.Insert(ctx, &api.Cluster{})
				Expect(err).ToNot(HaveOccurred())
			}

			// Try to list:
			items, err := generic.List(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(items).To(HaveLen(count))
			for _, item := range items {
				Expect(item).ToNot(BeNil())
			}
		})

		Describe("Check if object exists", func() {
			It("Returns true if the object exists", func() {
				// Insert the object:
				object := &api.Cluster{}
				id, err := generic.Insert(ctx, object)
				Expect(err).ToNot(HaveOccurred())

				// Check if it exists:
				exists, err := generic.Exists(ctx, id)
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue())
			})

			It("Returns false if the object doesn't exist", func() {
				// The database is empty, check the result:
				exists, err := generic.Exists(ctx, uuid.NewString())
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeFalse())
			})
		})

		It("Updates object", func() {
			// Create the object:
			object := &api.Cluster{
				Status: &api.ClusterStatus{
					ApiUrl: "my_url",
				},
			}
			id, err := generic.Insert(ctx, object)
			Expect(err).ToNot(HaveOccurred())

			// Try to update:
			object.Status.ApiUrl = "your_url"
			err = generic.Update(ctx, id, object)
			Expect(err).ToNot(HaveOccurred())

			// Get it and verify that the changes have been applied:
			object, err = generic.Get(ctx, id)
			Expect(err).ToNot(HaveOccurred())
			Expect(object.Status.ApiUrl).To(Equal("your_url"))
		})
	})
})

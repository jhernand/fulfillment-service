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
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/timestamppb"

	testsv1 "github.com/innabox/fulfillment-service/internal/api/tests/v1"
	"github.com/innabox/fulfillment-service/internal/database"
)

var _ = Describe("Generic DAO", func() {
	const (
		defaultLimit = 5
		maxLimit     = 10
		objectCount  = maxLimit + 1
	)

	var (
		ctx context.Context
		tx  database.Tx
	)

	sort := func(objects []*testsv1.Public) {
		sort.Slice(objects, func(i, j int) bool {
			return strings.Compare(objects[i].GetId(), objects[j].GetId()) < 0
		})
	}

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
			generic, err := NewGenericDAO[*testsv1.Public, *testsv1.Private]().
				SetLogger(logger).
				SetTable("objects").
				Build()
			Expect(err).ToNot(HaveOccurred())
			Expect(generic).ToNot(BeNil())
		})

		It("Fails if logger is not set", func() {
			generic, err := NewGenericDAO[*testsv1.Public, *testsv1.Private]().
				SetTable("objects").
				Build()
			Expect(err).To(MatchError("logger is mandatory"))
			Expect(generic).To(BeNil())
		})

		It("Fails if table is not set", func() {
			generic, err := NewGenericDAO[*testsv1.Public, *testsv1.Private]().
				SetLogger(logger).
				Build()
			Expect(err).To(MatchError("table is mandatory"))
			Expect(generic).To(BeNil())
		})

		It("Fails if default limit is zero", func() {
			generic, err := NewGenericDAO[*testsv1.Public, *testsv1.Private]().
				SetLogger(logger).
				SetTable("objects").
				SetDefaultLimit(0).
				Build()
			Expect(err).To(MatchError("default limit must be a possitive integer, but it is 0"))
			Expect(generic).To(BeNil())
		})

		It("Fails if default limit is negative", func() {
			generic, err := NewGenericDAO[*testsv1.Public, *testsv1.Private]().
				SetLogger(logger).
				SetTable("objects").
				SetDefaultLimit(-1).
				Build()
			Expect(err).To(MatchError("default limit must be a possitive integer, but it is -1"))
			Expect(generic).To(BeNil())
		})

		It("Fails if max limit is zero", func() {
			generic, err := NewGenericDAO[*testsv1.Public, *testsv1.Private]().
				SetLogger(logger).
				SetTable("objects").
				SetMaxLimit(0).
				Build()
			Expect(err).To(MatchError("max limit must be a possitive integer, but it is 0"))
			Expect(generic).To(BeNil())
		})

		It("Fails if max limit is negative", func() {
			generic, err := NewGenericDAO[*testsv1.Public, *testsv1.Private]().
				SetLogger(logger).
				SetTable("objects").
				SetMaxLimit(-1).
				Build()
			Expect(err).To(MatchError("max limit must be a possitive integer, but it is -1"))
			Expect(generic).To(BeNil())
		})

		It("Fails if max limit is less than default limit", func() {
			generic, err := NewGenericDAO[*testsv1.Public, *testsv1.Private]().
				SetLogger(logger).
				SetTable("objects").
				SetMaxLimit(100).
				SetDefaultLimit(1000).
				Build()
			Expect(err).To(MatchError(
				"max limit must be greater or equal to default limit, but max limit is 100 and " +
					"default limit is 1000",
			))
			Expect(generic).To(BeNil())
		})
	})

	Describe("Behaviour", func() {
		var generic *GenericDAO[*testsv1.Public, *testsv1.Private]

		BeforeEach(func() {
			// Create the table:
			_, err := tx.Exec(
				ctx,
				`
				create table objects (
					id text not null primary key,
					creation_timestamp timestamp with time zone not null default now(),
					deletion_timestamp timestamp with time zone not null default 'epoch',
					public_data jsonb not null default '{}',
					private_data jsonb not null default '{}'
				)
				`,
			)
			Expect(err).ToNot(HaveOccurred())

			// Create the DAO:
			generic, err = NewGenericDAO[*testsv1.Public, *testsv1.Private]().
				SetLogger(logger).
				SetTable("objects").
				SetDefaultOrder("id").
				SetDefaultLimit(defaultLimit).
				SetMaxLimit(maxLimit).
				Build()
			Expect(err).ToNot(HaveOccurred())
		})

		It("Creates object", func() {
			createResponse, err := generic.Create().Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			object := createResponse.GetPublic()
			getResponse, err := generic.Get().SetID(object.GetId()).Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(getResponse).ToNot(BeNil())
			Expect(getResponse.GetPublic()).ToNot(BeNil())
		})

		It("Sets metadata when creating", func() {
			response, err := generic.Create().Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			object := response.GetPublic()
			Expect(object.HasMetadata()).To(BeTrue())
		})

		It("Sets creation timestamp when creating", func() {
			response, err := generic.Create().Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetPublic().GetMetadata().GetCreationTimestamp().AsTime()).ToNot(BeZero())
		})

		It("Doesn't set deletion timestamp when creating", func() {
			response, err := generic.Create().Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetPublic().GetMetadata().HasDeletionTimestamp()).To(BeFalse())
		})

		It("Generates non empty identifiers", func() {
			response, err := generic.Create().Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(response.GetPublic().GetId()).ToNot(BeEmpty())
		})

		It("Doesn't put the generated identifier inside the input object", func() {
			object := &testsv1.Public{}
			_, err := generic.Create().SetPublic(object).Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(object.GetId()).To(BeEmpty())
		})

		It("Doesn't put the generated metadata inside the input object", func() {
			object := &testsv1.Public{}
			_, err := generic.Create().SetPublic(object).Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(object.Metadata).To(BeNil())
		})

		It("Gets object", func() {
			createResponse, err := generic.Create().Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			object := createResponse.GetPublic()
			getResponse, err := generic.Get().SetID(object.GetId()).Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(getResponse.GetExists()).To(BeTrue())
		})

		It("Lists objects", func() {
			// Insert a couple of rows:
			const count = 2
			for range count {
				_, err := generic.Create().Send(ctx)
				Expect(err).ToNot(HaveOccurred())
			}

			// Try to list:
			response, err := generic.List().Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			items := response.GetPublic()
			Expect(items).To(HaveLen(count))
			for _, item := range items {
				Expect(item).ToNot(BeNil())
			}
		})

		Describe("Paging", func() {
			var objects []*testsv1.Public

			BeforeEach(func() {
				// Create a list of objects and sort it like they will be sorted by the DAO. Not that
				// this works correctly because the DAO is configured with a default sorting. That is
				// intended for use only in these unit tests.
				objects = make([]*testsv1.Public, objectCount)
				for i := range len(objects) {
					objects[i] = &testsv1.Public{
						Id: uuid.NewString(),
					}
					_, err := generic.Create().SetPublic(objects[i]).Send(ctx)
					Expect(err).ToNot(HaveOccurred())
				}
				sort(objects)
			})

			It("Uses zero as default offset", func() {
				response, err := generic.List().SetLimit(1).Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items[0].GetId()).To(Equal(objects[0].GetId()))
			})

			It("Honours valid offset", func() {
				for i := range len(objects) {
					response, err := generic.List().
						SetOffset(i).
						SetLimit(1).
						Send(ctx)
					Expect(err).ToNot(HaveOccurred())
					items := response.GetPublic()
					Expect(items[0].GetId()).To(Equal(objects[i].GetId()))
				}
			})

			It("Returns empty list if offset is greater or equal than available items", func() {
				response, err := generic.List().
					SetOffset(objectCount).
					SetLimit(1).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(BeEmpty())
			})

			It("Ignores negative offset", func() {
				response, err := generic.List().
					SetOffset(-123).
					SetLimit(1).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items[0].GetId()).To(Equal(objects[0].GetId()))
			})

			It("Interprets negative limit as requesting zero items", func() {
				response, err := generic.List().
					SetLimit(-123).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.GetSize()).To(BeZero())
				Expect(response.GetPublic()).To(BeEmpty())
			})

			It("Interprets zero limit as requesting the default number of items", func() {
				response, err := generic.List().
					SetLimit(0).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.GetSize()).To(BeNumerically("==", defaultLimit))
				Expect(response.GetPublic()).To(HaveLen(defaultLimit))
			})

			It("Truncates limit to the maximum", func() {
				response, err := generic.List().
					SetLimit(maxLimit + 1).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.GetSize()).To(BeNumerically("==", maxLimit))
				Expect(response.GetPublic()).To(HaveLen(maxLimit))
			})

			It("Honours valid limit", func() {
				for i := 1; i < maxLimit; i++ {
					response, err := generic.List().
						SetLimit(i).
						Send(ctx)
					Expect(err).ToNot(HaveOccurred())
					Expect(response.GetSize()).To(BeNumerically("==", i))
					Expect(response.GetPublic()).To(HaveLen(i))
				}
			})

			It("Returns less items than requested if there are not enough", func() {
				response, err := generic.List().
					SetOffset(objectCount - 2).
					SetLimit(10).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.GetSize()).To(BeNumerically("==", 2))
				Expect(response.GetPublic()).To(HaveLen(2))
			})

			It("Returns the total number of items", func() {
				response, err := generic.List().
					SetLimit(1).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(response.GetTotal()).To(BeNumerically("==", objectCount))
			})
		})

		Describe("Check if object exists", func() {
			It("Returns true if the object exists", func() {
				createResponse, err := generic.Create().Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				object := createResponse.GetPublic()
				exists, err := generic.Exists(ctx, object.GetId())
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeTrue())
			})

			It("Returns false if the object doesn't exist", func() {
				exists, err := generic.Exists(ctx, uuid.NewString())
				Expect(err).ToNot(HaveOccurred())
				Expect(exists).To(BeFalse())
			})
		})

		It("Updates object", func() {
			createResponse, err := generic.Create().
				SetPublic(
					testsv1.Public_builder{
						MyString: "my_value",
					}.Build(),
				).
				Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			object := createResponse.GetPublic()
			object.SetMyString("your_value")
			updateResponse, err := generic.Update().SetPublic(object).Send(ctx)
			Expect(err).ToNot(HaveOccurred())
			object = updateResponse.GetPublic()
			Expect(object).ToNot(BeNil())
			Expect(object.GetMyString()).To(Equal("your_value"))
		})

		Describe("Filtering", func() {
			It("Filters by identifier", func() {
				for i := range 10 {
					_, err := generic.Create().
						SetPublic(
							testsv1.Public_builder{
								Id: fmt.Sprintf("%d", i),
							}.Build(),
						).
						Send(ctx)
					Expect(err).ToNot(HaveOccurred())
				}
				response, err := generic.List().
					SetFilter("this.id == '5'").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("5"))
			})

			It("Filters by identifier set", func() {
				for i := range 10 {
					_, err := generic.Create().
						SetPublic(
							testsv1.Public_builder{
								Id: fmt.Sprintf("%d", i),
							}.Build(),
						).
						Send(ctx)
					Expect(err).ToNot(HaveOccurred())
				}
				response, err := generic.List().
					SetFilter("this.id in ['1', '3', '5', '7', '9']").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				sort(items)
				Expect(items).To(HaveLen(5))
				Expect(items[0].GetId()).To(Equal("1"))
				Expect(items[1].GetId()).To(Equal("3"))
				Expect(items[2].GetId()).To(Equal("5"))
				Expect(items[3].GetId()).To(Equal("7"))
				Expect(items[4].GetId()).To(Equal("9"))
			})

			It("Filters by string JSON field", func() {
				for i := range 10 {
					_, err := generic.Create().
						SetPublic(
							testsv1.Public_builder{
								MyString: fmt.Sprintf("my_value_%d", i),
							}.Build(),
						).
						Send(ctx)
					Expect(err).ToNot(HaveOccurred())
				}
				response, err := generic.List().
					SetFilter("this.my_string == 'my_value_5'").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetMyString()).To(Equal("my_value_5"))
			})

			It("Filters by identifier or JSON field", func() {
				for i := range 10 {
					_, err := generic.Create().
						SetPublic(
							testsv1.Public_builder{
								Id:       fmt.Sprintf("%d", i),
								MyString: fmt.Sprintf("my_value_%d", i),
							}.Build(),
						).
						Send(ctx)
					Expect(err).ToNot(HaveOccurred())
				}
				response, err := generic.List().
					SetFilter("this.id == '1' || this.my_string == 'my_value_3'").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				sort(items)
				Expect(items).To(HaveLen(2))
				Expect(items[0].GetId()).To(Equal("1"))
				Expect(items[0].GetMyString()).To(Equal("my_value_1"))
				Expect(items[1].GetId()).To(Equal("3"))
				Expect(items[1].GetMyString()).To(Equal("my_value_3"))
			})

			It("Filters by identifier and JSON field", func() {
				for i := range 10 {
					_, err := generic.Create().
						SetPublic(
							testsv1.Public_builder{
								Id:       fmt.Sprintf("%d", i),
								MyString: fmt.Sprintf("my_value_%d", i),
							}.Build(),
						).
						Send(ctx)
					Expect(err).ToNot(HaveOccurred())
				}
				response, err := generic.List().
					SetFilter("this.id == '1' && this.my_string == 'my_value_1'").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("1"))
				Expect(items[0].GetMyString()).To(Equal("my_value_1"))
			})

			It("Filters by calculated value", func() {
				for i := range 10 {
					_, err := generic.Create().
						SetPublic(
							testsv1.Public_builder{
								MyInt32: int32(i),
							}.Build(),
						).
						Send(ctx)
					Expect(err).ToNot(HaveOccurred())
				}
				response, err := generic.List().
					SetFilter("(this.my_int32 + 1) == 2").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetMyInt32()).To(BeNumerically("==", 1))
			})

			It("Filters by nested JSON string field", func() {
				for i := range 10 {
					_, err := generic.Create().
						SetPublic(
							testsv1.Public_builder{
								Spec: testsv1.Spec_builder{
									SpecString: fmt.Sprintf("my_value_%d", i),
								}.Build(),
							}.Build(),
						).
						Send(ctx)
					Expect(err).ToNot(HaveOccurred())
				}
				response, err := generic.List().
					SetFilter("this.spec.spec_string == 'my_value_5'").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetSpec().GetSpecString()).ToNot(BeNil())
				Expect(items[0].GetSpec().GetSpecString()).To(Equal("my_value_5"))
			})

			It("Filters deleted", func() {
				createResponse, err := generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id: "0",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				object := createResponse.GetPublic()
				_, err = generic.Delete().SetPublic(object).Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				listResponse, err := generic.List().
					SetFilter("this.metadata.deletion_timestamp != null").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := listResponse.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("0"))
			})

			It("Filters not deleted", func() {
				createResponse, err := generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id: "0",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				object := createResponse.GetPublic()
				_, err = generic.Delete().SetPublic(object).Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				listResponse, err := generic.List().
					SetFilter("this.metadata.deletion_timestamp == null").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := listResponse.GetPublic()
				Expect(items).To(HaveLen(0))
			})

			It("Filters by timestamp in the future", func() {
				var err error
				now := time.Now()
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:          "old",
							MyTimestamp: timestamppb.New(now.Add(-time.Minute)),
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:          "new",
							MyTimestamp: timestamppb.New(now.Add(+time.Minute)),
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				response, err := generic.List().
					SetFilter("this.my_timestamp > now").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("new"))
			})

			It("Filters by timestamp in the past", func() {
				var err error
				now := time.Now()
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:          "old",
							MyTimestamp: timestamppb.New(now.Add(-time.Minute)),
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:          "new",
							MyTimestamp: timestamppb.New(now.Add(+time.Minute)),
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				response, err := generic.List().
					SetFilter("this.my_timestamp < now").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("old"))
			})

			It("Filters by presence of message field", func() {
				var err error
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:   "good",
							Spec: testsv1.Spec_builder{}.Build(),
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:   "bad",
							Spec: nil,
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				response, err := generic.List().
					SetFilter("has(this.spec)").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("good"))
			})

			It("Filters by presence of string field", func() {
				var err error
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:       "good",
							MyString: "my value",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:       "bad",
							MyString: "",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				response, err := generic.List().
					SetFilter("has(this.my_string)").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("good"))
			})

			It("Filters by presence of deletion timestamp", func() {
				var err error
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id: "good",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id: "bad",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Delete().SetID("good").Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				response, err := generic.List().
					SetFilter("has(this.metadata.deletion_timestamp)").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("good"))
			})

			It("Filters by absence of deletion timestamp", func() {
				var err error
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id: "good",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id: "bad",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Delete().SetID("bad").Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				response, err := generic.List().
					SetFilter("!has(this.metadata.deletion_timestamp)").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("good"))
			})

			It("Filters by presence of nested string field", func() {
				var err error
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id: "good",
							Spec: testsv1.Spec_builder{
								SpecString: "my value",
							}.Build(),
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id: "bad",
							Spec: testsv1.Spec_builder{
								SpecString: "",
							}.Build(),
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				response, err := generic.List().
					SetFilter("has(this.spec.spec_string)").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("good"))
			})

			It("Filters by string prefix", func() {
				var err error
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:       "good",
							MyString: "my value",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:       "bad",
							MyString: "your value",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				response, err := generic.List().
					SetFilter("this.my_string.startsWith('my')").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("good"))
			})

			It("Filters by string suffix", func() {
				var err error
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:       "good",
							MyString: "value my",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:       "bad",
							MyString: "value your",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				response, err := generic.List().
					SetFilter("this.my_string.endsWith('my')").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("good"))
			})

			It("Escapes percent in prefix", func() {
				var err error
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:       "good",
							MyString: "my% value",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:       "bad",
							MyString: "my value",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				response, err := generic.List().
					SetFilter("this.my_string.startsWith('my%')").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("good"))
			})

			It("Escapes underscore in prefix", func() {
				var err error
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:       "good",
							MyString: "my_ value",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				_, err = generic.Create().
					SetPublic(
						testsv1.Public_builder{
							Id:       "bad",
							MyString: "my value",
						}.Build(),
					).
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				response, err := generic.List().
					SetFilter("this.my_string.startsWith('my_')").
					Send(ctx)
				Expect(err).ToNot(HaveOccurred())
				items := response.GetPublic()
				Expect(items).To(HaveLen(1))
				Expect(items[0].GetId()).To(Equal("good"))
			})
		})
	})
})

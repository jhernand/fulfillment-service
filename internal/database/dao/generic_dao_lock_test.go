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
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	testsv1 "github.com/osac-project/fulfillment-service/internal/api/osac/tests/v1"
	"github.com/osac-project/fulfillment-service/internal/database"
)

var _ = Describe("Lock", func() {
	var generic *GenericDAO[*testsv1.Object]

	BeforeEach(func() {
		var err error

		// Create the DAO:
		generic, err = NewGenericDAO[*testsv1.Object]().
			SetLogger(logger).
			Build()
		Expect(err).ToNot(HaveOccurred())
	})

	// createObject inserts an object directly into the database using auto-commit so that it is
	// visible to all transactions.
	createObject := func(id string, tenant string) {
		_, err := pool.Exec(
			ctx,
			"insert into objects (id, tenant, data) values ($1, $2, '{}')",
			id, tenant,
		)
		Expect(err).ToNot(HaveOccurred())
	}

	// checkLocked verifies that the given identifiers correspond to rows that are currently locked
	// by using 'for update skip locked' to detect locked rows. If a row is locked by another
	// transaction, 'skip locked' will skip it, so an empty result means all the rows are locked.
	checkLocked := func(ids ...string) {
		// This needs to run with its own transaction and context to avoid interference with the main
		// transaction:
		txCtx, txCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer txCancel()
		tx, err := tm.Begin(txCtx)
		Expect(err).ToNot(HaveOccurred())
		defer func() {
			err := tx.End(txCtx)
			Expect(err).ToNot(HaveOccurred())
		}()
		txCtx = database.TxIntoContext(txCtx, tx)

		// Run the query to check if the row is locked:
		rows, err := pool.Query(
			ctx,
			"select id from objects where id = any($1) for update skip locked",
			ids,
		)
		Expect(err).ToNot(HaveOccurred())
		defer rows.Close()
		var unlocked []string
		for rows.Next() {
			var id string
			err = rows.Scan(&id)
			Expect(err).ToNot(HaveOccurred())
			unlocked = append(unlocked, id)
		}
		Expect(rows.Err()).ToNot(HaveOccurred())
		Expect(unlocked).To(BeEmpty())
	}

	// checkNotLocked verifies that the given identifiers correspond to rows that are not locked
	// by using the same 'for update skip locked' technique. If a row is not locked it will be
	// returned, so all the requested rows being returned means none of them are locked.
	checkNotLocked := func(ids ...string) {
		// This needs to run with its own transaction and context to avoid interference with the main
		// transaction:
		txCtx, txCancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer txCancel()
		tx, err := tm.Begin(txCtx)
		Expect(err).ToNot(HaveOccurred())
		defer func() {
			err := tx.End(txCtx)
			Expect(err).ToNot(HaveOccurred())
		}()
		txCtx = database.TxIntoContext(txCtx, tx)

		// Run the query to check if the row is unlocked:
		rows, err := tx.Query(
			txCtx,
			"select id from objects where id = any($1) for update skip locked",
			ids,
		)
		Expect(err).ToNot(HaveOccurred())
		defer rows.Close()
		var unlocked []string
		for rows.Next() {
			var id string
			err = rows.Scan(&id)
			Expect(err).ToNot(HaveOccurred())
			unlocked = append(unlocked, id)
		}
		Expect(rows.Err()).ToNot(HaveOccurred())
		Expect(unlocked).To(ConsistOf(ids))
	}

	It("Locks a single object", func() {
		createObject("obj1", "my_tenant")

		tx, err := tm.Begin(ctx)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			err := tx.End(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
		ctx = database.TxIntoContext(ctx, tx)

		_, err = generic.Lock().
			AddId("obj1").
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())

		checkLocked("obj1")
	})

	It("Locks multiple objects", func() {
		createObject("obj1", "my_tenant")
		createObject("obj2", "my_tenant")
		createObject("obj3", "my_tenant")

		tx, err := tm.Begin(ctx)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			err := tx.End(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
		ctx = database.TxIntoContext(ctx, tx)

		_, err = generic.Lock().
			AddIds("obj1", "obj2", "obj3").
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())

		checkLocked("obj1", "obj2", "obj3")
	})

	It("Fails with not found error when locking non-existent object", func() {
		tx, err := tm.Begin(ctx)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			err := tx.End(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
		ctx = database.TxIntoContext(ctx, tx)

		_, err = generic.Lock().
			AddId("does-not-exist").
			Do(ctx)
		Expect(err).To(HaveOccurred())
		var notFoundErr *ErrNotFound
		Expect(errors.As(err, &notFoundErr)).To(BeTrue())
		Expect(notFoundErr.IDs).To(ConsistOf("does-not-exist"))
	})

	It("Fails when one of multiple objects doesn't exist", func() {
		createObject("obj1", "my_tenant")

		tx, err := tm.Begin(ctx)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			err := tx.End(ctx)
			Expect(err).ToNot(HaveOccurred())
		})
		ctx = database.TxIntoContext(ctx, tx)

		_, err = generic.Lock().
			AddId("obj1").
			AddId("does-not-exist").
			Do(ctx)
		Expect(err).To(HaveOccurred())
		var notFoundErr *ErrNotFound
		Expect(errors.As(err, &notFoundErr)).To(BeTrue())
		Expect(notFoundErr.IDs).To(ConsistOf("does-not-exist"))
	})

	It("Unlocks object when transaction is committed", func() {
		createObject("obj1", "my_tenant")

		tx, err := tm.Begin(ctx)
		Expect(err).ToNot(HaveOccurred())
		ctx = database.TxIntoContext(ctx, tx)
		_, err = generic.Lock().
			AddId("obj1").
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())

		err = tx.End(ctx)
		Expect(err).ToNot(HaveOccurred())
		checkNotLocked("obj1")
	})

	It("Unlocks object when transaction is rolled back", func() {
		createObject("obj1", "my_tenant")

		tx, err := tm.Begin(ctx)
		Expect(err).ToNot(HaveOccurred())
		ctx = database.TxIntoContext(ctx, tx)
		_, err = generic.Lock().
			AddId("obj1").
			Do(ctx)
		Expect(err).ToNot(HaveOccurred())

		rollbackErr := errors.New("force rollback")
		tx.ReportError(&rollbackErr)
		err = tx.End(ctx)
		Expect(err).ToNot(HaveOccurred())
		checkNotLocked("obj1")
	})

	It("Prevents deadlocks by locking in consistent order", func() {
		createObject("a", "my_tenant")
		createObject("b", "my_tenant")

		// Start a transaction and lock 'a' using direct SQL:
		tx1, err := tm.Begin(ctx)
		Expect(err).ToNot(HaveOccurred())
		_, err = tx1.Exec(ctx, "select id from objects where id = 'a' for update")
		Expect(err).ToNot(HaveOccurred())

		// Start a goroutine that tries to lock 'b' and 'a' (in that order) via the DAO. Because the DAO sorts
		// identifiers, it will try to lock 'a' first, which is held by tx1, so it will block before reaching
		// 'b':
		done := make(chan error, 1)
		go func() {
			defer GinkgoRecover()
			tx2, err := tm.Begin(ctx)
			if err != nil {
				done <- err
				return
			}
			tx2Ctx, txCancel := context.WithTimeout(context.Background(), time.Second)
			defer txCancel()
			tx2Ctx = database.TxIntoContext(tx2Ctx, tx2)
			_, lockErr := generic.Lock().
				AddIds("b", "a").
				Do(tx2Ctx)
			err = tx2.End(ctx)
			done <- errors.Join(lockErr, err)
		}()

		// Give the goroutine time to reach the blocking lock on 'a' and verify that it
		// doesn't complete while 'a' is held:
		Consistently(done, 100*time.Millisecond).ShouldNot(Receive())

		// Verify that 'b' is not locked, proving that the DAO tried to lock 'a' first
		// even though 'b' was passed first:
		checkNotLocked("b")

		// Release 'a' by committing, allowing the goroutine to proceed:
		err = tx1.End(ctx)
		Expect(err).ToNot(HaveOccurred())

		// Verify the goroutine completed without error:
		Eventually(done, time.Second).Should(Receive(BeNil()))
	})
})

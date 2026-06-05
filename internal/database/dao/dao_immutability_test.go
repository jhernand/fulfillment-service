/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package dao

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"

	"github.com/osac-project/fulfillment-service/internal/database"
)

var _ = Describe("Immutable columns", func() {
	BeforeEach(func() {
		tx, err := database.TxFromContext(ctx)
		Expect(err).ToNot(HaveOccurred())
		_, err = tx.Exec(ctx, `
			insert into organizations (
				id,
				name,
				tenant,
				data
			)
			values (
				'my-tenant',
				'my-tenant',
				'my-tenant',
				'{}'
			)
		`)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Database trigger", func() {
		It("Rejects update that changes the one immutable column", func() {
			tx, err := database.TxFromContext(ctx)
			Expect(err).ToNot(HaveOccurred())
			_, err = tx.Exec(ctx, "savepoint before_update")
			Expect(err).ToNot(HaveOccurred())
			_, err = tx.Exec(ctx, `
				update organizations set
					name = 'your-tenant'
				where
					id = 'my-tenant'
			`)
			Expect(err).To(HaveOccurred())
			var pgErr *pgconn.PgError
			Expect(errors.As(err, &pgErr)).To(BeTrue())
			Expect(pgErr.Code).To(Equal(errImmutableCode))
			Expect(pgErr.Detail).To(MatchJSON(`[
				"name"
			]`))
			Expect(pgErr.Message).To(Equal(
				"column 'name' of table 'organizations' is immutable",
			))
			_, err = tx.Exec(ctx, "rollback to savepoint before_update")
			Expect(err).ToNot(HaveOccurred())
		})

		It("Rejects update that changes two immutable columns", func() {
			tx, err := database.TxFromContext(ctx)
			Expect(err).ToNot(HaveOccurred())
			_, err = tx.Exec(ctx, "savepoint before_update")
			Expect(err).ToNot(HaveOccurred())
			_, err = tx.Exec(ctx, `
				update organizations set
					name = 'your-name',
					tenant = 'your-tenant'
				where
					id = 'my-tenant'
			`)
			Expect(err).To(HaveOccurred())
			var pgErr *pgconn.PgError
			Expect(errors.As(err, &pgErr)).To(BeTrue())
			Expect(pgErr.Code).To(Equal(errImmutableCode))
			Expect(pgErr.Detail).To(MatchJSON(`[
				"name",
				"tenant"
			]`))
			Expect(pgErr.Message).To(Equal(
				"columns 'name' and 'tenant' of table 'organizations' are immutable",
			))
			_, err = tx.Exec(ctx, "rollback to savepoint before_update")
			Expect(err).ToNot(HaveOccurred())
		})

		It("Allows update that includes but doesn't change an immutable column", func() {
			tx, err := database.TxFromContext(ctx)
			Expect(err).ToNot(HaveOccurred())
			_, err = tx.Exec(ctx, `
				update organizations set
					name = 'my-tenant'
				where
					id = 'my-tenant'
			`)
			Expect(err).ToNot(HaveOccurred())
		})

		It("Allows update of other columns", func() {
			tx, err := database.TxFromContext(ctx)
			Expect(err).ToNot(HaveOccurred())
			_, err = tx.Exec(ctx, `
				update organizations set
					labels = '{"my-label": "my-value"}'::jsonb
				where
					id = 'my-tenant'
			`)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})

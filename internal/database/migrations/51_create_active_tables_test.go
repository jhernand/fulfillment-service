/*
Copyright (c) 2026 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package migrations

import (
	"context"

	. "github.com/onsi/ginkgo/v2/dsl/core"
	. "github.com/onsi/gomega"
)

var _ = DescribeMigration("Create active tables", func() {
	It("Creates the 'materialize_active_objects' function", func(ctx context.Context) {
		err := tool.Migrate(ctx, 51)
		Expect(err).ToNot(HaveOccurred())

		var count int
		err = conn.QueryRow(ctx, `
			select count(*)
			from information_schema.routines
			where routine_name = 'materialize_active_objects'
			  and routine_type = 'FUNCTION'
		`).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(1))
	})

	It("Creates an active table for each object table", func(ctx context.Context) {
		err := tool.Migrate(ctx, 51)
		Expect(err).ToNot(HaveOccurred())

		rows, err := conn.Query(ctx, `
			select c.relname
			from pg_catalog.pg_class c
			join pg_catalog.pg_namespace n on n.oid = c.relnamespace
			where n.nspname = 'public'
			  and c.relkind = 'r'
			  and c.relname like 'active_%'
			order by c.relname
		`)
		Expect(err).ToNot(HaveOccurred())
		defer rows.Close()

		var activeTables []string
		for rows.Next() {
			var name string
			err = rows.Scan(&name)
			Expect(err).ToNot(HaveOccurred())
			activeTables = append(activeTables, name)
		}
		Expect(rows.Err()).ToNot(HaveOccurred())

		Expect(activeTables).To(ContainElements(
			"active_clusters",
			"active_organizations",
			"active_users",
			"active_projects",
		))
	})

	It("Attaches a trigger to each object table", func(ctx context.Context) {
		err := tool.Migrate(ctx, 51)
		Expect(err).ToNot(HaveOccurred())

		var count int
		err = conn.QueryRow(ctx, `
			select count(*)
			from information_schema.triggers
			where trigger_name = 'materialize_active_objects'
			  and event_object_table = 'clusters'
		`).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(BeNumerically(">", 0))
	})

	It("Backfills existing active objects", func(ctx context.Context) {
		err := tool.Migrate(ctx, 51)
		Expect(err).ToNot(HaveOccurred())

		// The builtin organizations inserted by migration 48 should have been backfilled into the
		// active table because they have the default deletion_timestamp ('epoch').
		var count int
		err = conn.QueryRow(ctx, `
			select count(*) from active_organizations
		`).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(BeNumerically(">=", 2))
	})

	It("Adds a row to the active table when an object is inserted", func(ctx context.Context) {
		err := tool.Migrate(ctx, 51)
		Expect(err).ToNot(HaveOccurred())

		_, err = conn.Exec(ctx, `
			insert into clusters (id, name, tenant, data)
			values ('my-cluster', 'my-cluster', 'system', '{}')
		`)
		Expect(err).ToNot(HaveOccurred())

		var count int
		err = conn.QueryRow(ctx, `
			select count(*) from active_clusters where id = 'my-cluster'
		`).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(1))
	})

	It("Does not add a row when an object is inserted with a deletion timestamp", func(ctx context.Context) {
		err := tool.Migrate(ctx, 51)
		Expect(err).ToNot(HaveOccurred())

		_, err = conn.Exec(ctx, `
			insert into clusters (id, name, tenant, deletion_timestamp, data)
			values ('deleted-cluster', 'deleted-cluster', 'system', now(), '{}')
		`)
		Expect(err).ToNot(HaveOccurred())

		var count int
		err = conn.QueryRow(ctx, `
			select count(*) from active_clusters where id = 'deleted-cluster'
		`).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(0))
	})

	It("Removes the row from the active table when an object is soft-deleted", func(ctx context.Context) {
		err := tool.Migrate(ctx, 51)
		Expect(err).ToNot(HaveOccurred())

		_, err = conn.Exec(ctx, `
			insert into clusters (id, name, tenant, data)
			values ('my-cluster', 'my-cluster', 'system', '{}')
		`)
		Expect(err).ToNot(HaveOccurred())

		_, err = conn.Exec(ctx, `
			update clusters set deletion_timestamp = now() where id = 'my-cluster'
		`)
		Expect(err).ToNot(HaveOccurred())

		var count int
		err = conn.QueryRow(ctx, `
			select count(*) from active_clusters where id = 'my-cluster'
		`).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(0))
	})

	It("Adds the row back when a soft-deleted object is restored", func(ctx context.Context) {
		err := tool.Migrate(ctx, 51)
		Expect(err).ToNot(HaveOccurred())

		_, err = conn.Exec(ctx, `
			insert into clusters (id, name, tenant, data)
			values ('my-cluster', 'my-cluster', 'system', '{}')
		`)
		Expect(err).ToNot(HaveOccurred())

		_, err = conn.Exec(ctx, `
			update clusters set deletion_timestamp = now() where id = 'my-cluster'
		`)
		Expect(err).ToNot(HaveOccurred())

		_, err = conn.Exec(ctx, `
			update clusters set deletion_timestamp = 'epoch' where id = 'my-cluster'
		`)
		Expect(err).ToNot(HaveOccurred())

		var count int
		err = conn.QueryRow(ctx, `
			select count(*) from active_clusters where id = 'my-cluster'
		`).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(1))
	})

	It("Removes the row from the active table when an object is deleted", func(ctx context.Context) {
		err := tool.Migrate(ctx, 51)
		Expect(err).ToNot(HaveOccurred())

		_, err = conn.Exec(ctx, `
			insert into clusters (id, name, tenant, data)
			values ('my-cluster', 'my-cluster', 'system', '{}')
		`)
		Expect(err).ToNot(HaveOccurred())

		_, err = conn.Exec(ctx, `
			delete from clusters where id = 'my-cluster'
		`)
		Expect(err).ToNot(HaveOccurred())

		var count int
		err = conn.QueryRow(ctx, `
			select count(*) from active_clusters where id = 'my-cluster'
		`).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(0))
	})

	It("Does not change the active table when updating other columns", func(ctx context.Context) {
		err := tool.Migrate(ctx, 51)
		Expect(err).ToNot(HaveOccurred())

		_, err = conn.Exec(ctx, `
			insert into clusters (id, name, tenant, data)
			values ('my-cluster', 'my-cluster', 'system', '{}')
		`)
		Expect(err).ToNot(HaveOccurred())

		_, err = conn.Exec(ctx, `
			update clusters set data = '{"spec":{}}' where id = 'my-cluster'
		`)
		Expect(err).ToNot(HaveOccurred())

		var count int
		err = conn.QueryRow(ctx, `
			select count(*) from active_clusters where id = 'my-cluster'
		`).Scan(&count)
		Expect(err).ToNot(HaveOccurred())
		Expect(count).To(Equal(1))
	})
})

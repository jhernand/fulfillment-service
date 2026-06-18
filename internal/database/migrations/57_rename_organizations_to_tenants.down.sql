--
-- Copyright (c) 2026 Red Hat Inc.
--
-- Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with
-- the License. You may obtain a copy of the License at
--
--   http://www.apache.org/licenses/LICENSE-2.0
--
-- Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on
-- an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the
-- specific language governing permissions and limitations under the License.
--

-- Revert the tenants table rename back to organizations.

-- Drop the immutable column trigger:
drop trigger check_immutable_columns on tenants;

-- Drop all foreign key constraints that reference tenants(id):
do $$
declare
  r record;
begin
  for r in
    select
      con.conname as constraint_name,
      c.relname as table_name
    from
      pg_catalog.pg_constraint con
    join
      pg_catalog.pg_class c on c.oid = con.conrelid
    join
      pg_catalog.pg_class fc on fc.oid = con.confrelid
    join
      pg_catalog.pg_namespace fn on fn.oid = fc.relnamespace
    where
      con.contype = 'f' and
      fn.nspname = 'public' and
      fc.relname = 'tenants'
  loop
    execute format('alter table %I drop constraint %I', r.table_name, r.constraint_name);
  end loop;
end;
$$;

-- Rename the tables back:
alter table tenants rename to organizations;
alter table archived_tenants rename to archived_organizations;

-- Rename the indexes back:
alter index tenants_by_name rename to organizations_by_name;
alter index tenants_by_owner rename to organizations_by_owner;
alter index tenants_by_tenant rename to organizations_by_tenant;
alter index tenants_by_label rename to organizations_by_label;

-- Re-create the immutable column trigger on the original table:
create trigger check_immutable_columns
  before update on organizations
  for each row
  execute function check_immutable_columns('name', 'tenant');

-- Re-create all tenant foreign key constraints referencing organizations(id):
do $$
declare
  t text;
begin
  for t in
    select
      c.table_name
    from
      information_schema.columns c
    join
      information_schema.tables tb on
        tb.table_schema = c.table_schema and
        tb.table_name = c.table_name
    where
      c.table_schema = 'public' and
      c.column_name = 'tenant' and
      tb.table_type = 'BASE TABLE' and
      c.table_name not like 'archived_%' and
      c.table_name not in ('notifications', 'schema_migrations')
    order by
      c.table_name
  loop
    execute format(
      'alter table %I add constraint %I foreign key (tenant) references organizations (id)',
      t, t || '_tenant_fk'
    );
  end loop;
end;
$$;

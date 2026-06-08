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

-- This migration creates "active" tables for every object table. Each active table contains only the identifiers of
-- objects that have not been soft-deleted (whose deletion_timestamp is 'epoch'). A single reusable trigger function
-- maintains these tables automatically on insert, update, and delete operations.

-- Create the trigger function. It uses TG_TABLE_NAME to derive the name of the corresponding active table
-- dynamically, so a single function serves all object tables.
create function materialize_active_objects() returns trigger as $$
declare
  active_table text := 'active_' || tg_table_name;
begin
  if tg_op = 'DELETE' then
    execute format('delete from %I where id = $1', active_table) using old.id;
    return old;
  end if;

  if tg_op = 'INSERT' then
    if new.deletion_timestamp = 'epoch'::timestamptz then
      execute format('insert into %I (id) values ($1)', active_table) using new.id;
    end if;
    return new;
  end if;

  if tg_op = 'UPDATE' then
    if old.deletion_timestamp = 'epoch'::timestamptz and new.deletion_timestamp != 'epoch'::timestamptz then
      execute format('delete from %I where id = $1', active_table) using new.id;
    elsif old.deletion_timestamp != 'epoch'::timestamptz and new.deletion_timestamp = 'epoch'::timestamptz then
      execute format('insert into %I (id) values ($1)', active_table) using new.id;
    end if;
    return new;
  end if;

  return null;
end;
$$ language plpgsql;

-- Create the active tables, attach the trigger, and backfill existing data. This procedure dynamically discovers
-- all object tables and creates a corresponding active table for each one.
create procedure create_active_tables() language plpgsql as $$
declare
  t text;
begin
  for t in
    select c.relname
    from pg_catalog.pg_class c
    join pg_catalog.pg_namespace n on n.oid = c.relnamespace
    where n.nspname = 'public'
      and c.relkind = 'r'
      and c.relname not like 'archived_%'
      and c.relname not like 'active_%'
      and c.relname not in ('notifications', 'schema_migrations')
    order by c.relname
  loop
    execute format(
      'create table %I ('
        'id text not null primary key, '
        'constraint %I foreign key (id) references %I (id) on delete cascade'
      ')',
      'active_' || t,
      'active_' || t || '_fk',
      t
    );

    execute format(
      'create trigger materialize_active_objects '
        'after insert or update or delete on %I '
        'for each row execute function materialize_active_objects()',
      t
    );

    execute format(
      'insert into %I (id) select id from %I where deletion_timestamp = ''epoch''',
      'active_' || t, t
    );
  end loop;
end;
$$;

call create_active_tables();
drop procedure create_active_tables();

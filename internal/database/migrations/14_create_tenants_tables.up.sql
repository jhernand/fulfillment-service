--
-- Copyright (c) 2025 Red Hat Inc.
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

-- Create the tenants tables:
--
create table tenants (
  id text not null primary key,
  name text not null default '',
  creation_timestamp timestamp with time zone not null default now(),
  deletion_timestamp timestamp with time zone not null default 'epoch',
  finalizers text[] not null default '{}',
  creators text[] not null default '{}',
  tenants text[] not null default '{}',
  data jsonb not null
);

create table archived_tenants (
  id text not null,
  name text not null default '',
  creation_timestamp timestamp with time zone not null,
  deletion_timestamp timestamp with time zone not null,
  archival_timestamp timestamp with time zone not null default now(),
  creators text[] not null default '{}',
  tenants text[] not null default '{}',
  data jsonb not null
);

-- Create indexes for efficient querying:
create index tenants_by_name on tenants (name);
create index tenants_by_owner on tenants using gin (creators);
create index tenants_by_tenant on tenants using gin (tenants);

-- Create a function that automatically creates tenant records when rows are inserted or updated in other tables. For
-- each value in the tenants column, if a tenant with that name doesn't exist, a new tenant record is created with a
-- randomly generated UUID as the id.
create or replace function auto_create_tenant()
returns trigger as $$
declare
  tenant_name text;
  new_id text;
begin
  foreach tenant_name in array NEW.tenants loop
    if not exists (select 1 from tenants where name = tenant_name) then
      new_id := gen_random_uuid()::text;
      insert into tenants (id, name, tenants, data)
      values (
        new_id,
        tenant_name,
        array[tenant_name],
        jsonb_build_object(
          'id', new_id,
          'metadata', jsonb_build_object(
            'name', tenant_name,
            'tenants', jsonb_build_array(tenant_name)
          )
        )
      );
    end if;
  end loop;
  return NEW;
end;
$$ language plpgsql;

-- Create triggers on all tables that have a tenants column:
create trigger auto_create_tenant_on_cluster_templates
  before insert or update on cluster_templates
  for each row execute function auto_create_tenant();

create trigger auto_create_tenant_on_clusters
  before insert or update on clusters
  for each row execute function auto_create_tenant();

create trigger auto_create_tenant_on_host_classes
  before insert or update on host_classes
  for each row execute function auto_create_tenant();

create trigger auto_create_tenant_on_hubs
  before insert or update on hubs
  for each row execute function auto_create_tenant();

create trigger auto_create_tenant_on_virtual_machine_templates
  before insert or update on virtual_machine_templates
  for each row execute function auto_create_tenant();

create trigger auto_create_tenant_on_virtual_machines
  before insert or update on virtual_machines
  for each row execute function auto_create_tenant();

create trigger auto_create_tenant_on_hosts
  before insert or update on hosts
  for each row execute function auto_create_tenant();

create trigger auto_create_tenant_on_host_pools
  before insert or update on host_pools
  for each row execute function auto_create_tenant();

-- Populate tenants from existing data in all tables. This collects all unique tenant names from all tables and creates
-- tenant records for those that don't already exist.
do $$
declare
  tenant_name text;
  new_id text;
begin
  for tenant_name in
    select distinct unnest(tenants) as name from cluster_templates
    union
    select distinct unnest(tenants) as name from clusters
    union
    select distinct unnest(tenants) as name from host_classes
    union
    select distinct unnest(tenants) as name from hubs
    union
    select distinct unnest(tenants) as name from virtual_machine_templates
    union
    select distinct unnest(tenants) as name from virtual_machines
    union
    select distinct unnest(tenants) as name from hosts
    union
    select distinct unnest(tenants) as name from host_pools
  loop
    if not exists (select 1 from tenants where name = tenant_name) then
      new_id := gen_random_uuid()::text;
      insert into tenants (id, name, tenants, data)
      values (
        new_id,
        tenant_name,
        array[tenant_name],
        jsonb_build_object(
          'id', new_id,
          'metadata', jsonb_build_object(
            'name', tenant_name,
            'tenants', jsonb_build_array(tenant_name)
          )
        )
      );
    end if;
  end loop;
end;
$$;

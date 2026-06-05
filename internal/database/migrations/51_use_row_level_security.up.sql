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

-- Helper function that returns true when the given tenant is visible to the current session. The interceptor sets the
-- 'auth.tenants' session variable to a JSON array of allowed tenant identifiers, or to the string '*' when the subject
-- has access to all tenants. When the variable is absent or empty the function returns false, denying access by
-- default.
create or replace function check_visibility(tenant text) returns boolean as $$
declare
  raw text := current_setting('auth.tenants', true);
begin
  if raw is null or raw = '' then
    return false;
  end if;
  if raw = '*' then
    return true;
  end if;
  return raw::jsonb ? tenant;
exception
  when others then return false;
end;
$$ language plpgsql stable;

-- Enables row-level security on the given table, creating policies that delegate visibility checks to
-- 'check_visibility'. This procedure is kept after the migration so that future migrations can call it when adding new
-- tables.
create or replace procedure enable_row_level_security(t text) language plpgsql as $$
begin
  execute format('alter table %I enable row level security', t);
  execute format('alter table %I force row level security', t);
  execute format(
    'create policy %I on %I as permissive for select using (check_visibility(tenant))',
    'view_' || t, t
  );
  execute format(
    'create policy %I on %I as permissive for insert with check (true)',
    'create_' || t, t
  );
  execute format(
    'create policy %I on %I as permissive for update using (check_visibility(tenant))',
    'update_' || t, t
  );
  execute format(
    'create policy %I on %I as permissive for delete using (check_visibility(tenant))',
    'delete_' || t, t
  );
end;
$$;

-- Temporary procedure that finds all object tables with a 'tenant' column and applies row-level security to each.
create procedure enable_row_level_security_all() language plpgsql as $$
declare
  t text;
begin
  for t in
    select
      c.relname
    from
      pg_class c
    join
      pg_namespace n on n.oid = c.relnamespace
    join
      pg_attribute a on a.attrelid = c.oid
    where
      n.nspname = 'public' and
      c.relkind = 'r' and
      a.attname = 'tenant' and
      a.attnum > 0 and
      not a.attisdropped and
      c.relname not like 'archived_%'
    order by
      c.relname
  loop
    call enable_row_level_security(t);
  end loop;
end;
$$;

call enable_row_level_security_all();

drop procedure enable_row_level_security_all();

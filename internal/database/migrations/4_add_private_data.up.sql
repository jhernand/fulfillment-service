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

--
-- Move tables out of the private schema:
--
alter table private.hubs set schema public;

--
-- Add the `private_data` column to all the tables:
--
alter table cluster_orders add column if not exists private_data jsonb not null default '{}';
alter table cluster_templates add column if not exists private_data jsonb not null default '{}';
alter table clusters add column if not exists private_data jsonb not null default '{}';
alter table hubs add column if not exists private_data jsonb not null default '{}';

--
-- Drop the triggers that automatically create the private data:
--
drop trigger create_empty_private_cluster_order on cluster_orders;
drop function private.create_empty_cluster_order;
drop trigger create_empty_private_cluster on clusters;
drop function private.create_empty_cluster;

--
-- Move the private data from the private tables to the public tables:
--
update cluster_orders as public set private_data = (select private_data from private.cluster_orders as private where private.id = public.id);
update clusters as public set private_data = (select private_data from private.clusters as private where private.id = public.id);

--
-- Drop the private tables and schema:
--
drop table private.cluster_orders;
drop table private.clusters;
drop schema private;

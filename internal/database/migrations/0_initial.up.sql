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

create table cluster_templates (
  id text not null primary key,
  data jsonb not null
);

create table clusters (
  id text not null primary key,
  data jsonb not null
);

create table cluster_orders (
  id text not null primary key,
  data jsonb not null
);

create schema admin;

create table admin.cluster_orders (
  id text not null primary key,
  data jsonb not null
);

create table admin.hubs (
  id text not null primary key,
  data jsonb not null
);

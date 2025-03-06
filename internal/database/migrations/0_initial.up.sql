create table cluster_templates (
  id uuid not null primary key,
  title text not null,
  description text not null
);

create table cluster_order_states (
  id text not null primary key
);

create table clusters (
  id uuid not null primary key,
  api_url text not null,
  console_url text not null
);

insert into cluster_order_states (id)
values
  ('UNSPECIFIED'),
  ('ACCEPTED'),
  ('REJECTED'),
  ('CANCELED'),
  ('FULFILLED'),
  ('FAILED');

-- This table contains the details of cluster orders, excepts those that are only available when the order has been
-- fulfilled. Those details are in the 'fulfilled_cluster_orders' table.
create table cluster_orders (
  id uuid not null primary key,
  template_id uuid not null,
  state text not null default 'UNSPECIFIED',
  constraint fk_template foreign key (template_id) references cluster_templates (id),
  constraint fk_state foreign key (state) references cluster_order_states (id)
);

-- This table contains the relationship between orders that have been fulfilled and the resulting clusters.
create table fulfilled_cluster_orders (
   order_id uuid not null unique,
   cluster_id uuid not null unique,
   primary key (order_id, cluster_id),
   constraint fk_order foreign key (order_id) references cluster_orders (id),
   constraint fk_cluster foreign key (cluster_id) references clusters (id)
);

create table if not exists schema_migrations (
  version text primary key,
  applied_at text not null
);

create table if not exists api_keys (
  id text primary key,
  name text not null,
  prefix text not null unique,
  key_hash text not null,
  scopes text not null,
  enabled integer not null default 1,
  created_at text not null,
  updated_at text not null,
  last_used_at text,
  revoked_at text
);

create index if not exists idx_api_keys_prefix on api_keys(prefix);
create index if not exists idx_api_keys_enabled on api_keys(enabled);

create table if not exists endpoints (
  id text primary key,
  host text not null unique,
  upstream_url text not null,
  enabled integer not null default 1,
  health_path text not null default '/health',
  last_health_status text,
  last_health_status_code integer,
  last_health_latency_ms integer,
  last_health_error text,
  last_health_checked_at text,
  created_at text not null,
  updated_at text not null
);

create table if not exists endpoint_health_checks (
  id integer primary key autoincrement,
  endpoint_id text not null references endpoints(id),
  status text not null,
  status_code integer,
  latency_ms integer,
  error text,
  checked_at text not null
);

create index if not exists idx_endpoint_health_checks_endpoint_time
  on endpoint_health_checks(endpoint_id, checked_at);

create table if not exists usage_logs (
  id integer primary key autoincrement,
  request_id text not null unique,
  api_key_id text references api_keys(id),
  api_key_prefix text,
  endpoint_id text not null references endpoints(id),
  host text not null,
  method text not null,
  path text not null,
  model text,
  streaming integer not null default 0,
  usage_missing integer not null default 0,
  prompt_tokens integer,
  completion_tokens integer,
  total_tokens integer,
  status_code integer not null,
  upstream_status_code integer,
  latency_ms integer not null,
  client_ip text,
  user_agent text,
  error text,
  created_at text not null
);

create index if not exists idx_usage_logs_created_at on usage_logs(created_at);
create index if not exists idx_usage_logs_api_key_time on usage_logs(api_key_id, created_at);
create index if not exists idx_usage_logs_endpoint_time on usage_logs(endpoint_id, created_at);
create index if not exists idx_usage_logs_model_time on usage_logs(model, created_at);

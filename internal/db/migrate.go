package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Yan-Yu-Lin/rtx-gateway/internal/config"
)

const initialMigration = `
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
	`

const securityMigration = `
create table if not exists ip_bans (
  id integer primary key autoincrement,
  client_ip text not null,
  reason text not null,
  strikes integer not null default 1,
  banned_until text not null,
  manual integer not null default 0,
  created_at text not null,
  updated_at text not null,
  lifted_at text
);

create index if not exists idx_ip_bans_active
  on ip_bans(client_ip, banned_until, lifted_at);

create table if not exists security_events (
  id integer primary key autoincrement,
  client_ip text not null,
  event_type text not null,
  host text,
  path text,
  status_code integer,
  detail text,
  created_at text not null
);

create index if not exists idx_security_events_ip_time
  on security_events(client_ip, created_at);
create index if not exists idx_security_events_type_time
  on security_events(event_type, created_at);
`

func Migrate(ctx context.Context, database *sql.DB, endpoints []config.Endpoint) error {
	if _, err := database.ExecContext(ctx, initialMigration); err != nil {
		return fmt.Errorf("apply initial migration: %w", err)
	}

	if _, err := database.ExecContext(
		ctx,
		"insert or ignore into schema_migrations (version, applied_at) values (?, ?)",
		"001_initial",
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}

	if _, err := database.ExecContext(ctx, securityMigration); err != nil {
		return fmt.Errorf("apply security migration: %w", err)
	}

	if _, err := database.ExecContext(
		ctx,
		"insert or ignore into schema_migrations (version, applied_at) values (?, ?)",
		"002_security",
		time.Now().UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("record security migration: %w", err)
	}

	for _, endpoint := range endpoints {
		if err := upsertEndpoint(ctx, database, endpoint); err != nil {
			return err
		}
	}

	return nil
}

func upsertEndpoint(ctx context.Context, database *sql.DB, endpoint config.Endpoint) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := database.ExecContext(
		ctx,
		`insert into endpoints (id, host, upstream_url, enabled, health_path, created_at, updated_at)
		 values (?, ?, ?, 1, '/health', ?, ?)
		 on conflict(id) do update set
		   host = excluded.host,
		   upstream_url = excluded.upstream_url,
		   updated_at = excluded.updated_at`,
		endpoint.ID,
		strings.ToLower(endpoint.Host),
		endpoint.UpstreamURL.String(),
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("upsert endpoint %s: %w", endpoint.ID, err)
	}
	return nil
}

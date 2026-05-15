# RTX Gateway Implementation Plan

This project replaces nginx hardcoded bearer-token auth for RTXWS public vLLM endpoints with a small Go gateway that performs API key validation, proxying, usage logging, and admin operations.

## Current Target Topology

```text
internet -> nginx TLS -> 127.0.0.1:9188 Go public proxy
  -> 127.0.0.1:9180 RTXWS Gemma 4 vLLM
  -> 127.0.0.1:9183 RTXWS Chandra OCR vLLM

gateway.arthurlin.dev -> nginx -> 127.0.0.1:3008 Nuxt dashboard
gateway.arthurlin.dev /api/* -> nginx -> 127.0.0.1:9189 Go admin API
```

The gateway routes public API requests by `Host`:

- `rtx-llm.arthurlin.dev` -> `http://127.0.0.1:9180`
- `rtx-ocr.arthurlin.dev` -> `http://127.0.0.1:9183`

## Proposed Repository Layout

```text
.
├── cmd/
│   └── rtx-gateway/
│       └── main.go
├── internal/
│   ├── admin/          # admin API handlers
│   ├── auth/           # API key parsing, hashing, permission checks
│   ├── config/         # env parsing and defaults
│   ├── db/             # SQLite connection, migrations, query helpers
│   ├── health/         # endpoint health checks
│   ├── proxy/          # public reverse proxy and SSE handling
│   └── usage/          # response usage extraction and log records
├── migrations/         # SQLite schema migrations
├── dashboard/          # Nuxt 4 app, created in Phase 4
├── deploy/
│   ├── nginx/
│   └── systemd/
├── README.md
├── PLAN.md
└── go.mod
```

## Core Design Decisions

### Go Service Shape

The Go binary should run two HTTP servers:

- public proxy: `127.0.0.1:9188`
- admin API: `127.0.0.1:9189`

This keeps the public API and admin API separated at the listener level while still using one binary and one DB connection pool.

### SQLite

Use SQLite with WAL mode.

Startup PRAGMAs:

```sql
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;
PRAGMA busy_timeout=5000;
```

The Go service should own writes. The Nuxt dashboard should call the admin API rather than writing to SQLite directly.

### API Keys

Format:

```text
rtx_live_<8-char-prefix>_<32-char-secret>
```

Storage:

- store the 8-character prefix for lookup
- store only a keyed hash of the full API key
- compare with constant-time comparison
- show the raw key only once at creation time

Recommended hash:

```text
HMAC-SHA256(full_key, RTX_GATEWAY_KEY_PEPPER)
```

The key has enough entropy that a plain SHA-256 hash would be acceptable, but a server-side pepper is cheap insurance.

### Permissions

Each key has scopes:

```json
["llm", "ocr"]
```

The public proxy maps the incoming host to an endpoint id and checks that the key has that scope.

### Streaming

For `text/event-stream` responses:

- forward chunks immediately
- parse `data: ...` SSE lines as they pass through
- look for an OpenAI-compatible chunk with a `usage` object
- log usage at EOF or `[DONE]`
- if no usage appears, log `usage_missing=true` and leave token counts null

Do not buffer the whole stream.

### Large OCR Payloads

OCR requests can contain 5-10 MB base64 images. The public proxy should stream request bodies to the upstream and avoid logging bodies.

Set max body limits in both nginx and Go. Start with 64 MiB:

```text
RTX_GATEWAY_MAX_BODY_BYTES=67108864
```

### Error Handling

If upstream is down:

- return `502 Bad Gateway` for connection failures
- return `504 Gateway Timeout` for upstream timeouts
- log the request with `error`, `status_code`, `upstream_status_code` when available, and `latency_ms`
- do not consume API key quota for auth failures, but do log failed auth attempts in a separate security log or with `api_key_id=NULL`

## Database Schema

Initial schema should live in `migrations/001_initial.sql`.

### `api_keys`

```sql
create table api_keys (
  id text primary key,
  name text not null,
  prefix text not null unique,
  key_hash text not null,
  scopes text not null, -- JSON array: ["llm", "ocr"]
  enabled integer not null default 1,
  created_at text not null,
  updated_at text not null,
  last_used_at text,
  revoked_at text
);

create index idx_api_keys_prefix on api_keys(prefix);
create index idx_api_keys_enabled on api_keys(enabled);
```

### `endpoints`

```sql
create table endpoints (
  id text primary key, -- llm, ocr
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
```

### `endpoint_health_checks`

```sql
create table endpoint_health_checks (
  id integer primary key autoincrement,
  endpoint_id text not null references endpoints(id),
  status text not null, -- healthy, unhealthy
  status_code integer,
  latency_ms integer,
  error text,
  checked_at text not null
);

create index idx_endpoint_health_checks_endpoint_time
  on endpoint_health_checks(endpoint_id, checked_at);
```

### `usage_logs`

```sql
create table usage_logs (
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

create index idx_usage_logs_created_at on usage_logs(created_at);
create index idx_usage_logs_api_key_time on usage_logs(api_key_id, created_at);
create index idx_usage_logs_endpoint_time on usage_logs(endpoint_id, created_at);
create index idx_usage_logs_model_time on usage_logs(model, created_at);
```

## Phase 1: Go Proxy, Auth, Forwarding, Basic Logging

Goal: replace nginx bearer-token auth with Go API key auth and forwarding.

### Files to Create

- `cmd/rtx-gateway/main.go`
- `internal/config/config.go`
- `internal/db/db.go`
- `internal/db/migrate.go`
- `internal/auth/keys.go`
- `internal/auth/middleware.go`
- `internal/proxy/router.go`
- `internal/proxy/proxy.go`
- `internal/usage/log.go`
- `migrations/001_initial.sql`

### Behavior

Public requests:

```http
GET /v1/models
POST /v1/chat/completions
Authorization: Bearer rtx_live_...
Host: rtx-llm.arthurlin.dev
```

Flow:

1. Resolve endpoint from `Host`.
2. Validate bearer API key.
3. Check key scope includes endpoint id.
4. Forward to upstream.
5. Return upstream response.
6. Log request metadata without token usage initially.

### Basic Public Error Responses

Missing auth:

```json
{ "error": { "message": "missing bearer token", "type": "auth_error" } }
```

Invalid key:

```json
{ "error": { "message": "invalid API key", "type": "auth_error" } }
```

Forbidden endpoint:

```json
{ "error": { "message": "API key is not allowed to access this endpoint", "type": "permission_error" } }
```

Upstream down:

```json
{ "error": { "message": "upstream unavailable", "type": "upstream_error" } }
```

### Tests

- Unit test API key parser.
- Unit test HMAC hash and constant-time validation.
- Unit test host-to-endpoint routing.
- Integration test proxying with `httptest.Server`.
- Manual curl:

```bash
curl -H "Authorization: Bearer $KEY" http://127.0.0.1:9188/v1/models -H "Host: rtx-llm.arthurlin.dev"
```

## Phase 2: Usage Extraction

Goal: capture token usage from OpenAI-compatible responses.

### Files to Create or Extend

- `internal/usage/extract.go`
- `internal/proxy/response.go`
- `internal/proxy/sse.go`
- `internal/proxy/proxy.go`

### Non-Streaming Responses

For JSON responses, parse:

```json
{
  "model": "gemma-4",
  "usage": {
    "prompt_tokens": 123,
    "completion_tokens": 456,
    "total_tokens": 579
  }
}
```

The proxy may read the full response body, extract usage, then write the original bytes to the client.

### Streaming Responses

For SSE:

```text
data: {"choices":[...]}

data: {"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}

data: [DONE]
```

Forward each chunk immediately. Parse only the `data:` payloads. The final usage chunk may be absent unless the client requested:

```json
"stream_options": { "include_usage": true }
```

Phase 2 should log null usage when missing. Do not mutate request bodies yet.

### Tests

- Unit test usage extraction from normal JSON.
- Unit test SSE parser with final usage chunk.
- Unit test SSE parser without final usage chunk.
- Integration test that streaming chunks flush before completion.

## Phase 3: Admin API

Goal: API key CRUD, usage queries, and health endpoints.

### Files to Create

- `internal/admin/router.go`
- `internal/admin/auth.go`
- `internal/admin/keys.go`
- `internal/admin/usage.go`
- `internal/admin/health.go`
- `internal/health/checker.go`

### Admin Auth

Start pragmatic:

- Admin API requires `Authorization: Bearer $RTX_GATEWAY_ADMIN_TOKEN`.
- The token is only used by trusted dashboard server routes or direct Arthur operations.
- Browser-facing session auth can be added in the Nuxt layer in Phase 4.

Do not expose `RTX_GATEWAY_ADMIN_TOKEN` to browser JavaScript.

### Admin Endpoints

Create key:

```http
POST /admin/v1/keys
Authorization: Bearer <admin-token>
Content-Type: application/json

{
  "name": "PII platform dev",
  "scopes": ["ocr"]
}
```

Response:

```json
{
  "id": "key_...",
  "name": "PII platform dev",
  "prefix": "a1b2c3d4",
  "key": "rtx_live_a1b2c3d4_<secret>",
  "scopes": ["ocr"],
  "created_at": "2026-05-15T00:00:00Z"
}
```

List keys:

```http
GET /admin/v1/keys
```

Revoke key:

```http
POST /admin/v1/keys/{id}/revoke
```

Usage summary:

```http
GET /admin/v1/usage/summary?from=2026-05-01T00:00:00Z&to=2026-05-16T00:00:00Z&group_by=day
```

Recent requests:

```http
GET /admin/v1/usage/requests?limit=100&endpoint=ocr&api_key_id=key_...
```

Endpoint health:

```http
GET /admin/v1/health
POST /admin/v1/health/check
```

### Health Checks

Health checks should hit:

- `http://127.0.0.1:9180/health`
- `http://127.0.0.1:9183/health`

Store the latest result in `endpoints`, and optionally append history to `endpoint_health_checks`.

For Phase 3, health checks can run:

- on admin request
- periodically inside the Go process every 30 seconds

The periodic checker should never block public proxy requests.

### Tests

- Admin auth success/failure.
- Create key returns raw key once.
- Stored key hash validates but raw key is not stored.
- Revoke key blocks public proxy access.
- Usage summary SQL tests with fixture rows.
- Health check tests with `httptest.Server`.

## Phase 4: Nuxt Dashboard

Goal: browser UI for key management and usage visibility.

### Files to Create

- `dashboard/` Nuxt 4 app
- `dashboard/pages/login.vue`
- `dashboard/pages/index.vue`
- `dashboard/pages/keys.vue`
- `dashboard/pages/usage.vue`
- `dashboard/pages/health.vue`
- `dashboard/server/api/*` proxy routes to Go admin API

### Dashboard Auth

Use Nuxt server-side auth:

- Arthur logs in with a password or passphrase.
- Nuxt sets an HttpOnly session cookie.
- Nuxt server API routes call Go admin API with `RTX_GATEWAY_ADMIN_TOKEN`.
- Browser never sees the Go admin token.

This keeps Go admin auth simple and avoids direct DB access from Nuxt.

### Screens

Dashboard home:

- total requests today
- prompt/completion tokens today
- error rate
- endpoint health cards

Keys:

- list keys
- create key
- revoke key
- show raw key only immediately after creation

Usage:

- time-series by day/hour
- breakdown by key
- breakdown by endpoint
- recent requests table

Health:

- current LLM/OCR health
- recent health check history
- manual check button

### Tests

- Build Nuxt app.
- Verify login flow.
- Verify key creation flow.
- Verify dashboard calls server routes, not Go admin token from browser.

## Phase 5: Deployment

Goal: ship to jason cleanly.

### Files to Create

- `deploy/systemd/rtx-gateway.service`
- `deploy/nginx/rtx-llm.arthurlin.dev`
- `deploy/nginx/rtx-ocr.arthurlin.dev`
- `deploy/nginx/gateway.arthurlin.dev`
- `deploy/scripts/install.sh`
- `deploy/scripts/migrate.sh`

### systemd Service

The Go service should run as:

```text
User=rtx-gateway
Group=rtx-gateway
```

Suggested unit:

```ini
[Service]
EnvironmentFile=/etc/rtx-gateway/rtx-gateway.env
ExecStart=/opt/rtx-gateway/rtx-gateway
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/var/lib/rtx-gateway /var/log/rtx-gateway
```

### nginx Routing

Public API vhosts:

```nginx
location / {
  proxy_pass http://127.0.0.1:9188;
  proxy_set_header Host $host;
  proxy_set_header X-Real-IP $remote_addr;
  proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
  proxy_set_header X-Forwarded-Proto https;
  proxy_request_buffering off;
  client_max_body_size 64m;
  proxy_read_timeout 600s;
  proxy_send_timeout 600s;
}
```

Dashboard vhost:

```nginx
location /api/ {
  proxy_pass http://127.0.0.1:3008;
}

location / {
  proxy_pass http://127.0.0.1:3008;
}
```

Nuxt server routes can call the Go admin API on `http://127.0.0.1:9189`.

### Deployment Tests

- `systemctl status rtx-gateway`
- `curl http://127.0.0.1:9188/healthz`
- `curl http://127.0.0.1:9189/admin/v1/health`
- `curl https://rtx-llm.arthurlin.dev/v1/models` without auth -> 401
- `curl https://rtx-llm.arthurlin.dev/v1/models` with auth -> 200
- `curl https://rtx-ocr.arthurlin.dev/health` with auth policy decided -> 200 or 401 consistently
- verify usage row appears in SQLite
- verify streaming chat forwards chunks live

## Open Questions Before Coding

1. Should `/health` on public API hosts require an API key?
   - Current behavior allowed public health.
   - Recommendation: allow `GET /health` without API key, but log it separately and rate-limit in nginx if needed.

2. Should the gateway mutate streaming requests to add `stream_options.include_usage=true`?
   - Recommendation: not in Phase 2. Log missing streaming usage and revisit.

3. How long should usage logs be retained?
   - Recommendation: keep all rows initially; add retention after real volume is known.

4. Should failed auth attempts be persisted?
   - Recommendation: yes, but either in `usage_logs` with `api_key_id=NULL` or in a separate `auth_failures` table if the volume gets noisy.

5. Should endpoint permissions be host-based or path-based?
   - Recommendation: host-based for now: `llm` and `ocr`.

## First Implementation Milestone

The first useful milestone is:

- migrations apply
- one seed/admin-created API key exists
- authenticated `GET /v1/models` works for both hosts
- unauthenticated requests return 401
- one usage row is written per request

That milestone replaces nginx bearer auth without yet needing the dashboard.

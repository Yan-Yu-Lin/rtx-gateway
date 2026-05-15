# RTX Gateway

API gateway for the RTXWS public vLLM endpoints.

`rtx-gateway` sits behind nginx on `jason`, validates API keys, proxies OpenAI-compatible requests to RTXWS through the existing reverse SSH tunnel, and records usage metadata for auditing and dashboards.

## Architecture

```text
internet
  -> nginx + sslh on jason
  -> 127.0.0.1:9188 rtx-gateway public proxy
  -> 127.0.0.1:9180 RTXWS Gemma 4 vLLM tunnel
  -> 127.0.0.1:9183 RTXWS Chandra OCR vLLM tunnel

gateway.arthurlin.dev
  -> nginx
  -> 127.0.0.1:3008 Nuxt dashboard UI
  -> 127.0.0.1:9189 rtx-gateway admin API
```

The public proxy routes by `Host` header:

| Host | Upstream |
| --- | --- |
| `rtx-llm.arthurlin.dev` | `http://127.0.0.1:9180` |
| `rtx-ocr.arthurlin.dev` | `http://127.0.0.1:9183` |

Both upstreams are OpenAI-compatible vLLM APIs.

## Responsibilities

- Validate `Authorization: Bearer <api-key>` against hashed API keys in SQLite.
- Enforce per-key endpoint permissions.
- Forward requests to the correct RTXWS tunnel port.
- Preserve streaming behavior for SSE responses.
- Extract `usage.prompt_tokens`, `usage.completion_tokens`, and `usage.total_tokens` from OpenAI-compatible responses.
- Log request metadata, latency, status, model, token usage, client IP, and errors.
- Provide an admin API for key management, usage queries, and endpoint health.
- Serve data for a Nuxt dashboard.

## Runtime Environment

Verified target host: `jason`

- Ubuntu 24.04
- Go 1.22.2
- SQLite 3.45.1
- nginx on `127.0.0.1:8443` behind sslh on public `443`
- systemd for the Go service
- PM2 for the Nuxt dashboard
- service user: `rtx-gateway`
- binary: `/opt/rtx-gateway/rtx-gateway`
- database: `/var/lib/rtx-gateway/rtx-gateway.db`
- logs: journald and/or `/var/log/rtx-gateway`

## Development

```bash
go test ./...
go run ./cmd/rtx-gateway
```

Dashboard:

```bash
cd dashboard
npm ci
npm run dev
```

## Build

```bash
go build -o rtx-gateway ./cmd/rtx-gateway
```

For deployment on jason:

```bash
GOOS=linux GOARCH=amd64 go build -o dist/rtx-gateway ./cmd/rtx-gateway
```

## Configuration

Configuration should come from environment variables or an env file loaded by systemd.

Planned variables:

```env
RTX_GATEWAY_PUBLIC_ADDR=127.0.0.1:9188
RTX_GATEWAY_ADMIN_ADDR=127.0.0.1:9189
RTX_GATEWAY_DB_PATH=/var/lib/rtx-gateway/rtx-gateway.db
RTX_GATEWAY_KEY_PEPPER=replace-with-random-secret
RTX_GATEWAY_ADMIN_TOKEN=replace-with-admin-token
RTX_GATEWAY_LLM_HOST=rtx-llm.arthurlin.dev
RTX_GATEWAY_LLM_UPSTREAM=http://127.0.0.1:9180
RTX_GATEWAY_OCR_HOST=rtx-ocr.arthurlin.dev
RTX_GATEWAY_OCR_UPSTREAM=http://127.0.0.1:9183
RTX_GATEWAY_MAX_BODY_BYTES=67108864
```

Dashboard variables live in `/etc/rtx-gateway/dashboard.env`:

```env
NITRO_HOST=127.0.0.1
NITRO_PORT=3008
NUXT_ADMIN_API_URL=http://127.0.0.1:9189
NUXT_ADMIN_TOKEN=replace-with-same-value-as-RTX_GATEWAY_ADMIN_TOKEN
NUXT_SESSION_SECRET=replace-with-long-random-session-secret
NUXT_LOGIN_PASSWORD=replace-with-dashboard-login-passphrase
```

## API Key Format

API keys use this format:

```text
rtx_live_<8-char-prefix>_<32-char-secret>
```

Only the prefix and a hash of the full key are stored. The raw key is shown once at creation time.

## Deployment Sketch

Phase 5 deployment assets live under `deploy/`.

Important routing:

- `rtx-llm.arthurlin.dev` proxies to `127.0.0.1:9188`.
- `rtx-ocr.arthurlin.dev` proxies to `127.0.0.1:9188`.
- `gateway.arthurlin.dev` proxies to the Nuxt dashboard on `127.0.0.1:3008`.
- Nuxt server routes call the Go admin API on `127.0.0.1:9189` with `NUXT_ADMIN_TOKEN`.
- Browser traffic should not be proxied directly to `9189`; otherwise it bypasses Nuxt session auth.

Dry review the deploy files first:

```bash
ls -l deploy/scripts/
sed -n '1,220p' deploy/scripts/install.sh
```

Manual deployment command:

```bash
REMOTE_HOST=jason-ts ./deploy/scripts/install.sh
```

The script:

1. Cross-compiles the Go binary for linux/amd64.
2. Builds the Nuxt dashboard.
3. Copies artifacts to jason.
4. Installs `/opt/rtx-gateway/rtx-gateway`.
5. Creates `/etc/rtx-gateway/rtx-gateway.env` and `/etc/rtx-gateway/dashboard.env` from templates only if missing.
6. Installs the systemd unit and nginx vhosts.
7. Restarts `rtx-gateway`, reloads nginx, and starts/restarts the PM2 dashboard app.

Before first production restart, edit secrets:

```bash
ssh jason-ts 'sudoedit /etc/rtx-gateway/rtx-gateway.env'
ssh jason-ts 'sudoedit /etc/rtx-gateway/dashboard.env'
```

The Go service can also be managed directly:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now rtx-gateway
```

Manual migration helper:

```bash
REMOTE_HOST=jason-ts ./deploy/scripts/migrate.sh
```

`rtx-gateway` applies migrations at startup; the helper restarts the service and checks SQLite integrity.

Post-deploy checks:

```bash
ssh jason-ts 'sudo systemctl status rtx-gateway --no-pager'
ssh jason-ts 'pm2 status rtx-gateway-dashboard'
ssh jason-ts 'sudo nginx -t'
curl https://rtx-llm.arthurlin.dev/health
curl https://rtx-llm.arthurlin.dev/v1/models
```

See [PLAN.md](./PLAN.md) for the implementation plan.

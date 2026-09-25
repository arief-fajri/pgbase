# Single-Host Setup

<DocMeta audience="Operator" status="stable" verified="v0.5.4" />

The simplest production topology for small to medium workloads: one VPS, Docker Compose, and automatic HTTPS via Caddy.

## 1. Topology

```text
Client --(TLS :443)--> Caddy/nginx (terminates TLS)
  --(HTTP :8090, private network)--> pgbase
  --(TLS, sslmode=require|verify-full)--> Managed PostgreSQL 16+
```

The `docker-compose.prod.yml` wires exactly this on one host: Caddy auto-HTTPS, app + postgres on internal network, DB port never published.

## 2. Minimal bring-up

```bash
cp .env.example .env   # set DOMAIN + strong secrets (App. A)
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml exec pgbase pgbase superuser create admin@example.com "<STRONG-password>"
# open https://<DOMAIN>/_/ , sign in, enable rate limiting in settings
```

This gives you: auto HTTPS, only 80/443 published, restart policies + healthchecks, non-root image, `PB_HSTS=true` behind TLS, pgcrypto preinstalled. You still need to: set up periodic backups, configure `PB_ENCRYPTION_KEY`, enable rate limits, and rotate secrets.

## 3. Environment variables

See the full [Environment Variables](../reference/env.md) reference. The production-critical ones:

- `PB_POSTGRES_HOST/PORT/USER/PASSWORD/DBNAME/SSLMODE` — database connection
- `PB_ENCRYPTION_KEY` — mandatory 32-char key for encrypting settings secrets
- `PB_HSTS` — enable HSTS header when behind TLS
- `PB_METRICS_ADDR` / `PB_METRICS_EXPOSE` — Prometheus metrics endpoint
- `PB_BACKUP_MAX_EXTRACT_BYTES` — restore decompression cap

Build the binary: `go build -o pgbase ./examples/base`. The root package is a library — never `go run .`.

## 4. Dashboard hardening (after first login)

Enable in Settings: Rate Limits (off by default), `SuperuserIPs` whitelist, `TrustedProxy` headers. Store `PB_ENCRYPTION_KEY` externally — losing it makes encrypted settings undecryptable.

## 5. Connection pools (single-host sizing)

Effective per-instance ceiling = `DATA_MAX_OPEN + AUX_MAX_OPEN` (default 90, under stock `max_connections=100`). Rule: `(DATA+AUX) × instances ≤ max_connections − reserved`. Tune via `PB_POSTGRES_DATA/AUX_MAX_OPEN/IDLE_CONNS`, `CONN_MAX_LIFETIME/IDLE_TIME`, `CONNECT_TIMEOUT`, `STATEMENT/LOCK_TIMEOUT`, `DEFAULT_QUERY_EXEC_MODE`. See [Environment Variables](../reference/env.md) for details.

## 6. Multi-instance considerations

Rate limiter is in-memory per-instance, collections/settings cache invalidation is not yet verified under load, and JWT has no shared revocation. For multi-instance deployments, see the [Production Runbook](./production.md) and the [Realtime documentation](../flows/realtime.md).

## 7. Go-live checklist

- [ ] Dedicated non-superuser DB role; `sslmode=require`/`verify-full`
- [ ] `PB_ENCRYPTION_KEY` set (32 chars) + stored safely
- [ ] Strong superuser password; TLS terminated; `PB_HSTS=true`
- [ ] DB port unreachable from internet; process non-root
- [ ] Rate limiting on; `SuperuserIPs` set; `TrustedProxy` only behind proxy
- [ ] `/metrics` loopback-only (or explicitly protected)
- [ ] Secrets in a secret store (`.env` gitignored); tested backups with restricted bucket

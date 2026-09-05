# PRODUCTION.md — Running pgbase in Production

A short, self-contained runbook for deploying and operating **pgbase** (the
PostgreSQL fork of PocketBase) in production. It assumes you already built the
binary or image (see [Build](#1-build)) and have a PostgreSQL database to point
at.

> Security posture: this document reflects the hardening applied up to the
> current codebase — non-root container, opt-in settings encryption, opt-in
> HSTS, guarded `/metrics`, bounded backup/restore, and a checksum-verified

---

## 0. Quick single-host production (Docker Compose)

The simplest real production topology for small/medium workloads is **one VPS**,
`docker compose` with the bundled production file, and automatic HTTPS via
Caddy — effectively "docker compose up" plus the minimal configuration that
turns a demo into production.

```bash
# 1) on the VPS (Docker installed), clone/copy the repo
cp .env.example .env

# 2) edit .env: set DOMAIN + strong secrets (see Appendix A)
#    DOMAIN=api.example.com             <- your real domain pointing at the VPS
#    PB_POSTGRES_PASSWORD=<strong>
#    PB_ENCRYPTION_KEY=<32-char>

# 3) start everything (app + postgres + Caddy with automatic HTTPS)
docker compose -f docker-compose.prod.yml up -d

# 4) create the first superuser
docker compose -f docker-compose.prod.yml exec pgbase pgbase superuser create admin@example.com "<STRONG-password>"

# 5) open https://<DOMAIN>/_/ , sign in, and enable rate limiting in settings
```

What this gives you out of the box:

- **HTTPS + certificate automatically** (Caddy/Let's Encrypt) — no manual TLS.
- **Only ports 80/443 are published**; the app (`:8090`) and database are
  reachable **only via Caddy** on the internal compose network. The DB port is
  never exposed, even on loopback.
- **Restart policies & health checks** so services recover after crashes/boots.
- **The production hardening of the image** (non-root user, `--encryptionEnv`
  wiring for `PB_ENCRYPTION_KEY`) plus `PB_HSTS=true` behind TLS.
- pgcrypto preinstalled (avoids the first-boot extension deadlock).

Still yours to do (not automatable):

- **Periodic backups** — use the built-in backup API/dashboard (native
  `pg_dump`) on a cron and copy archives off the machine.
- **Set `PB_ENCRYPTION_KEY`** (mandatory; no default — without it settings
  secrets, incl. the token-signing secret, are plaintext at rest and travel
  unencrypted into backups) and **enable rate limiting** in the dashboard.
- Keep `.env` out of VCS and rotate secrets like a normal credential.

> This is a **single machine** — fine up to a few thousand users. For high availability / managed Postgres / S3 storage / zero-downtime deploys, see the topology in [#6](#6-recommended-production-topology) and monitoring in [#8](#8-monitoring-prometheus).

---

## 1. Build

```bash
# from the repository root
make build          # -> ./pgbase   (equivalent to: go build -o pgbase ./examples/base)

# or as a Docker image
docker build -t pgbase .
```

The runnable entrypoint lives in `examples/base`; the root Go package is a
library. Multi-platform release builds are configured via `.goreleaser.yaml`.

---

## 2. Environment variables

Flags take precedence over environment variables. Set these in production:

| Variable | Required | Description |
|----------|----------|-------------|
| `PB_POSTGRES_HOST` | yes | Postgres host. Prefer a managed/TLS-enabled instance. |
| `PB_POSTGRES_PORT` | no | Default `5432`. |
| `PB_POSTGRES_USER` | yes | DB user (least-privileged role; NOT `superuser`). |
| `PB_POSTGRES_PASSWORD` | yes | DB password (inject from a secret store, never a committed file). |
| `PB_POSTGRES_DBNAME` | yes | DB name (run app migrations on a dedicated database). |
| `PB_POSTGRES_SSLMODE` | **strongly recommended** | `require` for production, `verify-full` if your CA is available. **Do not run `disable` in production** (plaintext DB traffic). |
| `PB_ENCRYPTION_KEY` | **mandatory** | **32-character** key used to encrypt settings secrets at rest (SMTP/S3/OAuth2 + JWT token-signing secret, whose leak would allow forging auth tokens). Without it those values are stored base64-encoded **but unencrypted** (PGB-M02/CFG-01). The app reads it from the env var named by `--encryptionEnv`. |
| `PB_HSTS` | optional | `true` to emit `Strict-Transport-Security` (2y, includeSubDomains). Only meaningful when the app is served over HTTPS. |
| `PB_METRICS_ADDR` | optional | Bind address for the Prometheus `/metrics` listener (e.g. `127.0.0.1:9090`). Off when unset. |
| `PB_METRICS_EXPOSE` | optional | `true` only if you deliberately bind `/metrics` on a non-loopback address (e.g. inside a container behind a NetworkPolicy). Non-loopback binds **fail to start** without it. |
| `PB_BACKUP_MAX_EXTRACT_BYTES` | optional | Decompressed-size cap for restore, default `8 GiB` (zip-bomb guard, PGB-L03/N03). |

Example `.env` — see [Appendix A](#appendix-a-envproduction-example).

---

## 3. First run & migrations

Migrations run automatically on the first `serve` (schema bootstrap includes the
auth functional `LOWER()` unique indexes and the realtime outbox index).

Create the initial superuser — this works before or after `serve` (it connects
to the database directly):

```bash
./pgbase superuser create admin@example.com '<STRONG-password>'
```

Migrate manually if you prefer (equivalent to what `serve` does on start):

```bash
./pgbase migrate
```

---

## 4. Running the server

```bash
# 1) plain HTTP (behind your own TLS proxy) — recommended default:
./pgbase serve --http="0.0.0.0:8090"

# 2) built-in HTTPS via Let's Encrypt (autocert) — bind :80/:443 automatically:
./pgbase serve example.com www.example.com

# 3) loopback only (app and proxy on the same host):
./pgbase serve --http="127.0.0.1:8090"
```

CORS: `serve` accepts `--origins <list>` (default `["*"]`). This is safe with
Bearer-token auth and `AllowCredentials=false` (PGB-I05); set the real origin
list if the dashboard is served from a different origin.

When running behind a reverse proxy, also set **`TrustedProxy`** in the
dashboard settings so `RealIP`, IP rate limiting, and OAuth CSRF checks see the
client IP instead of the proxy's.

> **`TrustedProxy` only if ALL inbound traffic passes through the trusted
> proxy.** `RealIP()` trusts the configured `X-Forwarded-For` header verbatim —
> any host that can reach the app directly (or emit its own header) can spoof it.
> That directly weakens IP-based controls: rate limits and the `SuperuserIPs`
> whitelist can be bypassed with a forged header. Apply the same rule at the
> network edge: only the trusted proxy may reach the app port (bind loopback /
> internal network), and strip forged headers at the proxy itself.

---

## 5. Dashboard settings to enable for production

- **Rate limiting** — off by default (PGB-I03); enable in
  *Settings → Rate Limits* (login, auth, files, API). Login also has a built-in
  dummy-hash timing mitigation; OTP has a hardcoded 5/180s limiter.
- **Superuser IP whitelist** — *Settings → Admin* → `SuperuserIPs` to restrict
  which IPs may sign in as superuser.
- **Trusted proxy** — `TrustedProxy` headers (e.g. `X-Forwarded-For`) when
  behind a proxy. **Only set it when the app is guaranteed to be reachable
  exclusively through that trusted proxy** — otherwise the forwarded header can
  be spoofed to bypass rate limiting and the `SuperuserIPs` whitelist.
- Consider rotating and storing the `PB_ENCRYPTION_KEY` externally — losing it
  makes existing encrypted settings undecryptable.

---

## 6. Recommended production topology

```
      Client
        │  (TLS, :443)
        ▼
 Reverse proxy (Caddy/nginx — terminates TLS, sets PB_HSTS)
        │  (HTTP, :8090, private/proxy network)
        ▼
       pgbase
        │  (TLS, sslmode=require|verify-full)
        ▼
 Managed PostgreSQL (16+)
```

**Fast path:** the diagram above is exactly what
[`docker-compose.prod.yml`](#0-quick-single-host-production-docker-compose)
wires for you on one host — Caddy terminates TLS, `pgbase` stays on the internal
network, and (for a single-host stack) the postgres service is colocated instead
of managed.

- **Never** expose the database port publicly (the demo compose binds it to
  `127.0.0.1` only).
- **Never** run the process as root (the shipped Dockerfile already runs as an
  unprivileged user; `/pb_data` is chowned accordingly).
- Keep `pb_data`, backups, and the metrics listener off the public internet;
  bind `/metrics` to loopback (PGB-N01).

---

## 7. Backup & restore

- Backups use the **native `pg_dump`/`pg_restore`** format by default; create,
  download, upload, delete and restore are **superuser-only** (`/api/backups…`).
- Backup creation is bounded (default 10 min) and restore is bounded to 10 min;
  decompression is capped by `PB_BACKUP_MAX_EXTRACT_BYTES` (default 8 GiB).
- **Warning:** restore replays the archive via `pg_restore --clean`, i.e. it
  executes arbitrary SQL from the dump. Treat the backups bucket/filesystem as a
  full-DB-trust boundary (a tampered archive = DB takeover, PGB-N03). Guard it
  like your database backup credentials.

---

## 8. Monitoring (Prometheus)

pgbase ships an **opt-in** Prometheus endpoint on a **separate listener** from
the API. It is unauthenticated (PGB-N01), so traffic must stay on loopback (or a
protected internal interface).

**Enable**
```bash
PB_METRICS_ADDR=127.0.0.1:9090 ./pgbase serve --http=0.0.0.0:8090
curl http://127.0.0.1:9090/metrics    # Prometheus text format
```

**Metrics exposed** (custom prefix `pgbase_`; plus Go runtime + process collectors)

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `pgbase_http_request_duration_seconds` | histogram | `method`, `route`, `status` | `route` is the matched route **template** (low cardinality, e.g. `/api/collections/{id}`) |
| `pgbase_db_open_connections` | gauge | `db=data\|aux` | live `sql.DB` pool |
| `pgbase_db_in_use_connections` | gauge | `db` | connections in use |
| `pgbase_db_idle_connections` | gauge | `db` | idle connections |
| `pgbase_db_max_open_connections` | gauge | `db` | pool ceiling (0 = unlimited) |
| `pgbase_db_wait_count_total` | counter | `db` | connections waited for |
| `pgbase_db_wait_duration_seconds_total` | counter | `db` | time blocked on the pool |
| `pgbase_realtime_connected_clients` | gauge | — | connected SSE clients |
| `pgbase_realtime_dropped_messages` | gauge | — | dropped on full client buffers |

**Security note:** no authentication is applied to `/metrics`. Keep it:

- **loopback**: `127.0.0.1:9090` (host) — Prometheus on the same machine, or
- **internal-only**: bind a private interface; **or** publish the container
  port on `127.0.0.1` only (see compose note), **or** protect it with a
  reverse-proxy `basic_auth`, mTLS, or a firewall/NetworkPolicy. Non-loopback
  binds require `PB_METRICS_EXPOSE=true` (the app refuses otherwise).

### 8.1 Scraping — examples

**Same-host Prometheus** (binary/`prom/prometheus` Docker run on the host):

```yaml
# prometheus.yml
scrape_configs:
  - job_name: pgbase
    metrics_path: /metrics
    static_configs:
      - targets: ["127.0.0.1:9090"]
```

**With `docker-compose.prod.yml`** — enable on the app then publish loopback-only:

```bash
# .env
PB_METRICS_ADDR=0.0.0.0:9090        # inside the container = "non-loopback"
PB_METRICS_EXPOSE=true              # acknowledge the bind (guard)
# docker-compose.prod.yml: add to the pgbase service
#   ports:
#     - "127.0.0.1:9090:9090"
# -> host Prometheus scrapes http://127.0.0.1:9090/metrics
```

**Bundled stack (optional)** — include `deploy/docker-compose.monitoring.yml`
(Prometheus + Grafana + Alertmanager on an internal network scraping
`pgbase:9090`). Enable the listener on the app first:

```bash
# .env
GRAFANA_ADMIN_PASSWORD=<change-me>

# start everything (monitoring joins the prod compose network)
docker compose -f docker-compose.prod.yml -f deploy/docker-compose.monitoring.yml up -d
# Prometheus: http://localhost:9090  ·  Grafana: http://localhost:3000  ·  Alertmanager: http://localhost:9093
```

For a Prometheus running on the host (not a container), use
`deploy/prometheus.yml` (target `127.0.0.1:9090`).

**Cloud / external Prometheus:** bind the metrics listener on a private
interface (e.g. `10.0.0.5:9090`) with `PB_METRICS_EXPOSE=true`, and restrict the
security group / firewall to the Prometheus source only. Never publish it
publicly.

Alerting is your standard Prometheus stack — e.g. Alertmanager + webhooks. A
minimal, useful rule to start with: alert when
`rate(pgbase_db_wait_count_total[5m]) > 0` (pool saturation) or when
`pgbase_http_request_duration_seconds` p99 rises (see `deploy/prometheus.yml`).

See also `DEV.md §3d` for implementation details and the exact env semantics.

---

## 9. Self-update

```bash
./pgbase update
```
It **refuses non-HTTPS sources** and **verifies the asset sha256 against the
release `checksums.txt`** before replacing the binary (PGB-L09). Prefer
image-based deploys in containers.

---

## 10. Go-live checklist

- [ ] Dedicated non-`superuser` DB role; `PB_POSTGRES_SSLMODE=require`/`verify-full`
- [ ] `PB_ENCRYPTION_KEY` set (32 chars) + stored safely
- [ ] Superuser account created with a strong, unique password
- [ ] TLS terminated (proxy or built-in `--https`); `PB_HSTS=true`
- [ ] Database port not reachable from the internet
- [ ] Process runs as non-root (Docker image default)
- [ ] Rate limiting enabled in settings; `SuperuserIPs` set
- [ ] `TrustedProxy` configured when behind a proxy
- [ ] `/metrics` bound to loopback (or behind an explicit, protected exposure)
- [ ] Secrets in a secret store (never committed; `.env` is gitignored)
- [ ] Regular, tested backups (and the backups bucket access is restricted)

---

## Appendix A: `.env.production` example

```bash
# (required for docker-compose.prod.yml) public domain -> VPS
DOMAIN=api.example.com

# Database (managed/TLS Postgres) — for the prod-compose single-host stack
# point HOST at the internal "postgres" service instead:
#   PB_POSTGRES_HOST=postgres ; PB_POSTGRES_SSLMODE=disable
PB_POSTGRES_HOST=db.internal.example.com
PB_POSTGRES_PORT=5432
PB_POSTGRES_USER=pgbase_app
PB_POSTGRES_PASSWORD=<inject-from-secret-store>
PB_POSTGRES_DBNAME=pgbase
PB_POSTGRES_SSLMODE=verify-full

# Settings encryption (32 chars) — set via --encryptionEnv (or default in the
# shipped Dockerfile's CMD)
PB_ENCRYPTION_KEY=<32-character-random>

# Security headers / observability
PB_HSTS=true
PB_METRICS_ADDR=127.0.0.1:9090        # optional, loopback-only

# Backup restore decompression cap (default 8 GiB)
# PB_BACKUP_MAX_EXTRACT_BYTES=8589934592
```

## Appendix B: reverse-proxy snippets

**Caddy**

```caddyfile
example.com {
    encode gzip
    tls your-email@example.com
    reverse_proxy 127.0.0.1:8090
    header Strict-Transport-Security "max-age=63072000; includeSubDomains"
}
```

**nginx**

```nginx
server {
    listen 443 ssl http2;
    server_name example.com;
    ssl_certificate     /etc/letsencrypt/live/example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/example.com/privkey.pem;
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains" always;

    location / {
        proxy_pass http://127.0.0.1:8090;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

> If the proxy sets these headers, configure `TrustedProxy` (dashboard settings)
> so the app trusts `X-Forwarded-For` from the proxy only.

## Appendix C: systemd unit (`/etc/systemd/system/pgbase.service`)

```ini
[Unit]
Description=pgbase (PocketBase/Postgres fork)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=pgbase
Group=pgbase
WorkingDirectory=/var/lib/pgbase
EnvironmentFile=/etc/pgbase/.env
ExecStart=/usr/local/bin/pgbase serve --http="127.0.0.1:8090" --encryptionEnv=PB_ENCRYPTION_KEY
Restart=on-failure
RestartSec=3
# hardening (optional but recommended)
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/pgbase/pb_data
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

---

*See also `DEV.md` (development) and `README.md` (quick start)*
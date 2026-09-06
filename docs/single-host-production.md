# Single-Host Production

> Audience: Operator · Status: stable · Last verified: v0.5.2 (`923e860`)

Thin wrapper over `../PRODUCTION.md` (the runbook). This page states the topology
and checklist; flag/env details live in `../PRODUCTION.md` and `reference/env.md`.

## 1. Topology

```text
Client --(TLS :443)--> Caddy/nginx (terminates TLS)
  --(HTTP :8090, private network)--> pgbase
  --(TLS, sslmode=require|verify-full)--> Managed PostgreSQL 16+
```

`../PRODUCTION.md:167-180`. Fast path: `docker-compose.prod.yml` wires exactly
this on one host (Caddy auto-HTTPS, app + postgres on internal network, DB port
never published).

## 2. Minimal bring-up

```bash
cp .env.example .env   # set DOMAIN + strong secrets (App. A)
docker compose -f docker-compose.prod.yml up -d
docker compose -f docker-compose.prod.yml exec pgbase pgbase superuser create admin@example.com "<STRONG-password>"
# open https://<DOMAIN>/_/ , sign in, enable rate limiting in settings
```

Gives: auto HTTPS, only 80/443 published, restart policies + healthchecks,
non-root image, `PB_HSTS=true` behind TLS, pgcrypto preinstalled.
Still yours: periodic native backups off-machine, `PB_ENCRYPTION_KEY`,
rate limits, secret rotation (`../PRODUCTION.md` §0).

## 3. Secrets and env (pointers)

- Canonical table: `reference/env.md`. Production-critical:
  `PB_POSTGRES_HOST/PORT/USER/PASSWORD/DBNAME/SSLMODE`,
  `PB_ENCRYPTION_KEY` (**mandatory 32-char**, `--encryptionEnv`; without it SMTP/S3/OAuth2
  and token-signing secret are base64 but **unencrypted** and copied into `pg_dump`
  backups), `PB_HSTS`, `PB_METRICS_ADDR`/`PB_METRICS_EXPOSE`, `PB_BACKUP_MAX_EXTRACT_BYTES`.
- Build: `make build` (`go build -o pgbase ./examples/base`); image `docker build -t pgbase .`
  (`../PRODUCTION.md` §1). Root package is a library — never `go run .`.
- Serve: plain HTTP behind proxy (`serve --http="0.0.0.0:8090"`), built-in autocert
  (`serve example.com`), or loopback (`--http="127.0.0.1:8090"`). CORS `--origins`
  defaults `["*"]` (safe with Bearer auth, `AllowCredentials=false`).
- `TrustedProxy`: set **only** when all inbound passes through the proxy;
  otherwise `X-Forwarded-For` spoofing bypasses rate limits + `SuperuserIPs`.

## 4. Dashboard hardening (after first login)

Enable in Settings: Rate Limits (off by default; login has dummy-hash timing +
OTP has hardcoded 5/180s), `SuperuserIPs` whitelist, `TrustedProxy` headers.
Rotate `PB_ENCRYPTION_KEY` externally — losing it makes encrypted settings
undecryptable (`../PRODUCTION.md` §5).

## 5. Connection pools (single-host sizing)

Effective per-instance ceiling = `DATA_MAX_OPEN + AUX_MAX_OPEN` (default 90,
under stock `max_connections=100`). Rule:
`(DATA+AUX) × instances ≤ max_connections − reserved`.
Tune via `PB_POSTGRES_DATA/AUX_MAX_OPEN/IDLE_CONNS`, `CONN_MAX_LIFETIME/IDLE_TIME`,
`CONNECT_TIMEOUT`, `STATEMENT/LOCK_TIMEOUT`, `DEFAULT_QUERY_EXEC_MODE`
(`../DEV.md` §3, `reference/env.md`). Front large fan-out with PgBouncer;
a tested transaction-mode recipe is `roadmap-open` (Theme G).

## 6. Multi-instance is NOT GA

Rate limiter is in-memory per-instance (N× limit), collections/settings cache
invalidation unverified under load, cron single-fire unverified at N, JWT has no
shared revocation. Realtime cross-instance needs the experimental outbox
(`flows/realtime.md`). Stay single-host until `roadmap.md` Theme A is proven.

## 7. Go-live checklist (from `../PRODUCTION.md` §10)

- [ ] Dedicated non-superuser DB role; `sslmode=require`/`verify-full`
- [ ] `PB_ENCRYPTION_KEY` set (32 chars) + stored safely
- [ ] Strong superuser password; TLS terminated; `PB_HSTS=true`
- [ ] DB port unreachable from internet; process non-root
- [ ] Rate limiting on; `SuperuserIPs` set; `TrustedProxy` only behind proxy
- [ ] `/metrics` loopback-only (or explicitly protected)
- [ ] Secrets in a secret store (`.env` gitignored); tested backups with restricted bucket

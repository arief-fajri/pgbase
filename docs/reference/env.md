# Reference: Environment Variables (canonical)

> Audience: All · Status: stable · Last verified: v0.5.2 (`923e860`)

**Single source of truth.** `README.md`, `DEV.md`, `PRODUCTION.md` tables are
frozen pointers here. Code truth: `core/db_connect.go:37-139`,
`core/base.go:43-46,245-259`, `apis/metrics.go:34-59`, `apis/serve.go:84`,
`tools/archive/extract.go:16-39`, `core/realtime_outbox.go:47-51`.

Flags take precedence over env where both exist. `--pg-*` serve flags are
**not wired** to the connection — set `PB_POSTGRES_*` env vars (`../DEV.md` §3).

## PostgreSQL connection

| Variable | Default | Description |
|---|---|---|
| `PB_POSTGRES_HOST` | `localhost` | Postgres host; managed/TLS instance in prod |
| `PB_POSTGRES_PORT` | `5432` | Host port (tests use `5433`) |
| `PB_POSTGRES_USER` | `pgbase` | Least-privileged role, **not** superuser in prod |
| `PB_POSTGRES_PASSWORD` | *(empty, must set)* | Inject from secret store; compose fails fast if unset |
| `PB_POSTGRES_DBNAME` | `pgbase` | Dedicated database |
| `PB_POSTGRES_SSLMODE` | `prefer` | `require`/`verify-full` in prod; warns on non-loopback `disable` |

## Pool sizing and timeouts

Effective ceiling per instance = `DATA_MAX_OPEN + AUX_MAX_OPEN` (default **90**).
Rule: `(DATA+AUX) × instances ≤ max_connections − reserved`.

| Variable | Default | Pool / meaning |
|---|---|---|
| `PB_POSTGRES_DATA_MAX_OPEN_CONNS` | `80` | data — max open |
| `PB_POSTGRES_DATA_MAX_IDLE_CONNS` | `15` | data — max idle |
| `PB_POSTGRES_AUX_MAX_OPEN_CONNS` | `10` | aux (`_logs`, LISTEN-adjacent) — max open |
| `PB_POSTGRES_AUX_MAX_IDLE_CONNS` | `3` | aux — max idle |
| `PB_POSTGRES_CONN_MAX_LIFETIME` | `30m` | recycle age |
| `PB_POSTGRES_CONN_MAX_IDLE_TIME` | `3m` | idle close |
| `PB_POSTGRES_CONNECT_TIMEOUT` | `10` (seconds) | fail-fast dial on pool refill |
| `PB_POSTGRES_STATEMENT_TIMEOUT` | `60s` | role-level `statement_timeout` on boot (`off`/`0` resets) |
| `PB_POSTGRES_LOCK_TIMEOUT` | `30s` | role-level `lock_timeout` on boot (`off`/`0` resets) |
| `PB_POSTGRES_DEFAULT_QUERY_EXEC_MODE` | *(unset)* | `exec`/`simple_protocol` for PgBouncer transaction pooling |
| `PB_DB_VACUUM_CRON` | *(unset, off)* | daily whole-DB `VACUUM` schedule (e.g. `0 0 * * *`); autovacuum covers it |

App also enforces `DefaultQueryTimeout ~30s` on reads (`queryTimeoutHook`) and
writes (`withWriteDeadline`); caller context (HTTP disconnect) wins.

## Realtime, metrics, backups, auth, misc

| Variable | Default | Description |
|---|---|---|
| `PB_REALTIME_OUTBOX` | *(off)* | `1` enables DB-backed cross-instance realtime (experimental) |
| `PB_METRICS_ADDR` | *(unset = disabled)* | e.g. `127.0.0.1:9090`; enables `/metrics` on a separate listener |
| `PB_METRICS_EXPOSE` | *(unset)* | `true`/`1` acknowledges a **non-loopback** `/metrics` bind (fails to start otherwise) |
| `PB_BACKUP_MAX_EXTRACT_BYTES` | `8 GiB` | decompression cap on restore (zip-bomb guard) |
| `PB_PG_DUMP_BIN` / `PB_PG_RESTORE_BIN` | `pg_dump` / `pg_restore` on `PATH` | override client binary paths (must be regular + executable) |
| `PB_IMPERSONATE_MAX_TOKEN_DURATION` | `2592000` (30d, seconds) | upper bound for superuser impersonate tokens |
| `PB_ENCRYPTION_KEY` | *(unset = unencrypted!)* | **Mandatory 32-char** in prod; read via `--encryptionEnv`; encrypts SMTP/S3/OAuth2 + token-signing secret at rest |
| `PB_HSTS` | *(unset)* | `true` emits `Strict-Transport-Security` (2y, includeSubDomains) — only behind HTTPS |

## Test harness (separate prefix)

`tests/app.go` reads `PGTEST_HOST/PORT/USER/PASSWORD/DBNAME`
(defaults `localhost:5433/test/test/pgbase_test`). Raw core tests
(`base_test`, `log_printer_test`, …) use the production `PB_POSTGRES_*` path —
point both at the test DB in CI (`../DEV.md` §10).

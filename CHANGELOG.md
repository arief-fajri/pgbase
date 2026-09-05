## v0.5.2

Comprehensive security hardening — SSRF protection, download size caps, supply chain integrity, and safer defaults across the board.

### Security

- **Shared SSRF guard**: extracted `SafeHTTPClient` into `tools/security/httputil.go` as a reusable, exported package; `NewFileFromURL()` now uses it automatically. Private/loopback/multicast addresses are rejected at dial time.
- **Download size caps**: backup restore, `ghupdate` releases, and `filesystem.NewFileFromURL` all enforce byte limits (configurable per call site) to prevent self-DoS from oversized responses.
- **HTTPS-only asset downloads**: `ghupdate` validates that release asset URLs use `https://` before downloading — a tampered URL can no longer point at a plaintext host.
- **Impersonate token cap**: `PB_IMPERSONATE_MAX_TOKEN_DURATION` env var (default 30 days) limits the upper bound a superuser can request for an impersonate token, preventing leaked tokens from granting static access indefinitely.
- **SQL identifier quoting**: replaced all hardcoded `"name"` quoting in view/index DDL with `dbutils.DefaultDialect.QuoteIdentifier()` — embedded double-quotes are now escaped correctly.
- **`X-Content-Type-Options: nosniff`** added to file-serving responses.
- **pg_dump/pg_restore binary validation**: `lookupPGBinary` now verifies the override path references a regular, executable file instead of silently accepting a missing or non-executable path.

### Configuration

- **Default `pg-sslmode` changed from `disable` to `prefer`** — new installations will use TLS when the server supports it. Existing `.env` files with `PB_POSTGRES_SSLMODE=disable` are unaffected.
- **Startup warning hardened**: the missing-encryption-key banner now explicitly mentions the token-signing secret and states that `PB_ENCRYPTION_KEY` is **mandatory** for production (not merely "recommended").
- **Non-loopback `sslmode=disable` warning (CFG-02)**: the server now logs a warning when connecting to a non-loopback PostgreSQL host with `sslmode=disable`, flagging plaintext database traffic in production.

### Supply chain

- **GitHub Actions pinned by full SHA** (CFG-07): `actions/checkout`, `actions/setup-node`, `actions/setup-go`, `goreleaser/goreleaser-action` — prevents tag-swapping attacks.
- **Docker images pinned by digest** (CFG-06): `golang`, `alpine`, `prometheus`, `grafana`, `alertmanager` — bump deliberately by re-pinning.

### Documentation

- `PRODUCTION.md`: expanded `PB_ENCRYPTION_KEY` and `TrustedProxy` guidance with spoofing risk warnings.
- `DEV.md`: encryption key docs updated to reflect mandatory status.
- `.env.example`: `sslmode` and encryption key comments rewritten for clarity.

### Internal

- `NewUnsafeFileFromURL()` added to `tools/filesystem` for internal/trusted-URL use only (no SSRF guard, no size cap).
- `ValidateDialAddress()` exported from `tools/security` for dial-time IP validation.
- `jsvm` binds test updated to expect SSRF guard rejection on loopback `fileFromURL` calls.

## v0.5.1

Pure rebranding release — no behavioral changes.

- Renamed the root package files to `pgbase.go` / `pgbase_test.go`.
- Replaced all self-referencing "PocketBase" wording with **PGBase** across the core, JSVM `$app` type, dashboard UI, and legacy-backup docs/tests.
- Pointed `ghupdate` defaults to the fork repository (`arief-fajri/pgbase`).
- Rewrote `CONTRIBUTING.md` for the fork.
- Regenerated `types.d.ts` and rebuilt `ui/dist`.
- All references to the upstream PocketBase project are preserved.

## v0.5.0

Ships the first performance and security remediation batch for PostgreSQL, adds opt-in Prometheus `/metrics` and a DB-backed cross-instance realtime outbox, and hardens the docker-compose / release pipeline.

### Performance

- **Connection pool tuning**: data/aux pool sizes, `SetConnMaxLifetime` and `SetConnMaxIdleTime` are now env-tunable (`PB_POSTGRES_DATA_MAX_OPEN_CONNS`, `PB_POSTGRES_DATA_MAX_IDLE_CONNS`, `PB_POSTGRES_AUX_MAX_OPEN_CONNS`, `PB_POSTGRES_AUX_MAX_IDLE_CONNS`, `PB_POSTGRES_CONN_MAX_LIFETIME`, `PB_POSTGRES_CONN_MAX_IDLE_TIME`); defaults lowered (data 80 / aux 10) to stay under `max_connections`.

- **Functional identity indexes**: case-insensitive unique `LOWER(...)` indexes for email and all password-auth identity fields, closing the case-variant duplicate-account loophole and serving login lookups by index scan (conversion migrations + the PostgreSQL 23505 error is normalized back to `validation_not_unique`).

- **Batch log writes**: the log writer flushes chunked multi-row INSERTs (one round-trip instead of up to `BatchSize`) and skips the aux transaction wrapper for single-chunk flushes.

- **Bulk MFA/OTP cleanup**: expired sessions are now removed with one bulk DELETE per auth collection instead of fetch-then-per-row deletes.

- **gzip on API responses**: `/api` JSON is compressed on the wire (MinLength 1024); realtime SSE and file/backup downloads opt out.

- **Expand batching**: indirect/back-relation expand resolves all parents in one query (N+1 removed) while preserving the per-record relation cap.

- **Realtime access memoization**: record broadcast access-rule checks are cached per broadcast, and client queues are bounded (32) with non-blocking sends; slow-consumer drops are recorded per client via `DroppedCount()`.

- **Index-diff sync**: collection index sync is now three-tier — cosmetic field changes no longer rebuild indexes, index-only edits rebuild only the changed ones, structural changes keep the rebuild-all fallback; analyze is scoped to the changed table only.

- **Bounded writes**: model writes carry a client-side deadline (mirroring read timeouts) so dead connections self-heal without exhausting the pool; `connect_timeout` added to the DSN (`PB_POSTGRES_CONNECT_TIMEOUT`, default 10s).

- **Cron advisory locks**: each DB-touching cron (MFA/OTP, audit, logs, vacuum, auto-backup, heartbeat cleanup) runs under a non-blocking `pg_try_advisory_lock` so one instance executes it; the VACUUM cron is off by default.

- Statement/timeout configuration via `ALTER ROLE CURRENT_USER` at boot, a PgBouncer-compatible `default_query_exec_mode` toggle, and indexed cached collections for O(1) lookup.

### Cross-instance realtime (opt-in)

- **DB-backed outbox**: `core.PublishRealtimeEvent` writes outbox rows (create/update store record ids; delete stores the full pre-delete snapshot) and wakes listeners via `NOTIFY pb_realtime_outbox`. Enabled with `PB_REALTIME_OUTBOX`; default off means zero single-instance overhead.

- **Outbox listener**: consumes rows on every instance for local re-fanout, with reconnect/backoff + catch-up, a forward `(created,id)` cursor (fixes the stuck-window and unbounded memory growth), and per-instance origin stamping so instances skip their own events.

### Metrics

- **Opt-in Prometheus `/metrics`** on a dedicated listener (`PB_METRICS_ADDR`; loopback enforced unless `PB_METRICS_EXPOSE`): request duration histograms by route template, sql.DB pool stats (data + aux), realtime clients + dropped-message totals, Go runtime/process collectors.

### Security & hardening

- Dependency bumps close all `govulncheck` findings (pgx v5.9.2, x/image, toolchain `go1.25.14`) — `govulncheck ./...` and `npm audit` report 0 vulnerabilities / clean.

- Compose no longer ships a default DB password (fails fast without `.env`), binds Postgres to `127.0.0.1`, and the release image runs non-root with `--encryptionEnv` wiring.

- Server-side HTML allow-list sanitization for editor fields (on write and SQLite import) and an href scheme allow-list for the URL field; a startup warning when `--encryptionEnv` is missing.

- Opt-in HSTS (`PB_HSTS`), `Referrer-Policy` and tightened CSP; `ghupdate` restricted to HTTPS + `checksums.txt` verification; sensitive query params redacted from request logs.

- Backups bounded: `pg_dump` now runs outside the transaction (no pinned `xmin` horizon) and archive extraction is capped by `PB_BACKUP_MAX_EXTRACT_BYTES`.

### Deployment

- New `PRODUCTION.md` runbook, `docker-compose.prod.yml` (Caddy auto-HTTPS) and a monitoring stack under `deploy/` (Prometheus + Grafana + Alertmanager); README linked; `/pb_migrations` and `.env` gitignored.

### Migrations

- `email_functional_index`, `identity_functional_index`, `instance_heartbeats_init`, `logs_autovacuum_tuning`, `realtime_outbox_init`, `realtime_outbox_origin`, `realtime_outbox_indexes`.

## v0.4.0

Makes **native PostgreSQL `pg_dump` / `pg_restore` the default backup format** — a faithful, full-database snapshot — and demotes the portable SQLite dump to an opt-in legacy format. Adds offline `backup` / `restore` CLI commands.

### Backups

- **Native PostgreSQL dump by default**: `CreateBackup` now bundles a custom-format `pg_dump` archive (`pgdata.dump`) covering the *entire* database — schema, all record tables, `_collections`, `_params`, `_migrations`, `_superusers`, request `_logs` and the partitioned `_audits` / `_audit_reads` — as an exact-replica disaster-recovery snapshot (no `--exclude-table`).

- **Native restore**: `RestoreBackup` auto-detects a bundled `pgdata.dump` and restores it into the live database with `pg_restore --clean --if-exists` before swapping in the backup's storage files. Success is gated by a post-restore sanity check (non-empty `_collections`) rather than the noisy `pg_restore` exit code.

- **Opt-in portable SQLite dump**: the previous v0.23-format SQLite `data.db` export is still available for cross-engine portability and older tooling. Select it via the `Backups.Format` setting (`"pg"` default, `"sqlite"` legacy) or, per run, the `--format` CLI flag. Restore auto-detects either bundled format (`pgdata.dump` → native, `data.db` → legacy).

- **CLI `backup` / `restore` commands**: `backup [name] [--format pg|sqlite]` creates a backup, and `restore <name>` performs an **offline** restore that does *not* restart the process (safe to run while the server is stopped); (re)start the app afterwards to load the restored data.

### Dashboard

- The *Backup options* settings form now has a **Backup format** selector (Native PostgreSQL / Portable SQLite) bound to `Backups.Format`, so the default format for dashboard- and cron-generated backups can be chosen from the UI.

### Requirements

- Native backups shell out to the `pg_dump` / `pg_restore` client binaries. They must be available on `PATH` at runtime, and the client major version must be **>= the PostgreSQL server major** (`pg_dump` refuses to dump a newer server). The release image already ships the v16 client.

- Override the binary locations with the `PB_PG_DUMP_BIN` / `PB_PG_RESTORE_BIN` environment variables. If the client is unavailable, switch `Backups.Format` (or `--format`) to `sqlite`.

## v0.3.0

Turns native backups into **engine-portable full snapshots** and adds **legacy SQLite backup import**, so archives from upstream PocketBase (or an older SQLite-based deployment) can be restored directly into PostgreSQL.

### Backups

- **Portable database dump**: `CreateBackup` now dumps the live PostgreSQL database into a portable v0.23-format SQLite `data.db` bundled in the archive alongside the `pb_data` storage. A native backup is now a full schema + records + storage snapshot instead of storage only.

- **Cross-engine restore**: `RestoreBackup` auto-detects a bundled `data.db` and imports the schema, records and settings into PostgreSQL before swapping in the backup's storage files — no separate migration step (request logs are not imported).

- **Legacy import with version auto-detection**: restore archives produced by `v0.23+` and pre-`v0.23` (v0.22) PocketBase. Legacy field options are converted (option hoisting + renames), `created`/`updated` autodate fields are injected, and `_admins` are migrated to `_superusers`. Base collections are fully supported; pre-`v0.23` auth and view collections are best-effort.

### Dashboard

- The *Restore backup* modal now explains that a legacy SQLite `data.db` is auto-detected and imported into PostgreSQL, and that `pb_data` storage files are swapped from the backup.

- Hid the color-scheme (light/dark) toggle from the app header.

## v0.2.0

Adds a built-in **audit trails** subsystem for tracking data changes and read access, and rebrands the dashboard to **PG-Base**.

### Audit trails

- **Data changes trail** (`_audits`): records `create`/`update`/`delete` operations for an allowlist of collections, including per-field diffs (`changes`) and a full record `snapshot`.

- **Read access trail** (`_audit_reads`): records `view`/`list` access as metadata only (filter/sort/page/total — never record content). Written best-effort and non-blocking via a batched writer.

- **Audit settings**: enable each trail independently, choose the audited collections (allowlist by name, shared by both trails), set per-trail retention in days (`0` = keep forever), and optionally store the actor IP and User-Agent.

- **PostgreSQL month partitioning**: both audit tables use `RANGE` partitioning by month with a composite `(id, created)` primary key and a mandatory `DEFAULT` partition; a retention cron prunes expired data.

- **Redaction**: hidden and password fields are stripped from diffs and snapshots before they are persisted.

- **Superuser API**: `GET /api/audits`, `GET /api/audits/{id}` and `GET /api/audits/reads` for querying the trails.

- **Dashboard UI**: a new *Audits* page (with *Data changes* / *Read access* tabs, search/filter and detail preview) and an *Audit logs* settings page.

### Dashboard

- **Rebranded to PG-Base**: the footer now shows the fork version (`PG-Base v{version}`) and links to the fork repository, and the *Docs* link points to the project README.

- Reworked the *Audit logs* settings page into stacked per-trail cards with a searchable collections allowlist and a clearer actions layout.

### Fixes

- Request logs are now ordered newest-first by `created`. The previous ordering relied on `@rowid`, which maps to a random-string id in the PostgreSQL fork and produced a scrambled order.

## v0.1.0

Initial release of **pgbase** — a hard fork of [PocketBase](https://github.com/pocketbase/pocketbase) that replaces the embedded SQLite database with **PostgreSQL** as the sole supported backend.

- Replaced SQLite with PostgreSQL throughout the core (via `pgx` v5 + `dbx`).

- Ported all dialect-specific logic to PostgreSQL: `JSONB` columns and path access, `timestamptz` date handling, `pgcrypto` (`gen_random_bytes`) record-id defaults, `information_schema`/`pg_catalog` introspection, and view/field type inference.

- Moved the auxiliary logs storage to PostgreSQL as well.

- Schema-based isolation for the test harness (one PostgreSQL schema per test app).

- Removed SQLite-only code paths, drivers and build tags.

> pgbase is derived from PocketBase (https://github.com/pocketbase/pocketbase); see `LICENSE.md` for attribution. The upstream PocketBase changelog history has been intentionally reset for this fork — refer to the PocketBase repository for the pre-fork history.

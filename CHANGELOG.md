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

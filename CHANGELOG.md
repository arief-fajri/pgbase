## v0.2.0

Adds a built-in **audit trails** subsystem for tracking data changes and read access.

- **Data changes trail** (`_audits`): records `create`/`update`/`delete` operations for an allowlist of collections, including per-field diffs (`changes`) and a full record `snapshot`.

- **Read access trail** (`_audit_reads`): records `view`/`list` access as metadata only (filter/sort/page/total — never record content). Written best-effort and non-blocking via a batched writer.

- **Audit settings**: enable each trail independently, choose the audited collections (allowlist by name, shared by both trails), set per-trail retention in days (`0` = keep forever), and optionally store the actor IP and User-Agent.

- **PostgreSQL month partitioning**: both audit tables use `RANGE` partitioning by month with a composite `(id, created)` primary key and a mandatory `DEFAULT` partition; a retention cron prunes expired data.

- **Redaction**: hidden and password fields are stripped from diffs and snapshots before they are persisted.

- **Superuser API**: `GET /api/audits`, `GET /api/audits/{id}` and `GET /api/audits/reads` for querying the trails.

- **Dashboard UI**: a new *Audits* page (with *Data changes* / *Read access* tabs, search/filter and detail preview) and an *Audit logs* settings page.

## v0.1.0

Initial release of **pgbase** — a hard fork of [PocketBase](https://github.com/pocketbase/pocketbase) that replaces the embedded SQLite database with **PostgreSQL** as the sole supported backend.

- Replaced SQLite with PostgreSQL throughout the core (via `pgx` v5 + `dbx`).

- Ported all dialect-specific logic to PostgreSQL: `JSONB` columns and path access, `timestamptz` date handling, `pgcrypto` (`gen_random_bytes`) record-id defaults, `information_schema`/`pg_catalog` introspection, and view/field type inference.

- Moved the auxiliary logs storage to PostgreSQL as well.

- Schema-based isolation for the test harness (one PostgreSQL schema per test app).

- Removed SQLite-only code paths, drivers and build tags.

> pgbase is derived from PocketBase (https://github.com/pocketbase/pocketbase); see `LICENSE.md` for attribution. The upstream PocketBase changelog history has been intentionally reset for this fork — refer to the PocketBase repository for the pre-fork history.

## v0.1.0

Initial release of **pgbase** — a hard fork of [PocketBase](https://github.com/pocketbase/pocketbase) that replaces the embedded SQLite database with **PostgreSQL** as the sole supported backend.

- Replaced SQLite with PostgreSQL throughout the core (via `pgx` v5 + `dbx`).

- Ported all dialect-specific logic to PostgreSQL: `JSONB` columns and path access, `timestamptz` date handling, `pgcrypto` (`gen_random_bytes`) record-id defaults, `information_schema`/`pg_catalog` introspection, and view/field type inference.

- Moved the auxiliary logs storage to PostgreSQL as well.

- Schema-based isolation for the test harness (one PostgreSQL schema per test app).

- Removed SQLite-only code paths, drivers and build tags.

> pgbase is derived from PocketBase (https://github.com/pocketbase/pocketbase); see `LICENSE.md` for attribution. The upstream PocketBase changelog history has been intentionally reset for this fork — refer to the PocketBase repository for the pre-fork history.

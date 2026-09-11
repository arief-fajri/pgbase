# AGENTS.md — instructions for AI coding agents working on this repository

PG-BASE is a hard fork of PocketBase with PostgreSQL as the only storage
engine (pgx v5 + forked `dbx`). The public REST API stays PocketBase-compatible
— compatibility is a product feature, divergence must be deliberate, tested,
and documented in `docs/fork-deltas.md`.

Read first, depending on the task:
- Architecture: `docs/architecture/end-to-end.md`, `docs/architecture/backend-layers.md`
- Behavior deltas vs upstream: `docs/fork-deltas.md`
- Fork/upstream policy: `FORK_STRATEGY.md`
- Full dev guide: `docs/developing.md` (single source of truth for commands)

## Build

```bash
go build -o pgbase ./examples/base   # the runnable entrypoint is examples/base
```

The dashboard UI is prebuilt and committed at `ui/dist` (embedded via
`ui/embed.go`). Only rebuild it when changing `ui/src`:

```bash
npm --prefix ui ci && npm --prefix ui run build   # BEFORE go build
```

## Test

Tests need a PostgreSQL 16+ instance. Standard setup:

```bash
docker compose -f tests/docker-compose.test.yml up -d --wait
PB_POSTGRES_HOST=localhost PB_POSTGRES_PORT=5433 PB_POSTGRES_USER=test \
PB_POSTGRES_PASSWORD=test PB_POSTGRES_DBNAME=pgbase_test \
  go test ./... -count=1 -p 4 -timeout=1200s        # same as `make test`
```

- Tests are DB-backed: `tests.NewTestApp()` creates a database per test via
  `CREATE DATABASE ... TEMPLATE`. `-p 4` is safe; do not use `-p 1`.
- The same suite must pass under `-race` (CI runs a dedicated race job).
- Raw `core.NewBaseApp` tests use the `PB_POSTGRES_*` env above, not `PGTEST_*`.
- `pg_dump`/`pg_restore` must be on PATH (v16+) or the backup round-trip test skips.

## Lint

```bash
golangci-lint run -c ./golangci.yml ./...   # must be zero issues; CI blocks on it
```

Formatting: `gofmt`/`goimports` defaults (alphabetical import order within
groups — the v0.5.1 rebrand once broke this; keep the tree clean).

## Docs

- Build the docs site before opening docs PRs: `npm --prefix docs ci && npm --prefix docs run docs:build`.
- Internal doc links must keep the `.md` suffix (e.g. `[roadmap](./roadmap.md)`).
  VitePress strips it for routing, but the lychee CI check resolves links
  against the filesystem — extensionless links fail the `docs` workflow.
- `make docs-check` (markdownlint) must stay clean; new pages need a
  `<DocMeta>` frontmatter line and a sidebar entry in `docs/.vitepress/config.mts`.

## Code layout

- `core/` — domain: app bootstrap, collections, records, fields, auth, audit, backup
- `apis/` — REST routes and middlewares
- `forms/` — request-validation layer used by apis
- `tools/` — self-contained utilities (search/filter+sort translation is fork-diverged: `tools/search`)
- `migrations/` — PostgreSQL-only DDL, auto-applied at boot via advisory lock
- `third_party/dbx` — forked query builder (PgSQL dialect)
- `plugins/` — optional: jsvm hooks, migratecmd, ghupdate
- `tests/` — test harness (`tests/app.go`)
- `ui/src` — Svelte dashboard (14 field types, `ui/src/fields/<type>/`)

## Hard rules

1. Never re-introduce SQLite or `sqlite3` references — the port is complete.
2. Never break the PocketBase REST API contract without a documented,
   tested fork-delta entry and a compat regression test.
3. New DDL goes in `migrations/` (PostgreSQL syntax; follow the existing
   naming `YYYYMMDDHHMMSS_name.go` + `init()` registration).
4. SQL identifiers in DDL paths must use `dbutils.DefaultDialect.QuoteIdentifier()`.
5. Env fallbacks for tests (`PGTEST_*`) must not appear in production code paths.
6. Security-relevant changes need a regression test (SSRF guard, caps,
   quoting, auth behaviors all have existing tests to model after).

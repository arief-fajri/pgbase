# For AI Agents

<DocMeta audience="App Builder" status="stable" verified="v0.5.2 (923e860)" />

A deployment guide for AI coding agents (Claude Code, Codex, Cursor, …) that
need to provision a PG-BASE backend for an application. Everything here is
scriptable: no dashboard clicking required.

## TL;DR — one command to a running backend

```sh
curl -fsSL https://raw.githubusercontent.com/arief-fajri/pgbase/main/deploy/quickstart.sh | sh
```

Boots PG-BASE + PostgreSQL (pgvector-ready) via Docker, generates credentials,
creates the first superuser, and prints the working URLs. Idempotent —
re-running reuses the stack.

Prefer managing the database yourself? Install the binary instead:

```sh
curl -fsSL https://raw.githubusercontent.com/arief-fajri/pgbase/main/install.sh | sh
```

The binary is statically linked (CGO_ENABLED=0) and talks to any PostgreSQL
16+ — checksum-verified against the release `checksums.txt` before extracting.

## Prompt template

Paste this to your agent (adjust the bracketed parts):

```text
Provision a PG-BASE backend for this project.

1. Run the quickstart (Docker required):
   curl -fsSL https://raw.githubusercontent.com/arief-fajri/pgbase/main/deploy/quickstart.sh | sh
   It prints the dashboard URL, an API base URL, and superuser credentials
   saved in ./pgbase-quickstart/.env — read them from there.

2. Create an "items" collection via the superuser REST API:
   POST {base}/api/collections  (Authorization: superuser token from
   POST {base}/api/collections/_superusers/auth-with-password)

3. Use the PocketBase SDK for CRUD — the REST API is PocketBase-compatible:
   GET/POST {base}/api/collections/items/records with filter/sort/page params.

4. Do NOT use SQLite-specific assumptions; the storage engine is PostgreSQL
   (JSONB for schemaless fields, timestamptz for dates). Known behavior
   differences vs PocketBase: https://arief-fajri.github.io/pgbase/fork-deltas
```

## Deterministic CLI contract

Safe to call from scripts/agents — flags are stable, side effects are
predictable:

| Command | Idempotent | Notes |
|---|---|---|
| `pgbase serve` | — | long-running; `--http`, `--https`, `--origins`, `--pg-*` flags override env |
| `pgbase superuser upsert EMAIL PASS` | yes | create-or-update; prefer it over `create` |
| `pgbase superuser update/delete/otp/ips` | yes | `delete` of a missing account succeeds |
| `pgbase backup [name]` | no | unnamed runs autogenerate a new backup; pass a name to overwrite deterministically; `--format pg\|sqlite` |
| `pgbase restore NAME` | yes | offline restore from a named backup |
| `pgbase migrate up\|down\|create\|collections` | yes | schema migrations, advisory-lock serialized |

Global flags (work on subcommands): `--dir`, `--encryptionEnv`, `--dev`,
`--queryTimeout`. **Once the app booted with `--encryptionEnv=NAME`, every
CLI invocation needs the same flag** — settings secrets are encrypted with
that key (the quickstart wires it to `PB_ENCRYPTION_KEY`).

## Configuration

Connection settings come from env or CLI flags — never from the database:

| Variable | Flag | Default |
|---|---|---|
| `PB_POSTGRES_HOST` | `--pg-host` | `localhost` |
| `PB_POSTGRES_PORT` | `--pg-port` | `5432` |
| `PB_POSTGRES_USER` | `--pg-user` | — |
| `PB_POSTGRES_PASSWORD` | `--pg-password` | — |
| `PB_POSTGRES_DBNAME` | `--pg-dbname` | — |
| `PB_POSTGRES_SSLMODE` | `--pg-sslmode` | `prefer` |

Full list (pool tuning, timeouts, backup caps, rate limits):
[env reference](./reference/env.md).

## API surface

- REST + realtime: PocketBase-compatible — use the
  [PocketBase JS/Dart SDKs](https://pocketbase.io/docs) and docs.
- Differences: [fork deltas](./fork-deltas.md) (PostgreSQL-only engine,
  case-insensitive identity indexes, editor sanitization, batch transaction
  semantics — read before relying on edge behavior).

## What is deliberately NOT there yet

Honest boundaries (see the [roadmap](./roadmap.md)):

- No MCP server yet (Sprint 0b) — this page + the CLI contract are the
  agent interface today.
- No vector/similarity fields yet (Sprint 0b) — the quickstart's Postgres
  ships the pgvector extension pre-created, so the stack stays valid when
  they land.
- Multi-instance topologies need S3 file storage + the realtime outbox
  (opt-in) — see [single-host production](./single-host-production.md) first.

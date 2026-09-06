# PGBase Documentation

> Audience: All (Contributor | Operator | App Builder) · Status: stable · Last verified: v0.5.2 (`923e860`)

PGBase is a PostgreSQL-only backend-as-a-service, forked from PocketBase v0.39.11 (WIP).
It keeps the PocketBase REST API compatible while replacing SQLite with PostgreSQL
(`pgx` v5 + `dbx`, JSONB columns, `timestamptz`, `pgcrypto` IDs).

> **Live site:** <https://arief-fajri.github.io/pgbase/> (auto-deployed from `main`
> on docs changes). Local preview: `npm --prefix docs run docs:dev`
> (serves at `http://localhost:5174/pgbase/`).

## Where to start (pick your track)

| Track | Start here | Existing runbooks (do not duplicate) |
|---|---|---|
| **App Builder** (use the dashboard + API) | `fork-deltas.md` → `collections-and-api-rules.md` | Upstream [PocketBase docs](https://pocketbase.io/docs) apply except the deltas listed |
| **Operator** (self-host, backup, monitor) | `single-host-production.md` → `backup-restore-observability.md` → `reference/env.md` | `../PRODUCTION.md` is the production runbook; `../DEV.md` §3 covers pool tuning |
| **Contributor** (build, test, release) | `contributing-releasing.md` | `../DEV.md` is the single source of truth for dev; `../CONTRIBUTING.md` for PR flow |

## Page index

| Page | Audience | Status | Purpose |
|---|---|---|---|
| `fork-deltas.md` | Builder | stable | PG-only behavior deltas vs upstream (must-read before building) |
| `collections-and-api-rules.md` | Builder | stable | How to use collections + API rules + in-dashboard API preview |
| `single-host-production.md` | Operator | stable | Single-host Compose + Caddy topology, secrets, go-live checklist |
| `backup-restore-observability.md` | Operator | stable | `pg_dump` vs `sqlite` formats, offline restore, `/metrics` |
| `contributing-releasing.md` | Contributor | stable | Branch → test → lint → UI build → PR → tag → draft release |
| `reference/env.md` | All | stable | **Canonical** `PB_*` environment variable table (single source) |
| `architecture/backend-layers.md` | Contributor | stable | Layers, bootstrap chain, dual-pool DB, middleware execution order |
| `flows/auth.md` | All | stable | Password / OAuth2 / OTP / MFA / refresh / impersonate flows |
| `flows/realtime.md` | All | stable / experimental (outbox) | SSE + opt-in cross-instance outbox |
| `roadmap.md` | All | living document | Phased roadmap (multi-instance, perf, upstream tracking, observability) |

Badges: `stable` = verified on this tag. `experimental` = opt-in, needs hardening
(realtime outbox). `roadmap-open` = tracked in `roadmap.md`, not yet built
(multi-instance HA, `_logs` partitioning, GIN JSONB, OTel, PITR/WAL, K8s, SDK matrix).

## Conventions used in all pages

- File references use `path:line` (e.g. `core/base.go:43`).
- Mermaid diagrams render natively on GitHub; ASCII is used only where
  `../PRODUCTION.md` §6 already has the topology.
- Env defaults live **only** in `reference/env.md`. Other pages link there;
  `../README.md`, `../DEV.md`, `../PRODUCTION.md` tables are frozen pointers.
- Version stamp: if a page says `last-verified` older than the current tag,
  treat code as truth and open a docs issue.

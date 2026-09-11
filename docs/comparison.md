# Comparison & Positioning

<DocMeta audience="All" status="living document" verified="v0.5.2 (923e860)" />

An honest comparison to help you pick the right tool. Verified against the
repositories/codebases linked below on 2026-09-11; capabilities evolve —
re-check the links before making decisions based on this page.

## TL;DR

PG-BASE is for teams that want **the PocketBase developer experience on a
real PostgreSQL database, self-hosted as a single binary**. If that sentence
doesn't describe you, one of the tools below probably serves you better —
see "When not to choose PG-BASE".

## Feature comparison

| | **PG-BASE** | [PocketBase](https://github.com/pocketbase/pocketbase) | [Supabase](https://github.com/supabase/supabase) | [postgrebase](https://github.com/zhenruyan/postgrebase) | [pg-pocketbase](https://github.com/statewright/pg-pocketbase) |
|---|---|---|---|---|---|
| Storage engine | PostgreSQL only (pgx v5, JSONB, timestamptz) | SQLite only (officially no plans for other DBs) | PostgreSQL (+ extensions: pgvector, PostGIS, …) | PostgreSQL / MySQL + Redis cache | PostgreSQL |
| API model | PocketBase REST + realtime (compatible, [documented deltas](./fork-deltas.md)) | PocketBase | PostgREST/REST + client libraries | PocketBase-style | PocketBase REST |
| Deployment shape | 1 binary + 1 Postgres | 1 binary (embedded DB) | ~a dozen services self-hosted, or managed cloud | Docker multi-service | Overlay on upstream binary |
| Auth, rules, files, dashboard | PocketBase feature set (inherited) | yes | own model (RLS-centric) | fork-dependent | upstream feature set |
| Audit trails (reads + writes) | yes (`_audits`/`_audit_reads`, partitioned) | no | via extensions | no | no |
| Backups | native `pg_dump`/`pg_restore` + legacy SQLite import + offline CLI | SQLite file copy | platform-managed | fork-dependent | fork-dependent |
| Observability | Prometheus `/metrics` + request logs | request logs | platform tooling | fork-dependent | fork-dependent |
| Security hardening | SSRF guard, download caps, checksum-verified self-update, SHA-pinned CI/digests | baseline upstream | platform-grade | baseline upstream | upstream baseline |
| Upstream tracking | hard fork + documented drift policy (`FORK_STRATEGY.md`, repo root) | n/a (is upstream) | n/a | hard fork (PG+MySQL) | build-tag overlay — minimal drift by design |
| Test suite | 217 files, DB-per-test isolation, `-race` in CI | upstream suite | upstream suites | fork-dependent | upstream suite |
| Vector / AI features | roadmap (Sprint 0b; pgvector pre-provisioned in the quickstart) | impossible on SQLite | native (pgvector, RRF) | unknown | unknown |
| Self-host cost floor | ~$5/mo VPS (1 binary + 1 Postgres) | ~$5/mo VPS | ~$15–50/mo (multi-container) | mid (multi-service) | ~$5/mo VPS |

Where a cell says "fork-dependent" or "unknown", verify against that
project's repository — this page deliberately avoids claims it cannot check
from code.

## When to choose what

**Choose PocketBase** if SQLite's ceiling doesn't affect you: single-writer
concurrency, no extensions, and single-node deployment are fine. It is the
upstream, has the largest community, and zero fork risk.

**Choose Supabase** if you want a full platform — managed or self-hosted —
with native pgvector, Row Level Security tooling, edge functions, and a
large ecosystem, and you're fine running (or paying for) a dozen services.

**Choose PG-BASE** if you want PocketBase's programming model (collections,
API rules, SDKs, single binary, embedded dashboard) but on PostgreSQL:
concurrent writes, `pg_dump` operational workflows, audit trails, and a
path to extensions.

**Consider postgrebase / pg-pocketbase** if you need PostgreSQL with
PocketBase today and weigh their tradeoffs (broader feature surface vs
overlay-style upstream tracking) — evaluate their current state directly;
both predate PG-BASE and have their own priorities.

## When NOT to choose PG-BASE

Honest boundaries (all tracked in the [roadmap](./roadmap.md)):

- You need **vector/AI features today** — pgvector support is roadmap
  (Sprint 0b), not shipped.
- You need **horizontal scale today** — multi-instance works opt-in
  (realtime outbox, advisory-lock cron) with documented limitations
  (per-instance rate limits, S3 file storage required, fsnotify-based cache
  invalidation needs a shared data dir).
- You need **PITR/WAL archiving guidance today** — native `pg_dump`
  round-trips are covered; WAL tooling docs are roadmap (Phase 3).
- You want a **platform** (edge functions, managed cloud, RLS editor) — that
  is explicitly out of scope.

## How we stay honest

- Every fork behavior difference is documented and tested:
  [fork deltas](./fork-deltas.md).
- Performance/positioning claims must be reproducible — the roadmap's
  quality gates require benchmarks or production evidence before P2 items
  ship.
- Upstream security advisories are tracked under a documented SLA
  (`FORK_STRATEGY.md`, repo root).

See also: [for agents](./agents.md) · [single-host production](./single-host-production.md) · [roadmap](./roadmap.md)

# PG-BASE — Product Roadmap

<DocMeta audience="All" status="living document" verified="v0.6.0" />

> Canonical product roadmap. Last review: 2026-09-27 (stability-first restructure — DRR-0005).
>
> North star: **make PostgreSQL feel as easy to use as PocketBase.**
>
> That sentence is a constraint, not a parent project. PG-BASE does not track another product's releases.

---

## 0. What we are building

PG-BASE is a self-hosted PostgreSQL application backend.

> A self-hosted, single-binary application backend built around PostgreSQL — with the simplicity of PocketBase and the operational portability of PostgreSQL.

Shorter: **Your PostgreSQL. Your backend. One binary.**

PG-BASE turns a PostgreSQL database the operator already runs into an application backend: REST, auth, rules, realtime, files, and a dashboard. The database stays reachable with `psql`. There is no platform to operate.

PostgreSQL is the foundation, not the pitch. Several products already sit on PostgreSQL. The product is the application layer and the operational shape: one binary, your database, no platform tax.

### Who it is for

| ICP | Role | What they need |
|---|---|---|
| PocketBase users who need PostgreSQL without a rewrite | Acquisition (Era 1) | A repeatable import and a client that keeps working |
| Developers who already chose PostgreSQL | Long-term market (Era 2) | Auth, CRUD, rules, realtime, files, and an admin UI without assembling them |
| Agencies | Deployment story | One appliance per client app. Clear ownership, one backup, one database |
| AI coding agents | Secondary channel (Era 3) | Deterministic install, schema, and operate commands |

Era 1 is how people arrive. Era 2 is the category. Era 3 is an interface, not a new product.

### What it is not

- Not a Supabase or Appwrite clone. Do not add a feature because a platform has it.
- Not a BaaS category play. "Where are edge functions, queues, billing, and a cloud?" is the wrong question.
- Not "PocketBase, but PostgreSQL" as an identity. That sentence is a wedge. The moat is the whole experience.
- Not a hidden database. Callers and operators can use SQL.

The main alternative is a backend the team builds itself: PostgreSQL, an ORM, an HTTP framework, an auth library, object storage, realtime, and an admin UI. PG-BASE exists so that stack is optional.

---

## 1. Five layers

Layers are the product model. Phases below are the order of work.

### Layer 1 — PostgreSQL-native

JSONB, transactions, indexes, extensions, `pg_dump` / `pg_restore`, migrations, pooling, replicas, `LISTEN`/`NOTIFY`, and SQL. Do not emulate another engine's limits. Do not hide PostgreSQL.

### Layer 2 — Client compatibility

A distribution moat, not a permanent identity. PocketBase JS and Dart SDKs should keep working, and a PocketBase data directory should import with a verification report. Compatibility is tested against [the API contract](../reference/api-contract.md) and a suite we own. It is not a release-tracking process.

### Layer 3 — Production primitives

Backup, restore, restore verification, schema migrations, health and readiness, metrics, structured logs, security, rate limiting, an upgrade path, disaster recovery, and pool behavior. Boring on purpose. People buy this so production does not fall over.

### Layer 4 — PostgreSQL advantage

Capabilities an embedded-database backend cannot offer, exposed through the same simple model. Order: pgvector, then full-text search, then extension guidance (`pg_trgm`, PostGIS). This is not an AI database and not an AI platform.

### Layer 5 — Interfaces

One backend. Several doors:

```text
Human        → dashboard, CLI, SDK
AI agent     → CLI, MCP
Application  → REST, realtime
Operator     → psql, pg_dump, metrics
```

---

## 2. Moat order

Build these in order. Do not skip ahead to a cloud or a feature-parity list.

1. **Migration** — PocketBase data in, verified, minimal client change.
2. **Compatibility** — same SDK, behavior defined by our contract and tests.
3. **PostgreSQL-native capabilities** — vector, SQL, extensions, replicas.
4. **Operational simplicity** — one binary, one PostgreSQL, optional S3.
5. **Agent-native control** — CLI and MCP over the same backend.

---

## 3. What we will not build

- Edge functions or a serverless runtime.
- A private database abstraction. `psql` must keep working.
- A private query language. Simple cases use the REST API. SQL and PostgreSQL extensions stay available.
- A managed cloud, billing, orgs, or quotas before self-hosted use shows demand.
- Kubernetes, Helm, or a multi-service deploy as the default. The promise is one binary plus one PostgreSQL.
- Feature parity with Supabase or Appwrite.
- OpenTelemetry or a tracing product. Prometheus `/metrics` is the operational signal.
- PITR or WAL archiving. Point-in-time recovery stays a PostgreSQL tool, not a PG-BASE feature.
- Default Grafana dashboards or Alertmanager rules. Operators bring their own Prometheus.
- An in-process `CREATE INDEX CONCURRENTLY` path. Large indexes are built with `psql`.

---

## 4. What already exists

Verified against the tree at v0.6.0. This is inventory, not a promise that every row is finished.

| Capability | State | Anchor |
|---|---|---|
| PostgreSQL engine (`pgx` v5, JSONB, `timestamptz`) | Shipped | `core/db_connect.go`, `tools/dbutils/pgsql.go` |
| Security hardening (SSRF guard, download caps, identifier quoting, pinned CI) | Shipped | `tools/security/`, `.github/workflows/` |
| Secret scanning (gitleaks full history) | Shipped | `.gitleaks.toml`, `.github/workflows/security-scan.yaml` |
| Audit trails (`_audits`, `_audit_reads`, partitioned) | Shipped | `core/audit_hooks.go`, `migrations/` |
| Native `pg_dump` / `pg_restore` plus offline CLI | Shipped | `core/backup_pg_*.go`, `cmd/backup.go` |
| Legacy SQLite archive import | Partial — no dry-run, no report, no dedicated command | `core/backup_sqlite_import.go` |
| Realtime outbox (opt-in `LISTEN`/`NOTIFY`) | Shipped, not default | `core/realtime_outbox.go` |
| Prometheus `/metrics` | Shipped | `apis/metrics.go` |
| Pool tuning and PgBouncer notes | Shipped | `core/db_connect.go` |
| One-command provision and install | Shipped | `deploy/quickstart.sh`, `install.sh` |
| Agent quick start and CLI contract | Partial — MCP not started | `docs/agents.md` |
| Production Compose, Caddy, runbook | Shipped | `docker-compose.prod.yml`, `docs/deployment/` |
| Restore verification | Shipped | `core/backup_pg_verify.go` — archive-derived TOC gate: pre-restore archive validation, stderr classification, table/index completeness, settings/auth sanity (W-08 closed 2026-09-26) |
| pgvector field, similarity API | Not started | Phase 4 |
| MCP server | Not started | Phase 5 |
| Distributed rate limiter | Not started — limit is per instance | Phase 3 |
| Readiness distinct from liveness | Shipped | `apis/ready.go` (`GET /api/ready`: 200/503 on a data-pool + `_collections` probe; health stays liveness-only) |
| Versioning and upgrade policy | Shipped | `docs/deployment/upgrades.md` — app/PostgreSQL upgrade + rollback policy; PG 16/17 claim observed by the `pg-matrix` job in `.github/workflows/release.yaml` |

Live per-capability status — implementation, tests, failure behavior, risk — is the [Production Readiness Matrix](./production-readiness.md) (first audit 2026-09-27, re-audited at every phase exit).

System model: [Platform Design](./methodology/platform-design.md), [Guardrails](./methodology/guardrails.md), [Observability](./methodology/observability.md), [Checklists](./methodology/checklists.md). Known weaknesses stay in [Failure Analysis](./methodology/failure-modes.md).

---

## 5. Phases

Each item still needs the 10-step definition of done in its PR. See [Checklists §7](./methodology/checklists.md#7-definition-of-done-per-non-trivial-change).

The moat order (§2) is the value thesis; the phases below are the order of work. Validation is both a gate — every phase exit cites [Production Readiness Matrix](./production-readiness.md) rows it closes — and a workstream: Phase 2 starts during Phase 1 and gates v1.0.0 together with the Phase 3 topology evidence (DRR-0005).

### Phase 0 — Trust

Complete (2026-09-26): security policy, secret scanning, reproducible releases, native backups, metrics, production docs, one-command install, production-path hygiene (W-06), readiness (`/api/ready`), restore verification (W-08), diagnostic metrics, restore drills (`last_verified_backup`), reliability tests for the §1–2 shutdown/timeout gaps, and the versioning and upgrade policy with an observed PostgreSQL 16/17 CI matrix.

**Exit (met):** an operator can install, back up, restore, upgrade and roll back, and tell whether the process is ready.

### Phase 1 — Migration

Highest priority. This is the acquisition engine, not the product identity.

| Item | State | Target |
|---|---|---|
| Migration CLI | Partial | `pgbase migrate --from pocketbase` with dry-run, progress, and safe failure. Engine today: `core/backup_sqlite_import.go` |
| Verification report | Not started | Pre/post counts, relations, auth, files |
| Compatibility suite | Not started | Auth, CRUD, filter, sort, expand, relations, realtime, files, rules, JS/Dart SDKs — tested against our contract |
| Realtime correctness contract | Not started | Close W-01 (delete published pre-commit) and write the realtime delivery semantics — at-most-once vs at-least-once, replay — into the contract **before** the compatibility suite freezes: a suite built against current behavior would codify the bugs |
| Playbook | Not started | What application code must change, and how to recover a failed import |
| Endpoint reference | Not started | A published route map. Until then the in-dashboard API preview is the syntax reference |

Public status stays honest on [Migrate](../migrate.md). Do not document the CLI as shipped until it is.

**Exit:** a real PocketBase application moves to PG-BASE with minimal client changes, through a repeatable, verified process. (Tagging v1.0.0 is gated on this exit plus Phase 2 evidence plus the Phase 3 topology — criteria in `evidence/records/DRR-0004.md`, amended by `evidence/records/DRR-0005.md`.)

### Phase 2 — Production validation & evidence

A parallel workstream: it starts during Phase 1 and gates v1.0.0 — it does not block behind Phase 1 feature code. The primitives exist (see the [Production Readiness Matrix](./production-readiness.md)); this phase produces the evidence that they hold.

| Item | State | Target |
|---|---|---|
| SLO & capacity envelope | Not started | Documented targets (availability, p95/p99 latency, RTO) and measured limits (max tested RPS, concurrent clients, SSE connections, DB size, upload throughput). RPO stays operator-side — backup interval, documented and drilled, not owned (§3: no PITR) |
| Load harness | Not started | Repeatable load + capacity benchmark; results committed as reproducible evidence |
| Failure-injection automation | Partial — experiments A–E are one-shot manual records | Automated: PG restart, app crash, network partition, pool exhaustion, slow query, reconnect storm. Storage failure and burn-in stay harness-level |
| Burn-in / soak | Not started | The DRR-0004 time criterion backed by a dogfooding workload — a real application the project runs itself |
| Concurrency tests | Not started | Concurrent-writes correctness ([Checklists §1](./methodology/checklists.md)); W-16 resolved or accepted with a written decision |
| Audit DEFAULT retention | Not started — W-09 | Prune the `DEFAULT` partition; the partition cron itself is untested — both close here |
| Request-log retention | Partial — DELETE-based cleanup shipped (6 h cron, MaxDays gate, autovacuum tuning) | Range partition, or an equivalent PostgreSQL-native retention path, so cleanup stops bloating |
| JSONB filter indexes | Not started | `GIN` / `jsonb_path_ops` for multi-value filters. Those filters full-scan today |

**Exit:** every production claim has a number, a test scenario, and reproducible evidence — no claim without a record.

### Phase 3 — HA & Scale

The documented multi-instance topology is part of the v1.0.0 gate (DRR-0005). Single instance stays the default shape; this phase makes `LB → N instances → PostgreSQL` a tested, documented pattern instead of an implied one.

| Item | State | Target |
|---|---|---|
| Multi-instance topology test | Not started | The documented topology survives failure-injection + load from the Phase 2 harness, incl. restart without duplicated jobs and a PgBouncer-tested leg |
| Realtime outbox hardening | Opt-in — W-01 fix ordered into Phase 1 | Replay on restart (W-02), then the default-on decision. Metrics: active subscriptions, outbox lag, broadcast latency, reconnects (I15) |
| Distributed rate limiter | Not started | N instances share one limit — or per-instance math stays the documented contract |
| Cache invalidation | Partial | DB `LISTEN` instead of a filesystem sentinel. Cross-host staleness is security-relevant |
| Cron guard | Open — fails open on a lock-acquisition error (W-03) | No duplicated jobs on every instance under DB errors |
| File storage | Partial | S3 required for more than one instance. Warn if local storage sees multiple heartbeats (W-05) |
| PgBouncer | Documented | Tested against a real pooler, not only notes |
| Replicas | Not started | Read-replica guidance after the single-primary path is boring — guidance, not a failover manager (§3) |

**Exit:** `LB → N instances → PostgreSQL` is a tested, documented pattern within the Phase 2 capacity envelope. Local disk and per-instance limits are called out, not implied away.

### Phase 4 — PostgreSQL advantage

| Item | State | Target |
|---|---|---|
| Vector field | Not started | Dimension and distance metric on a collection field |
| Vector indexes | Not started | IVFFlat / HNSW, with written trade-offs |
| Similarity API | Not started | Top-K through the REST API, with filter |
| Hybrid search | Later in this phase | Full-text plus vector. Needs sort grammar in `tools/search` |
| Full-text guidance | Not started | PostgreSQL FTS through the same model |
| Extension guidance | Later | `pg_trgm`, PostGIS — guidance, not a new platform |

**Exit:** a developer creates a vector field, inserts rows, and queries top-K without custom SQL. The page does not call PG-BASE an AI database.

### Phase 5 — Agent interface & Ecosystem

One-command provision and `docs/agents.md` already exist. Starter templates are the landing zone migrated apps need.

| Item | State | Target |
|---|---|---|
| MCP server | Not started | `pgbase mcp`. v1 is read-mostly: collections, schema, records, auth. Admin tools later |
| CLI contract | Partial | `serve`, `migrate`, `backup`, `restore`, `superuser` idempotent where claimed, flags documented |
| Schema from the agent | Not started | Create and alter collections from CLI or MCP without the dashboard |
| Starter templates | Not started | Next.js, Svelte, Nuxt, Flutter, and one AI kit |
| Webhook primitive | Not started | Small, with retries — not a workflow platform |

**Exit:** an agent goes from zero to a running backend, then creates a collection and a record, with commands that do the same thing twice; a new PostgreSQL app has a template that reaches a running PG-BASE without a platform signup.

Cloud, billing, and a control plane stay out of scope until self-hosted use shows demand.

---

## 6. Rules

- **No platform parity.** A feature request that starts with "Supabase has X" is not a reason to build X.
- **Contract rule.** Caller-visible changes update [api-contract.md](../reference/api-contract.md) and add a regression test.
- **Evidence rule.** Performance and scale claims ship with a reproducible result.
- **Validation gate.** Every phase exit cites [Production Readiness Matrix](./production-readiness.md) rows it closes — a phase is done when its rows are green with evidence, not when its code merges.
- **Honesty rule.** Docs do not describe planned commands of any phase as available until they are.
- **Failure rule.** Classify A–E in [Failure Analysis](./methodology/failure-modes.md) before changing code.

---

## 7. Risks

| Risk | Why it matters | Response |
|---|---|---|
| Teams build the stack themselves | This is the main competitor | Keep the path from PostgreSQL to a working backend under ten minutes |
| Migration trust | A bad import ends the acquisition channel | Dry-run, verification, no silent success |
| Simplicity erosion | Every extra service taxes the promise | Refuse edge functions, Kubernetes-first, and a cloud control plane |
| Contract drift | SDKs work until they don't | Compatibility suite against our contract, not a foreign release feed |
| Restore doubt | Backups that were never restored are not backups | Phase 0 leftover stays P0 |
| Multi-instance inconsistency | Scale claims without tests | Phase 3 is gated on a real topology test (in the v1.0.0 gate — DRR-0005) |
| v1.0.0 delay | HA joined the 1.0 gate (DRR-0005) | The blocker set is enumerated so the gate stays auditable; dogfooding burn-in absorbs wall-clock cost |

---

## 8. How to contribute

Open an issue that names the phase and the item (`Phase 1 / verification report`).

- Contract changes include a regression test and an `api-contract.md` edit.
- Migration changes include verification and failure handling.
- Scale changes include a multi-instance test plan.
- Performance changes include a before/after result.

See [contributing.md](./contributing.md), [developing.md](./developing.md), and the [production runbook](../deployment/production.md).

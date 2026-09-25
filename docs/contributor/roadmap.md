# PG-BASE — Product Roadmap

<DocMeta audience="All" status="living document" verified="v0.5.4" />

> Canonical product roadmap. Last review: 2026-09-25 (positioning rewrite).
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

Verified against the tree at v0.5.4. This is inventory, not a promise that every row is finished.

| Capability | State | Anchor |
|---|---|---|
| PostgreSQL engine (`pgx` v5, JSONB, `timestamptz`) | Shipped | `core/db_connect.go`, `tools/dbutils/pgsql.go` |
| Security hardening (SSRF guard, download caps, identifier quoting, pinned CI) | Shipped | `tools/security/`, `.github/workflows/` |
| Audit trails (`_audits`, `_audit_reads`, partitioned) | Shipped | `core/audit_hooks.go`, `migrations/` |
| Native `pg_dump` / `pg_restore` plus offline CLI | Shipped | `core/backup_pg_*.go`, `cmd/backup.go` |
| Legacy SQLite archive import | Partial — no dry-run, no report, no dedicated command | `core/backup_sqlite_import.go` |
| Realtime outbox (opt-in `LISTEN`/`NOTIFY`) | Shipped, not default | `core/realtime_outbox.go` |
| Prometheus `/metrics` | Shipped | `apis/metrics.go` |
| Pool tuning and PgBouncer notes | Shipped | `core/db_connect.go` |
| One-command provision and install | Shipped | `deploy/quickstart.sh`, `install.sh` |
| Agent quick start and CLI contract | Partial — MCP not started | `docs/agents.md` |
| Production Compose, Caddy, runbook | Shipped | `docker-compose.prod.yml`, `docs/deployment/` |
| Restore verification | Partial | roadmap Phase 0 leftover |
| pgvector field, similarity API | Not started | Phase 2 |
| MCP server | Not started | Phase 3 |
| Distributed rate limiter | Not started — limit is per instance | Phase 4 |
| Readiness distinct from liveness | Not started — `/api/health` does not ping the database | Phase 0 leftover |

System model: [Platform Design](./methodology/platform-design.md), [Guardrails](./methodology/guardrails.md), [Observability](./methodology/observability.md), [Checklists](./methodology/checklists.md). Known weaknesses stay in [Failure Analysis](./methodology/failure-modes.md).

---

## 5. Phases

Each item still needs the 10-step definition of done in its PR. See [Checklists §7](./methodology/checklists.md#7-definition-of-done-per-non-trivial-change).

### Phase 0 — Trust

Most of this is done: security policy, reproducible releases, native backups, metrics, production docs, one-command install, production-path hygiene (W-06).

Still P0, because "we have a backup" is not "we trust a restore":

| Item | State | Target |
|---|---|---|
| Restore verification | Partial | Post-restore counts, expected schema, settings sanity. A partial restore fails loudly. |
| Restore drills | Not started | A recorded drill that feeds `last_verified_backup` |
| Readiness | Partial | `/api/ready` checks the database. Liveness is not readiness. |
| Versioning and upgrade policy | Not started | How PG-BASE versions break, how to roll back an app or PostgreSQL upgrade, and a PostgreSQL 16/17 CI matrix so that claim is observed |
| Secret scanning | Not started | A CI job that fails the build on committed secrets (G-SEC-01), alongside govulncheck and npm audit |
| Diagnostic metrics | Not started | Error ratio (`pgbase_http_requests_total`), DB event counters (query/lock timeout, reconnect, rollback) for G-DB-03/04 and G-REL-01, and backup attempt/success/duration/size. `last_verified_backup` stays with restore drills |
| Reliability tests | Not started | Query-timeout, connect-timeout, and graceful-shutdown tests — Evaluation Checklists §1–2 gaps |

**Exit:** an operator can install, back up, restore, and tell whether the process is ready — with the gaps above closed.

### Phase 1 — Migration

Highest priority. This is the acquisition engine, not the product identity.

| Item | State | Target |
|---|---|---|
| Migration CLI | Partial | `pgbase migrate --from pocketbase` with dry-run, progress, and safe failure. Engine today: `core/backup_sqlite_import.go` |
| Verification report | Not started | Pre/post counts, relations, auth, files |
| Compatibility suite | Not started | Auth, CRUD, filter, sort, expand, relations, realtime, files, rules, JS/Dart SDKs — tested against our contract |
| Playbook | Not started | What application code must change, and how to recover a failed import |
| Endpoint reference | Not started | A published route map. Until then the in-dashboard API preview is the syntax reference |

Public status stays honest on [Migrate](../migrate.md). Do not document the CLI as shipped until it is.

**Exit:** a real PocketBase application moves to PG-BASE with minimal client changes, through a repeatable, verified process.

### Phase 2 — PostgreSQL advantage

| Item | State | Target |
|---|---|---|
| Vector field | Not started | Dimension and distance metric on a collection field |
| Vector indexes | Not started | IVFFlat / HNSW, with written trade-offs |
| Similarity API | Not started | Top-K through the REST API, with filter |
| Hybrid search | Later in this phase | Full-text plus vector. Needs sort grammar in `tools/search` |
| Full-text guidance | Not started | PostgreSQL FTS through the same model |
| Extension guidance | Later | `pg_trgm`, PostGIS — guidance, not a new platform |

**Exit:** a developer creates a vector field, inserts rows, and queries top-K without custom SQL. The page does not call PG-BASE an AI database.

### Phase 3 — Agent interface

One-command provision and `docs/agents.md` already exist.

| Item | State | Target |
|---|---|---|
| MCP server | Not started | `pgbase mcp`. v1 is read-mostly: collections, schema, records, auth. Admin tools later |
| CLI contract | Partial | `serve`, `migrate`, `backup`, `restore`, `superuser` idempotent where claimed, flags documented |
| Schema from the agent | Not started | Create and alter collections from CLI or MCP without the dashboard |

**Exit:** an agent goes from zero to a running backend, then creates a collection and a record, with commands that do the same thing twice.

### Phase 4 — Scale

Single instance first. Scale out when you need it. The target is not "more scalable than a platform."

| Item | State | Target |
|---|---|---|
| Realtime outbox | Opt-in | Publish after commit, replay on restart, then consider default-on. Metrics: active subscriptions, outbox lag, broadcast latency (I15) |
| Distributed rate limiter | Not started | N instances share one limit |
| Cache invalidation | Partial | DB `LISTEN` instead of a filesystem sentinel. Cross-host staleness is security-relevant |
| File storage | Partial | S3 required for more than one instance. Warn if local storage sees multiple heartbeats |
| PgBouncer | Documented | Keep the transaction-mode notes accurate |
| Replicas | Not started | Read-replica guidance after the single-primary path is boring |
| Load harness | Not started | Topology survives restart without duplicated jobs, including the cron guard that fails open on a lock-acquisition error (W-03), plus concurrency tests (Evaluation Checklists §1) |
| JSONB filter indexes | Not started | `GIN` / `jsonb_path_ops` for multi-value filters. Those filters full-scan today |
| Request-log retention | Not started | `_logs` cleanup must not rely on bulk `DELETE`. Range partition, or an equivalent PostgreSQL-native retention path |
| Audit DEFAULT retention | Not started | The `DEFAULT` partition is pruned. Named partitions already drop (W-09) |

**Exit:** `LB → N instances → PostgreSQL` is a tested, documented pattern. Local disk and per-instance limits are called out, not implied away.

### Phase 5 — Ecosystem

Only after Phases 1–3 are real.

Starter templates (Next.js, Svelte, Nuxt, Flutter, and one AI kit). SDK notes that point at our contract. A small webhook primitive with retries — not a workflow platform.

**Exit:** a new PostgreSQL app has a template that reaches a running PG-BASE without a platform signup.

Cloud, billing, and a control plane stay out of scope until self-hosted use shows demand.

---

## 6. Rules

- **No platform parity.** A feature request that starts with "Supabase has X" is not a reason to build X.
- **Contract rule.** Caller-visible changes update [api-contract.md](../reference/api-contract.md) and add a regression test.
- **Evidence rule.** Performance and scale claims ship with a reproducible result.
- **Honesty rule.** Docs do not describe Phase 1–3 commands as available until they are.
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
| Multi-instance inconsistency | Scale claims without tests | Phase 4 is gated on a real topology test |

---

## 8. How to contribute

Open an issue that names the phase and the item (`Phase 1 / verification report`).

- Contract changes include a regression test and an `api-contract.md` edit.
- Migration changes include verification and failure handling.
- Scale changes include a multi-instance test plan.
- Performance changes include a before/after result.

See [contributing.md](./contributing.md), [developing.md](./developing.md), and the [production runbook](../deployment/production.md).

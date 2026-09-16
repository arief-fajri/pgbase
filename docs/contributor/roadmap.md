# PG-BASE — Product & Development Roadmap

<DocMeta audience="All" status="living document" verified="v0.5.2 (923e860)" />

> Single source of truth (git-tracked). Working drafts (`feature-plan/`) are retired — this file is canonical.
> Baseline: v0.5.2 (`923e860`) · Last review: 2026-09-12 (adopted system-thinking methodology).
>
> **Development is held on Sprint 0b until the methodology gate (below) passes.** See [§7](#7-sequencing-gate-and-exit-criteria).
> All prior roadmap documents (roadmap-v2, v3, v4, v5, consolidation, `feature-plan` drafts) are superseded by this file.

---

## 0. Product thesis

### Problem

PocketBase is simple and productive, but its SQLite-only architecture creates a permanent ceiling: single-writer concurrency, no horizontal scaling, no PostgreSQL extensions (pgvector, PostGIS, pg_trgm), and no path to production-grade operations without leaving PocketBase entirely. Today, users who hit that wall leave for Supabase — a platform that requires a dozen containers and $15–50/mo just to self-host.

### Promise

> **PocketBase DX. PostgreSQL underneath. Self-hosted by default.**

PG-BASE exists for one reason: give teams the PocketBase developer experience on a real PostgreSQL database, deployable as a single binary on a $5/mo VPS, with a clear upgrade path to horizontal scale when they need it.

### Primary users

1. **PocketBase users at the SQLite wall** — need concurrent writes, horizontal scale, or PostgreSQL extensions.
2. **Self-hosted developers** who find Supabase's multi-container stack too heavy and want a focused PostgreSQL backend.
3. **Teams starting new applications** that require PostgreSQL from day one but prefer the PocketBase programming model.

### What PG-BASE is not

PG-BASE is not a Supabase clone, not a general-purpose backend platform, and not a managed service. The strategic advantage is **simplicity + PostgreSQL + PocketBase API compatibility + self-hosting**.

---

## 1. Release readiness assessment

Verified against the actual codebase on 2026-09-08 (v0.5.2 lineage). Status updated 2026-09-12 to reflect Sprint 0a completion.

### Fully implemented

| Feature | Evidence |
|---|---|
| **PostgreSQL port** (pgx v5, PgSQLDialect, forked dbx) | `core/db_connect.go:88` opens via pgx; `tools/dbutils/pgsql.go`; `third_party/dbx/builder_pgsql.go` |
| **Security hardening** (SSRF guard, download caps, checksum-verified updates, SQL identifier quoting, pinned CI/digests, `sslmode=prefer` default, encryption-key guidance) | `tools/security/httputil.go`; `tools/archive/extract.go`; `core/db_connect.go:79-86` |
| **Audit trails** (`_audits` + `_audit_reads`, month-partitioned, batched async writer, retention cron) | `core/audit_hooks.go`; `core/audit_writer.go`; `migrations/1787237001_audits_init.go`; `apis/audits_test.go` |
| **Native pg_dump/pg_restore backups** + legacy SQLite round-trip + offline CLI | `core/backup_pg_*.go`; `core/backup_sqlite_import.go`; `cmd/backup.go` |
| **Cross-instance realtime outbox** (NOTIFY/LISTEN, cursor pagination, origin stamping, TTL cleanup) | `core/realtime_outbox.go`; `apis/realtime_outbox_*_test.go` |
| **Prometheus metrics** (`/metrics` on dedicated listener, HTTP histograms, DB pool stats, realtime collectors) | `apis/metrics.go`; `apis/metrics_test.go` |
| **Connection pool tuning** (data 80/aux 10 defaults, env vars, PgBouncer compat, role-level timeouts) | `core/base.go:43-47`; `core/db_connect.go:64-165` |
| **CI/CD** (GoReleaser, PostgreSQL test container, `-race`, golangci-lint, security-scan, SHA-pinned actions) | `.github/workflows/*` |
| **Dashboard UI** (audit trails UI, backup format selector, PG-BASE branding) | `ui/src/audits/`; `ui/package.json` |
| **PostgreSQL-native migrations** (16+ migrations + migration-level tests) | `migrations/` |
| **Test suite** (217+ test files, DB-per-test via `CREATE DATABASE ... TEMPLATE`, ~1:1 test-to-prod LOC) | `tests/app.go` |
| **Production deployment** (docker-compose.prod.yml + Caddy auto-HTTPS, monitoring stack, runbook) | `docker-compose.prod.yml`; `deploy/`; `docs/deployment/production.md` |
| **Trust & trial path (Sprint 0a)** — forked SECURITY.md, one-command provision (`deploy/quickstart.sh`, `install.sh`), agent quickstart (`docs/agents.md`), positioning (`docs/comparison.md`), upstream drift decision (`FORK_STRATEGY.md`), published releases | `docs/agents.md`; `docs/comparison.md`; `FORK_STRATEGY.md` |

### Partially implemented

| Feature | What exists | What's missing |
|---|---|---|
| **PocketBase → PG-BASE migration** | Legacy SQLite backup import (`core/backup_sqlite_import.go`) handles v0.22/v0.23 schema, field conversion, auth migration | No dedicated `pgbase migrate` CLI; no compatibility test suite/matrix; no dry-run/verification workflow (Sprint 0c) |
| **Deterministic CLI contract** | `serve`, `superuser`, `backup`, `restore`, `migrate` documented and mostly idempotent | Full idempotence + flag-documentation audit for agent consumption (Sprint 0b) |
| **S3 backup offload + retention** | First-class S3 wiring + auto-backup cron with `CronMaxKeep` retention (default 3) | No PITR/WAL; no documented lifecycle beyond `CronMaxKeep`; no automated restore drills |

### Not started

| Feature | Status |
|---|---|
| **pgvector support** | Zero code, zero dependency, zero migration (Sprint 0b, held) |
| **MCP server** | Zero references in codebase (Sprint 0b, held) |
| **Distributed rate limiter** | In-memory per-instance; N instances = N× limit |
| **Multi-instance load/soak testing harness** | No k6/vegeta scenarios exist |
| **OpenTelemetry tracing** | No OTel dependency or instrumentation |
| **Health/readiness (liveness ≠ readiness)** | `/api/health` returns static 200 without DB ping |
| **Webhooks with retries/idempotency/DLQ** | Not implemented |

### System-state snapshot (invariant / guard-rail / observability coverage)

The system model that owns the rows below is [Platform Design](./methodology/platform-design.md) (invariants), [Quality Guardrails](./methodology/guardrails.md) (never-allowed), [Observability](./methodology/observability.md) (proof), [Evaluation Checklists](./methodology/checklists.md) (evaluation gates). Roadmap items exist to move rows from `gap` to `✓`.

| Domain | State | Primary proof | Open gaps |
|---|---|---|---|
| Data integrity (I1–I5, G-DATA) | **✓ mostly** | tx/rollback tests, migration tests | migration-failure experiment D; restore gate hardening (Task 45) |
| API compatibility (I6–I8, G-API) | **⚠️ gaps** | `fork-deltas.md` contract | compatibility test suite (Tasks 18/24) |
| Security (I9–I12, G-SEC) | **✓ strong** | security tests, prod compose, metrics guard | CI secret scan (G-SEC-01) |
| Reliability (G-REL) | **⚠️ partial** | layered timeouts (30s/60s/30s/10s) | experiments A (outage), B (pool saturation) |
| Observability (§6 fragment: RED + pool + realtime) | **⚠️ partial** | `apis/metrics.go` | backup metrics incl. `last_verified_backup`; timeout/rollback counters |
| Disaster recovery (I5, G-REL-04) | **⚠️ partial** | `pg_dump`/`pg_restore` path | restore verification (Task 45/44); RPO/RTO drill (experiment E) |
| Upgrade/upstream (G-UPG) | **✓ mostly** | `FORK_STRATEGY.md`, `UPSTREAM.md`, fork-deltas | versioning policy (Task 23); upgrade runbook (Task 47); PG 16/17 matrix (Task 59) |

### Known weaknesses

Managed canonically in [Failure Analysis §2](./methodology/failure-modes.md#2-known-weaknesses-verified-in-code-2026-09-08-audit) (W-01…W-09 with A–E classification and location). Roadmap tracks the *remedy* tasks (see Theme E and Theme F); the failure doc tracks the *behavior*.

---

## 2. Market context

### Macro trends (validated data, 2026)

| Signal | Data | Implication |
|---|---|---|
| BaaS market | $31.36B (2025) → $114.05B (2035), ~13.78% CAGR | Large, growing, structurally healthy |
| PostgreSQL adoption | 80% of startup primary databases (up from 76%) | Postgres is the default relational engine |
| Supabase growth | ~$170M ARR (May 2026), +221% YoY, database launches +600% YoY | "Open Postgres BaaS" demand validated at scale |
| Agent-driven demand | 60%+ of new Supabase databases created by AI tools | The greenfield acquisition channel of 2026 |
| Vector search | Postgres + pgvector = default vector stack; hybrid FTS + vector (RRF) = standard RAG pattern | Vector is table stakes, not a feature |
| Self-hosting economics | Self-hosted Supabase ≈ dozen containers, $15–50/mo; PocketBase = single binary, ~$5/mo | Cost & simplicity gap is defensible |

> Sourcing note: market figures are secondary-source estimates that cannot be verified from this repository. Before external publication, attach citations or soften the wording.

### The PocketBase ceiling

- PocketBase official FAQ: **"PocketBase uses embedded SQLite... there are no plans for supporting other databases."**
- Full backward compatibility not guaranteed before v1.0; not recommended for production-critical apps before v1.0.
- Every PocketBase user who needs concurrent writes, horizontal scale, or PostgreSQL extensions **must leave PocketBase**.
- PG-BASE's wedge: **keep the PocketBase DX, replace SQLite with PostgreSQL**.

---

## 3. Competitive positioning

| Project | Approach | Threat level |
|---|---|---|
| **pocketbase/pocketbase** | Upstream; SQLite-only; 60K+ community | Partner + constraint (API compatibility source) |
| **zhenruyan/postgrebase** | Fork: PG + MySQL, Redis cache, multi-instance; established since Oct 2023 | High — most mature rival fork |
| **statewright/pg-pocketbase** | Build-tag overlay (not a fork); minimal drift; LISTEN/NOTIFY, advisory locks | Strategic — the structure competitors may converge on |
| **arief-fajri/pgbase** | Hard fork at ~v0.39.11-lineage; deep security/perf hardening; audit trails; native pg_dump | — |

**Critical reading:** no leader in the "PocketBase + Postgres" category. Whoever ships **trust + capability + clear "why us"** wins. PG-BASE's hardening is a head start on trust; capability gaps (pgvector, migration CLI, agent tooling) are the blockers.

---

## 4. SWOT

### Strengths

- Production-grade PostgreSQL port (pgx v5, JSONB, `timestamptz`, `pgcrypto`).
- Security hardening rival forks lack: SSRF guard, download caps, checksum-verified updates, pinned CI, encryption-key guidance.
- Native `pg_dump`/`pg_restore` backups + offline CLI + legacy SQLite import.
- Audit trails (`_audits`/`_audit_reads`) — no rival fork offers this.
- Cross-instance realtime outbox, Prometheus metrics, runbook, monitoring stack.
- 217+ test files with database-per-test isolation.

### Weaknesses

- Fork drift risk: upstream at v0.40.x; PG-BASE tracks ~v0.39.11-lineage.
- No AI/vector story despite running on PostgreSQL.
- No dedicated migration tool from stock PocketBase.
- No MCP server or agent-facing tooling beyond the CLI contract.
- Restore verification is weak today (W-08); observability gaps vs multi-instance.
- **Methodology gate closed (2026-09-12)** — system-thinking restructure landed and experiments A–E all pass (E1–E8).

### Opportunities

- Agent economy: AI agents create the majority of new backends; single binary + Postgres is the ideal agent-deploy shape.
- pgvector + hybrid search as a native differentiator stock PocketBase can never add.
- Self-hosting simplicity gap vs Supabase stack heaviness.
- Category leadership: the niche has no incumbent.

### Threats

- Build-tag overlays (no-drift architecture) become the default; hard forks lose.
- Supabase keeps absorbing the wedge (native pgvector, RLS, agent-friendly).
- Upstream CVE reputational damage if drift response is slow.
- Maintainer burnout with slow adoption feedback loop.

---

## 5. Guiding principles

1. **PocketBase compatibility is a product feature.** Existing applications should require minimal or no code changes.
2. **Migration is first-class.** Moving from PocketBase to PG-BASE must be easier than rewriting the application.
3. **PostgreSQL-native, not SQLite-emulated.** Use PostgreSQL capabilities when they create measurable user value.
4. **Safe defaults, opt-in complexity.** Single-instance stays simple; scale features are easy to enable.
5. **No silent divergence.** Every compatibility difference must be intentional, documented, and tested.
6. **Evidence over speculation.** Claims require reproducible evidence (L0–L3, `evidence/`).
7. **Production trust before feature breadth.** Security, upgrades, backups, restore, and observability outrank experimental features.
8. **Compatibility before optimization.** A faster incompatible implementation does not satisfy the product promise.
9. **Greenfield capability drives adoption.** Features that help new users/agents pick PG-BASE outrank deeper operational polish.

### 5.1 Prioritization rule (system-thinking two-axis)

Priority is scored on **two axes**; an item needs both:

```text
Priority = (A) system-risk gap closed   ×   (B) product value delivered
```

- **A (system-risk gap):** does the item convert a system-state row from `⚠️ gap` → `✓` in [§1 snapshot](#system-state-snapshot-invariant--guard-rail--observability-coverage)? (0 = no state change; 1 = closes one gap; 2 = closes multiple or a security/correctness-critical gap)
- **B (product value):** adoption/capability impact per the themes below, weighted by acquisition principle (§5, principle 9).

Both axes must score ≥ 1 for a P0/P1. Items that only add product value without touching a system gap are P2 or context-dependent. This replaces ad-hoc ordering: **trust and correctness gaps outrank feature breadth**, which is what "production trust before feature breadth" already claimed.

---

## 6. Roadmap themes

Each theme notes its **system-thinking hook** (property changed, related invariants/guardrails, how the result is observed and evidenced). The full 10-step DoD per change lives in the PR ([Evaluation Checklists §7](./methodology/checklists.md#7-definition-of-done-per-non-trivial-change)).

### Theme A — AI-native capability (Sprint 0b, **HELD on methodology gate**)

pgvector and vector search are table stakes in 2026. Stock PocketBase cannot add this on SQLite.

> **ST hook:** *new system property* — vector storage/similarity. Invariants: I2/I4 (schema sync, explicit DDL). Guard rails: G-DATA-03, G-API-05, G-DB-03. Observe: query latency/top-K correctness. Evidence: compatibility test + index trade-off benchmark (L3).

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **pgvector field type** | NOT_STARTED | `vector` column type with dimension + distance metric options, exposed through PocketBase-style schema/API | P0 / L |
| **Vector indexes** | NOT_STARTED | IVFFlat / HNSW with documented trade-offs (accuracy vs speed vs memory) | P0 / M |
| **Similarity query API** | NOT_STARTED | `sort=embedding.<metric>` with filter + top-K through REST API | P0 / M |
| **Hybrid search** | NOT_STARTED | FTS + vector reciprocal rank fusion | P1 / M (needs new sort grammar in `tools/search`) |
| **Embedding-on-write hook** | NOT_STARTED | Webhook/jsvm hook to auto-populate vectors on create/update | P1 / M |
| **Full-text search guidance** | NOT_STARTED | PostgreSQL FTS examples + integration patterns | P1 / S |

**Definition of success:** a developer creates a `vector` field, inserts records, and queries top-K similarity through the documented API with no custom SQL.

### Theme B — Agent deployability (Sprint 0b, **HELD**)

AI coding agents are the #1 source of new backend demand; the CLI contract is the agent interface today.

> **ST hook:** *behavioral contract* — deterministic CLI idempotence. Guard rails: G-AI-01, G-API (CLI is not REST). Observe: flag documentation + idempotence tests. Evidence: agent-followable script run (L1+).

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **MCP server** | NOT_STARTED | `pgbase mcp` — v1 read-mostly tools (collections, schema, records, auth); admin tools P2 | P0 / L |
| **Deterministic CLI contract** | PARTIAL | `serve`, `migrate`, `backup`, `restore`, `superuser` idempotent + flag-documented | P0 / M |
| **One-command provision** | DONE (Sprint 0a) | `deploy/quickstart.sh` + `install.sh` verified | P0 / M ✅ |
| **Agent-facing quick start** | DONE (Sprint 0a) | `docs/agents.md` | P0 / S ✅ |
| **Positioning & comparison doc** | DONE (Sprint 0a) | `docs/comparison.md` | P0 / S ✅ |

**Definition of success:** an agent (or human) goes zero → running PG-BASE + Postgres backend in under 10 minutes.

### Theme C — PocketBase migration (Sprint 0c)

The most obvious acquisition channel: existing PocketBase users who need PostgreSQL.

> **ST hook:** *data integrity + compatibility*. Invariants: I5 (restorable/verifiable), I6. Guard rails: G-DATA-04/05, G-API-01/04/05, G-UPG-05. Observe: verification report (pre/post counts). Evidence: migration verification run (L3) + compat suite in CI.

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **Migration CLI** | PARTIAL | `pgbase migrate --from pocketbase` with validation, dry-run, progress, safe failure handling (engine exists: `core/backup_sqlite_import.go`) | P0 / M |
| **Compatibility test suite** | NOT_STARTED | Automated tests: PB REST API, auth, filters, relations, expand, realtime, files, SDK behavior | P0 / L |
| **Compatibility matrix** | NOT_STARTED | Document supported PB behavior, JS/Dart SDK versions, PG versions, known differences | P0 / M |
| **Migration verification** | NOT_STARTED | Pre/post counts, relation checks, auth checks, checksums, compatibility report | P0 / M |
| **Migration playbook** | NOT_STARTED | Minimal-code migration guide + failure recovery | P0 / S |

**Definition of success:** a real PocketBase application migrates to PG-BASE with minimal code changes through a repeatable, verifiable process.

### Theme D — Upstream trust & releases

The fork model is the #1 existential technical risk; drift must be addressed deliberately.

> **ST hook:** *upgrade/upstream invariants*. Guard rails: G-UPG-01…06, G-AI (DIVERGE = B2). Observe: upstream triage log in CHANGELOG. Evidence: security-backport test + compatibility suite.

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **Upstream drift decision** | DONE (Sprint 0a) | `FORK_STRATEGY.md` — hard fork + watch→triage→act + cost model | P0 / M ✅ |
| **CVE/advisory triage SLA** | DONE (Sprint 0a) | SLA table in `FORK_STRATEGY.md` §5; `UPSTREAM.md` snapshot | P0 / M ✅ |
| **Reproducible releases** | DONE (Sprint 0a) | GoReleaser + published v0.1.0–v0.5.2 releases | P0 / S ✅ |
| **Security policy & reporting** | DONE (Sprint 0a) | Forked `.github/SECURITY.md` (GitHub private vuln reporting) | P0 / S ✅ |
| **CI hygiene quick wins** | DONE (Sprint 0a) | `-race` + golangci-lint blocking jobs, weekly security-scan, SHA-pinned lychee with `failIfEmpty` | P0 / S ✅ |
| **CI secret scan (G-SEC-01)** | NOT_STARTED | Extend security-scan with a secret scanner | P0 / S — **methodology gap gate** |
| **Versioning & upgrade policy** | NOT_STARTED | Versioning scheme, breaking-change policy, security-backport policy | P0 / S |
| **Upgrade runbook** | NOT_STARTED | PostgreSQL major-version upgrades + PG-BASE app upgrades with rollback | P1 / M |
| **Dependency update cadence** | AD_HOC | Dependabot/Renovate for pinned digests | P1 / M |
| **Security regression tests** | PARTIAL | SSRF, download caps, identifier quoting, auth, permissions, encryption-key now tested; extend to compat | P1 / M |

### Theme E — Horizontal scalability (P1)

Several subsystems assume one process; this blocks honest multi-instance claims.

> **ST hook:** *correctness under scale*. Directly remediates FAILURE-MODES W-01…W-05, W-07 (class B/C). Guard rails: G-REL, G-DB-02/07/08. Observe: multi-instance integration test + load harness metrics. Evidence: load/soak results (L3) + failure experiments.

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **Realtime outbox hardening** | IMPLEMENTED (opt-in) — W-01/W-02 open | Fix phantom-delete (publish post-commit) + replay-on-restart; default-on only after hardening, load-test + HA docs | P1 / M |
| **Distributed rate limiter** | NOT_STARTED | Shared PG/Redis store; N instances = configured limit, not N× limit | P1 / M |
| **Cache invalidation (DB-backed)** | PARTIAL — W-04 | Replace fsnotify sentinel with DB LISTEN; today cross-host = stale settings (security-relevant) | P1 / M |
| **Cron coordination fail-open** | IMPLEMENTED — W-03 | Document + mitigate lock-acquisition failure edge; single-fire semantics | P1 / S |
| **Multi-instance file storage** | PARTIAL — W-05 | Document S3 requirement; startup warning for local storage + N heartbeats | P1 / S |
| **Token revocation** | NOT_STARTED | Optional shared immediate-revocation for cross-instance logout | P2 / M |
| **PgBouncer + pool-sizing guidance** | IMPLEMENTED | Validated transaction-mode setup documented end-to-end | P1 / S |
| **Multi-instance test environment** | NOT_STARTED | Reproducible N-instance integration/load environment (today peers are simulated by direct outbox rows) | P1 / M |

**Definition of success:** `LB → N PG-BASE instances → PostgreSQL` is a tested, documented deployment pattern surviving instance restarts without duplicated jobs, broken realtime, or inconsistent security behavior.

### Theme F — Backup & disaster recovery (P1)

Native `pg_dump` is solid for small/medium; production PostgreSQL needs a broader recovery story.

> **ST hook:** *invariant I5 + G-REL-04*. Directly related to **experiment E** (methodology gate). Security critical: restore gate is currently weak (W-08). Docs: [Disaster Recovery](../deployment/disaster-recovery.md).

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **Restore verification (hardened)** | PARTIAL — W-08 | Post-restore: per-table counts, expected schema, settings sanity; partial restore fails loudly | P1 / S |
| **Restore drills** | NOT_STARTED | Automated or documented restore verification with integrity checks — feeds `last_verified_backup` | P1 / M |
| **PITR/WAL archiving guidance** | NOT_STARTED | pgBackRest/WAL-G for large databases | P1 / M |
| **S3-compatible backup offload** | IMPLEMENTED | Wiring + retention exist; add documented lifecycle policy + large-object verification | P2 / S |
| **RPO/RTO guidance** | NOT_STARTED | Dump-based vs WAL/PITR expectations (defaults: RPO ≤ 15m, RTO ≤ 1h — unverified targets) | P1 / S |
| **Upgrade runbook** | NOT_STARTED | PG major-version upgrades + app upgrades | P1 / M |

### Theme G — Observability & operability (P1)

Metrics exist; production users need enough visibility to diagnose across instances and PostgreSQL.

> **ST hook:** *guardrail observability pairing* (Observability §5). Items below convert `⚠️ gap` → `✓` in the snapshot. Guardrails: G-REL-01, G-DB-08, I15. Docs: [Observability](./methodology/observability.md).

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **Backup/restore metrics + `last_verified_backup`** | NOT_STARTED | Export backup attempt/success/failure/duration/size + last verified restore | P1 / S — **methodology gap gate** |
| **Timeout/rollback counters** | NOT_STARTED | Query-timeout, lock-timeout, connection-failure, rollback event counters | P1 / S |
| **Structured request logs** | NOT_STARTED | Configurable JSON log sinks | P1 / M |
| **OpenTelemetry tracing** | NOT_STARTED | Request → auth → collection → SQL path; OTLP export | P1 / L |
| **Health/readiness endpoints** | PARTIAL | Add `/api/ready` with DB connectivity checks (liveness ≠ readiness) | P1 / S |
| **Default Grafana dashboards** | PARTIAL | Pre-built dashboards for key signals | P1 / M |
| **Alertmanager rules** | NOT_STARTED | Pool saturation, p99, outbox lag, error rate, backup failures | P2 / S |

### Theme H — Testing & benchmark harness (P1)

Performance work is measured, not assumed.

> **ST hook:** *evidence infrastructure*. Failure experiments A/B (methodology gate) depend on these. Guard rails: G-REL-02, G-DB-08, G-API-05.

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **Load/soak harness** | NOT_STARTED | k6/vegeta scenarios: auth, CRUD, relations+expand, realtime fanout, concurrent writes, multi-instance, pool saturation | P1 / L (bounded Go harness first for experiment B) |
| **CI PostgreSQL matrix** | NOT_STARTED | Test across PG 16/17 | P2 / M |
| **Coverage measurement & gate** | NOT_STARTED | Measure coverage in CI first; gate later (`coverage.out` fossil is deleted — not evidence) | P2 / S |
| **Dashboard UI tests** | NOT_STARTED | Automate `ui/src/audits/` + fork-specific pages | P2 / M |
| **Fuzz/property tests** | NOT_STARTED | Filter → SQL translation (`tools/search` + `tools/dbutils`) — riskiest fork-diverged code | P2 / L |

### Theme I — Performance at scale (P2, evidence-gated)

Deferred items from the performance audit. Ship only when benchmarks prove value.

| ID | Item | Decision rule | Priority |
|---|---|---|---|
| **PERF-I04** | Partition `_logs` (bulk DELETE → dead-tuple churn) | Ship when benchmark shows operational benefit | P2 |
| **PERF-I02** | GIN/expression indexes on JSONB multi-value columns | Validate with representative workloads | P1/P2 |
| **PERF-I05** | Audit `DEFAULT` partition never pruned (W-09) | Fix when growth evidence warrants | P2 |
| **PERF-I08** | `CREATE INDEX CONCURRENTLY` path for large tables | Provide safer large-table index workflow | P1/P2 |
| **PERF-I07** | Random 15-char TEXT PKs (B-tree bloat) | Preserve compatibility; do not change without compelling evidence | — |

### Theme J — Developer experience & ecosystem (P1/P2)

| Item | Priority |
|---|---|
| Webhooks with retries, idempotency, dead-letter handling | P1 |
| Migration authoring UX (preview, dry-run, failure diagnostics) | P1 |
| SDK compatibility documentation (JS, Dart) | P1 |
| Starter templates ("Deploy PG-BASE + Next.js/Vue/Svelte in 5 minutes") | P1 |
| Background-job primitive (deliberately small, not a workflow platform) | P2 |
| Docker Compose deployment templates (before Kubernetes/Helm) | P2 |
| JS VM (`jsvm`) hooks parity + docs | P2 |

---

## 7. Sequencing, gate, and exit criteria

### Phase 0 — Ship the wedge (P0)

**Sprint 0a — Trust & trial path:** **DONE** (2026-09-11). Security policy, CI hygiene, one-command provision, agent quickstart, positioning doc, reproducible releases, upstream drift decision.

> **Methodology gate (definition of "system thinking is fully applied") — Sprint 0b is HELD until all pass:**
>
> - [x] **E1** Framework doc split into the 7 system docs ([Platform Design](./methodology/platform-design.md), [Quality Guardrails](./methodology/guardrails.md), [Failure Analysis](./methodology/failure-modes.md), [Observability](./methodology/observability.md), [Evaluation Checklists](./methodology/checklists.md), [Disaster Recovery](../deployment/disaster-recovery.md), [Upstream Tracking](./upstream.md)); framework file removed; no duplicate docs remain (`.feature-plan/` retired 2026-09-12)
> - [x] **E2** `AGENTS.md` is the operational contract (loop, DoD, failure classification, authority) — all agents read it (single entry point; W-10 classified B/C before fix)
> - [x] **E3** PR template (10-step DoD) + issue templates (A–E classification, DRR) live (`.github/pull_request_template.md`, `.github/ISSUE_TEMPLATE/`, `evidence/records/DRR-template.md`)
> - [x] **E4** Mechanical guard rails in CI: secret scan (G-SEC-01), timeout/invariant tests, API-compat baseline (`.github/workflows/security-scan.yaml`, `core/coldboot_migration_test.go`, `apis/api_surface_baseline_test.go`)
> - [x] **E5** Failure experiments A–E executed once each, artifacts + verdict in `evidence/experiments/` (L2 first runs, L3 on claims) and reflected in [Failure Analysis §4](./methodology/failure-modes.md#4-controlled-failure-experiments) — all five PASS (L3), 2026-09-12; full suite + `-race` green on the same tree
> - [x] **E6** Roadmap/checklists restructured on the two-axis prioritization; `feature-plan/` retired
> - [x] **E7** Decision records & learning log live (B1 = 24 h window, G-AI-03; DRRs in `evidence/records/`) — template live; no B1/B2 decisions taken yet; `evidence/learnings.md` populated
> - [x] **E8** Docs nav/sidebar include "Engineering methodology"; docs build + lychee + markdownlint clean

**Sprint 0b — Differentiator (HELD until E1–E8 pass → **unheld 2026-09-12**, starts next):**

4. pgvector minimal: native field type + vector indexes + top-K similarity API (hybrid search deferred to P1).
5. MCP server v1 (`pgbase mcp`, read-mostly toolset) + deterministic CLI contract.

**Sprint 0c — Migration:**

6. Migration CLI (`pgbase migrate --from pocketbase`) + compatibility matrix + playbook.

**Phase 0 exit criteria:** a developer/agent goes zero → running PG-BASE backend with a vector collection in under 10 minutes; a PocketBase app migrates with minimal code changes.

### Phase 1 — Credibility (P0/P1)

1. CVE/advisory triage SLA + versioning/upgrade policy (completed in Sprint 0a except versioning policy).
2. Compatibility test suite in CI.
3. Published positioning & comparison doc (**done**).
4. Security regression tests (extend to compat).
5. Agent-facing quick start guide (**done** in Sprint 0a).

**Exit criteria:** release cadence established; public page explains where PG-BASE wins with reproducible evidence.

### Phase 2 — Scale out (P1)

1. Distributed rate limiter. 2. Realtime outbox hardening (phantom-delete, replay-on-restart) — then evaluate default-on. 3. DB-backed invalidation. 4. Multi-instance topology documented + load-tested (S3 requirement). 5. PgBouncer + pool-sizing guidance published. 6. Load/soak harness running.

**Exit criteria:** `LB → N instances → Postgres` is a tested, documented deployment pattern.

### Phase 3 — Operational maturity (P1)

1. OpenTelemetry tracing + structured logs. 2. S3 backup offload + PITR/WAL guidance. 3. Restore drills with recorded RPO/RTO. 4. Grafana dashboards + Alertmanager rules. 5. Health/readiness endpoints.

**Exit criteria:** operators can diagnose, recover, and restore with documented procedures.

### Phase 4 — Ecosystem (P1/P2)

1. Webhooks with retries + DLQ. 2. Starter templates. 3. SDK compatibility docs. 4. Background-job primitive (evidence-gated). 5. PostgreSQL extension guidance (pg_trgm, PostGIS). 6. GIN JSONB optimization (benchmark-gated).

**Exit criteria:** starter templates exist; first community contributions from outside maintainers.

---

## 8. Quality gates

Every roadmap item must carry the system-thinking header (property → invariant → guardrails → observe → test → evidence → checklist) in its PR — the [10-step DoD](./methodology/checklists.md#7-definition-of-done-per-non-trivial-change). Release evaluation gates live in [Evaluation Checklists](./methodology/checklists.md).

**Promotion rule:** promote a P2 item to P1 only when at least one of: production-deployment evidence, reproducible benchmark data, repeated community demand, or a concrete security/reliability/compatibility requirement.

**Performance rule:** every performance change ships with a reproducible before/after result (L3).

**Compatibility rule:** every compatibility change includes the corresponding PocketBase behavior and a regression test.

**Failure rule:** on any failure, classify A–E ([Failure Analysis §3](./methodology/failure-modes.md#3-failure-classification-a-e)) before changing code; class B–E implies the corresponding system layer updates in the same PR.

---

## 9. Risk register

| Risk | Impact | Mitigation |
|---|---|---|
| Upstream PocketBase security CVE | High | Advisory tracking + triage SLA + security backports (FORK_STRATEGY §5) |
| Fork drift (upstream outpaces backport) | High | Drift decision documented; overlay re-eval triggers (FORK_STRATEGY §7) |
| Compatibility drift | High | Compatibility suite (Tasks 18/24), matrix, release regression gates |
| Migration data loss or corruption | High | Dry-run, verification, backups, idempotent steps, rollback docs (Theme C) |
| Multi-instance inconsistency | High | Reproducible HA environment, failure tests, explicit semantics (Theme E) |
| Cross-host settings/collections staleness (W-04) | High | DB-backed invalidation; document shared-data-dir limit until shipped |
| Misdirected vulnerability reports | Medium | Resolved in Sprint 0a (forked SECURITY.md) |
| Methodology gate slippage (maintainer bandwidth) | Medium | Doc restructure is Level-A agent work; gate scoped to E1–E8; experiments A/B scoped with bounded harness |
| Supabase absorbs the wedge | High | Self-hosters: stack weight + cost; agents: single-binary deployability |
| Performance work without user value | Medium | Benchmark gate + P2 promotion rule |
| Excessive product scope | High | Preserve the strategic wedge; reject broad platform expansion |
| Maintainer burnout | High | Small milestones, agent-executed doc/template work, clear ownership |

---

## 10. Out of scope

- Re-adding SQLite as a supported backend.
- Breaking PocketBase REST API compatibility.
- Changing the random TEXT primary-key model without a compatibility-preserving design.
- Competing with Supabase/Appwrite on feature breadth.
- Building an edge/serverless runtime or a broad cloud control plane.
- Building a feature-heavy workflow or queue platform.
- Building a managed cloud before self-hosted adoption validates demand.
- Monetization plays before the product capability and community exist.

---

## 11. How to contribute

Open a discussion or issue referencing the relevant theme and item (e.g. `Theme E / Realtime outbox hardening`). Bug reports use the failure-classification template; B-level changes use the decision-request template. Requirements:

- Compatibility changes include the corresponding PocketBase behavior and a regression test.
- Performance changes include a reproducible benchmark or load-test scenario.
- Scale-out changes include a multi-instance test plan.
- Migration changes include verification and failure-handling behavior.
- Priority changes explain the evidence (two-axis scoring, §5.1).

See [`contributing.md`](./contributing.md), [`developing.md`](./developing.md), and [Production Runbook](../deployment/production.md); system model in [Platform Design](./methodology/platform-design.md); safety constraints in [Quality Guardrails](./methodology/guardrails.md).

---

*This document supersedes all prior roadmap documents. The previous versions remain in git history for reference.*

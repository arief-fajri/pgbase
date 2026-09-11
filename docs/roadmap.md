# PG-BASE — Product & Development Roadmap

<DocMeta audience="All" status="living document" verified="v0.5.2 (923e860)" />

> Single source of truth · Baseline: v0.5.2 (`923e860`) · Last review: 2026-09-08 (revised after independent code audit)
> All prior roadmap documents (roadmap-v2, v3, v4, v5, consolidation) are superseded by this file.

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

Verified against the actual codebase on 2026-09-08. Every claim below links to real code.

### Fully implemented

| Feature | Evidence |
|---|---|
| **PostgreSQL port** (pgx v5, PgSQLDialect, forked dbx) | `core/db_connect.go:88` opens via pgx; `tools/dbutils/pgsql.go` (163 lines); `third_party/dbx/builder_pgsql.go` (105 lines) |
| **Security hardening** (SSRF guard, download caps, checksum-verified updates, SQL identifier quoting, pinned CI/digests, `sslmode=prefer` default, encryption-key guidance) | `tools/security/httputil.go:18-69` (SafeHTTPClient + ValidateDialAddress); `tools/archive/extract.go:16` (PB_BACKUP_MAX_EXTRACT_BYTES); `core/db_connect.go:79-86` (SSLMode warning) |
| **Audit trails** (`_audits` write + `_audit_reads` read, month-partitioned, batched async writer, retention cron) | `core/audit_hooks.go` (539 lines); `core/audit_writer.go` (171 lines); `migrations/1787237001_audits_init.go`; `apis/audits_test.go` (337 lines) |
| **Native pg_dump/pg_restore backups** + legacy SQLite round-trip import/export + offline CLI | `core/backup_pg_export.go` (157 lines); `core/backup_pg_import.go` (93 lines); `core/backup_sqlite_import.go` (991 lines); `cmd/backup.go` (109 lines) |
| **Cross-instance realtime outbox** (DB-backed NOTIFY/LISTEN, cursor pagination, origin stamping, hourly TTL cleanup) | `core/realtime_outbox.go` (325 lines); `migrations/1787237104-106`; `apis/realtime_outbox_listener_test.go` + `realtime_outbox_integration_test.go` |
| **Prometheus metrics** (`/metrics` on dedicated listener, HTTP histograms, DB pool stats, realtime collectors) | `apis/metrics.go` (344 lines); `apis/metrics_test.go` (347 lines); `go.mod` prometheus client v1.23.2 |
| **Connection pool tuning** (DataMaxOpenConns/IdleConns, env vars, PgBouncer compat, role-level timeouts) | `core/base.go:43-47` (defaults); `core/db_connect.go:64-165` (env vars + statement/lock timeouts + `default_query_exec_mode` for PgBouncer) |
| **CI/CD** (GitHub Actions, GoReleaser, PostgreSQL test container, 32-bit cross-compile check, actions pinned by SHA) | `.github/workflows/release.yaml` (135 lines); `.goreleaser.yaml` |
| **Dashboard UI** (PocketBase-fork with audit trails UI, backup format selector, PG-BASE branding) | `ui/src/audits/` (3 files); `ui/src/settings/audit/pageAuditSettings.js`; `ui/package.json` name: "PG Base Superuser UI" |
| **PostgreSQL-native migrations** (16 migrations + 3 migration-level tests: JSONB, TIMESTAMPTZ, partitions, pgcrypto, functional indexes, autovacuum tuning, outbox, heartbeats) | `migrations/` — all PostgreSQL-specific syntax |
| **Test suite** (217 test files, database-per-test isolation via `CREATE DATABASE ... TEMPLATE`; 7 core test files use a shared DB with manual cleanup; ~1:1 test-to-prod LOC ratio) | `tests/app.go` (1,149 lines); CI runs `go test ./... -count=1 -p 4` |
| **Production deployment** (docker-compose.prod.yml with Caddy auto-HTTPS, monitoring stack under deploy/, production runbook) | `docker-compose.prod.yml`; `deploy/`; `docs/production.md` |

### Partially implemented

| Feature | What exists | What's missing |
|---|---|---|
| **PocketBase → PG-BASE migration** | Legacy SQLite backup import (`core/backup_sqlite_import.go`, 991 lines) handles v0.22/v0.23 schema, field conversion, auth migration | No dedicated `pgbase migrate` CLI command; no standalone migration tool; no compatibility test suite; no compatibility matrix; no dry-run/verification workflow |
| **S3 backup offload + retention** | First-class S3 destination wiring (`core/base.go:722` `NewBackupsFilesystem` → `filesystem.NewS3`: bucket/region/endpoint/access-key/secret/path-style); auto-backup cron `__pbAutoBackup__` (`Backups.Cron`) with count-based retention (`Backups.CronMaxKeep`, default 3 — `core/settings_model.go:171`); restore reads directly from S3 (`core/base_backup.go:280`) | No PITR/WAL archiving; no documented S3 lifecycle/retention policy beyond `CronMaxKeep`; no automated restore drills |

### Not started

| Feature | Status |
|---|---|
| **pgvector support** | Zero code, zero dependency, zero migration. Purely aspirational. |
| **MCP server** | Zero references in codebase. No MCP tooling exists. |
| **Distributed rate limiter** | In-memory per-instance only (`apis/middlewares_rate_limit.go:162-189`, `store.Store` + map). N instances = N× the configured limit. |
| **Multi-instance load/soak testing harness** | No k6, vegeta, or equivalent scenarios exist. |
| **OpenTelemetry tracing** | No OTel dependency or instrumentation. |
| **Health/readiness endpoints** | `/api/health` returns a static 200 with no DB ping (`apis/health.go:18`); no readiness/liveness distinction for orchestrators. |
| **Webhooks with retries/idempotency/DLQ** | Not implemented. |
| **PITR/WAL archiving guidance** | Not documented. |

### Baseline health snapshot

- Build + full test suite: **green** (CI verified)
- `govulncheck ./...` and `npm audit`: **clean** at v0.5.2 (manual one-off; not scheduled in CI)
- ~70k LOC non-test Go (excluding UI), 217 test files, ~1:1 test-to-prod LOC ratio

### Known weaknesses (verified in code, 2026-09-08 audit)

Real, observable in the current tree; each is either missing from the themes below or needs explicit scope in them:

- **Phantom deletes**: outbox delete events are published pre-commit and outside the record transaction (`apis/realtime.go:485`); a failed delete still notifies peers. Create/update self-heal on the receiver; delete cannot.
- **Outbox delivery is at-most-once**: listener cursor is in-memory and seeded to the tail at startup — events published while an instance is down are never replayed (`apis/realtime_outbox_listener.go:61-70`).
- **Cron guard fails open**: on a lock-acquisition DB error the job runs unguarded on every instance (`core/cron_guard.go:45-67`); a lock-connection death mid-run can double-fire.
- **Cache invalidation is file-based, not DB-based**: cross-instance settings/collections reload relies on fsnotify sentinel files in a shared `pb_data/.notify` (`core/notify_watcher.go`); cross-host deployments serve stale settings/schemas until restart — security-relevant.
- **Local file storage breaks multi-instance**: default uploads land on one instance's disk (`core/base.go:701-715`); S3 is required for any multi-instance topology.
- **Test env fallbacks in production code**: `PGTEST_PASSWORD`/`PGTEST_SSLMODE` consulted in `core/realtime_outbox.go:282-289`.
- **Audit read-trail drops under burst**: 500-slot buffer drops events when full (`core/audit_writer.go:54-61`) — deliberate, but must be documented to operators.
- **Audit DEFAULT partition is never pruned** (`core/audit_hooks.go:494-539`): rows landing in DEFAULT are retained forever (PERF-I05).
- **Restore success gating is weak**: a single `count(_collections) >= 1` check passes partial restores (`core/backup_pg_import.go:78-84`).
- **`.github/SECURITY.md` is verbatim upstream**: vulnerability reports are misdirected to `support@pocketbase.io`.
- **CI gaps**: no `-race`, no golangci-lint in CI, no scheduled vuln scans; `lychee-action@v2` is unpinned (breaks the project's own SHA-pinning policy) and the link check is effectively disabled (`failIfEmpty: false` + all URLs ignored).
- **UI has zero automated tests**, including the fork-specific Audits pages (`ui/src/audits/`).
- **Minor debt**: ignored `json.Marshal` error (`core/realtime_outbox.go:120`); vestigial `processed_at` column + dead partial index (migration `1787237106`); dead `tests/db.go:SetupTestDB`; hardcoded DSN in `tools/search/filter_test.go:25`; stale `coverage.out` fossil (do not cite as evidence).

---

## 2. Market context

### Macro trends (validated data, 2026)

| Signal | Data | Implication |
|---|---|---|
| BaaS market | $31.36B (2025) → $114.05B (2035), ~13.78% CAGR | Large, growing, structurally healthy |
| PostgreSQL adoption | 80% of startup primary databases (up from 76%) | Postgres is the default relational engine |
| Supabase growth | ~$170M ARR (May 2026), +221% YoY, database launches +600% YoY | "Open Postgres BaaS" demand validated at scale |
| Agent-driven demand | 60%+ of new Supabase databases created by AI tools (Claude Code, Codex, Bolt, Lovable) | The greenfield acquisition channel of 2026 |
| Vector search | Postgres + pgvector = default vector stack; hybrid FTS + vector (RRF) = standard RAG pattern | Vector is table stakes, not a feature |
| Self-hosting economics | Self-hosted Supabase ≈ dozen containers, $15–50/mo; PocketBase = single binary, ~$5/mo | Cost & simplicity gap is defensible |

> Sourcing note: market figures above are secondary-source estimates that cannot be verified from this repository. Before external publication, attach citations or soften the wording — unsourced numbers in a public doc damage credibility.

### The PocketBase ceiling

- PocketBase official FAQ: **"PocketBase uses embedded SQLite... there are no plans for supporting other databases."**
- Full backward compatibility not guaranteed before v1.0; docs say NOT recommended for production-critical apps before v1.0.
- Every PocketBase user who needs concurrent writes, horizontal scale, or PostgreSQL extensions **must leave PocketBase**. Today they mostly leave to Supabase.
- PG-BASE's wedge: **keep the PocketBase DX, replace SQLite with PostgreSQL**.

---

## 3. Competitive positioning

| Project | Approach | Threat level |
|---|---|---|
| **pocketbase/pocketbase** | Upstream; SQLite-only; 60K+ community | Partner + constraint (API compatibility source) |
| **zhenruyan/postgrebase** | Fork: PG + MySQL, Redis cache, multi-instance; established since Oct 2023 | High — most mature rival fork; broader feature surface |
| **statewright/pg-pocketbase** | Build-tag overlay (not a fork); tracks upstream with minimal diff; LISTEN/NOTIFY, advisory locks | Strategic — structurally avoids fork drift; the architecture competitors may converge on |
| **arief-fajri/pgbase** | Hard fork at ~v0.39.11-lineage; deep security/perf hardening; audit trails; native pg_dump | — |

**Critical reading:** the "PocketBase + Postgres" category has no leader. Demand is proven (PocketBase's SQLite ceiling is real) but fragmented across several micro-projects. Whoever ships **trust + capability + a clear "why us"** wins the category. PG-BASE's hardening is a head start on trust; the capability gaps (pgvector, migration CLI, agent tooling) are the blockers.

---

## 4. SWOT

### Strengths

- Production-grade PostgreSQL port (pgx v5, JSONB, `timestamptz`, `pgcrypto`, introspection).
- Security hardening that rival forks lack: SSRF guard, download caps, checksum-verified updates, pinned CI/digests, encryption-key guidance.
- Native `pg_dump`/`pg_restore` backups + offline CLI + legacy SQLite import.
- Audit trails (`_audits`/`_audit_reads`) — no rival fork offers this.
- Cross-instance realtime outbox, Prometheus metrics, production runbook, monitoring stack.
- 217 test files with database-per-test isolation.

### Weaknesses

- Fork drift risk: upstream at v0.40.x; PG-BASE tracks ~v0.39.11-lineage. Every upstream release adds maintenance cost.
- No demo, no one-click trial path — a new evaluator cannot "feel" the product in under 10 minutes.
- No AI/vector story despite running on PostgreSQL (pgvector is technically possible today, but zero code exists).
- No dedicated migration tool from stock PocketBase (only legacy archive import).
- No MCP server or agent-facing tooling.

### Opportunities

- Agent economy: AI coding agents now create the majority of new backends. A single binary + Postgres is the ideal shape for agent deployment.
- pgvector + hybrid search as a native differentiator over stock PocketBase (which cannot ever add it on SQLite).
- Self-hosting simplicity gap vs Supabase stack heaviness.
- Category leadership: the "PocketBase → PostgreSQL" niche has no incumbent.

### Threats

- Build-tag overlays (no-drift architecture) become the default approach; hard forks lose.
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
6. **Evidence over speculation.** Performance and prioritization claims require reproducible benchmarks or production evidence.
7. **Production trust before feature breadth.** Security, upgrades, backups, restore, and observability outrank experimental features.
8. **Compatibility before optimization.** A faster incompatible implementation does not satisfy the product promise.
9. **Greenfield capability drives adoption.** Features that help new users/agents pick PG-BASE outrank deeper operational polish. Ops depth retains users; capability acquires them.

---

## 6. Roadmap themes

### Theme A — AI-native capability (P0)

pgvector and vector search are table stakes in 2026. Stock PocketBase cannot add this on SQLite. This is the single strongest differentiator available.

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **pgvector field type** | NOT_STARTED | `vector` column type with dimension + distance metric options, exposed through PocketBase-style schema/API | P0 / L |
| **Vector indexes** | NOT_STARTED | IVFFlat / HNSW support with documented trade-offs (accuracy vs speed vs memory) | P0 / M |
| **Similarity query API** | NOT_STARTED | `sort=embedding.<metric>` with filter + top-K through the documented REST API | P0 / M |
| **Hybrid search** | NOT_STARTED | FTS + vector reciprocal rank fusion — the dominant RAG pattern in 2026 | P1 / M (requires a new sort grammar in `tools/search`; demoted from P0 to keep Phase 0 shippable) |
| **Embedding-on-write hook** | NOT_STARTED | Webhook or jsvm hook to auto-populate vectors on record create/update | P1 / M |
| **Full-text search guidance** | NOT_STARTED | PostgreSQL FTS examples and integration patterns | P1 / S |

**Definition of success:** a developer creates a `vector` field, inserts records, and queries top-K similarity through the documented API with no custom SQL.

### Theme B — Agent deployability (P0)

AI coding agents are the #1 source of new backend demand. PG-BASE's single-binary + Postgres architecture is ideal for agent deployment — but zero agent tooling exists.

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **MCP server** | NOT_STARTED | `pgbase mcp` — v1 scope: read-mostly tools (collections, schema, records, auth); backup/admin tools follow in P2 | P0 / L |
| **Deterministic CLI contract** | PARTIAL | `serve`, `migrate`, `backup`, `restore`, `superuser` — all idempotent + flag-documented for agent consumption | P0 / M |
| **One-command provision** | NOT_STARTED | `curl … \| sh` or `docker run` that boots PGBase + Postgres and returns a working URL | P0 / M |
| **Agent-facing quick start** | NOT_STARTED | "Give your agent the following prompt + this template" guide | P0 / S |
| **Positioning & comparison doc** | NOT_STARTED | Honest comparison vs PocketBase, Supabase, postgrebase, pg-pocketbase | P0 / S |

**Definition of success:** an agent (or human) can go from zero to a running PG-BASE + Postgres backend in under 10 minutes.

### Theme C — PocketBase migration (P0)

The most obvious acquisition channel: existing PocketBase users who need PostgreSQL. The backup/restore path exists but is not a first-class migration experience.

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **Migration CLI** | PARTIAL | `pgbase migrate --from pocketbase` with validation, dry-run, progress, safe failure handling. The conversion engine already exists and is tested (`core/backup_sqlite_import.go`, 991 lines); the remaining work is a CLI wrapper + verification report — estimated M, not L. | P0 / M |
| **Compatibility test suite** | NOT_STARTED | Automated tests against representative PocketBase REST API, auth, filters, relations, expand, realtime, files, SDK behavior | P0 / L |
| **Compatibility matrix** | NOT_STARTED | Document supported PocketBase behavior, JS/Dart SDK versions, PostgreSQL versions, known differences | P0 / M |
| **Migration verification** | NOT_STARTED | Pre/post counts, relation checks, auth checks, checksums, compatibility report | P0 / M |
| **Migration playbook** | NOT_STARTED | Minimal-code migration guide with examples and failure recovery | P0 / S |

**Definition of success:** a real PocketBase application can migrate to PG-BASE with minimal code changes through a repeatable, verifiable process.

### Theme D — Upstream trust & releases (P0)

The fork model is the #1 existential technical risk. Every upstream release adds drift. This must be addressed deliberately.

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **Upstream drift decision** | NOT_STARTED | Document the deliberate choice: stay hard fork vs adopt build-tag overlay vs formal upstream-sync contract. Include maintenance-cost estimate. | P0 / M |
| **CVE/advisory triage SLA** | AD_HOC | Monitor PocketBase releases + Go/npm advisories; triage and backport applicable fixes within documented SLA | P0 / M |
| **Reproducible releases** | PARTIAL | GoReleaser configured (`.goreleaser.yaml`); CI triggers on tags; but no published GitHub releases yet | P0 / S |
| **Versioning & upgrade policy** | NOT_STARTED | Define versioning scheme, breaking-change policy, security-backport policy | P0 / S |
| **Security policy & reporting** | NOT_STARTED | `.github/SECURITY.md` is verbatim upstream and misdirects vulnerability reports to PocketBase — fork it with PG-BASE's own contact channel | P0 / S |
| **CI hygiene quick wins** | PARTIAL | Add `-race` and golangci-lint to CI; schedule govulncheck + npm audit; SHA-pin `lychee-action` (docs.yaml); fix the effectively-disabled link-check config | P0 / S |
| **Dependency update cadence** | AD_HOC | Scheduled `govulncheck` + `npm audit` in CI; Dependabot/Renovate for pinned digests | P1 / M |
| **Security regression tests** | NOT_STARTED | Retain tests for SSRF, file/download limits, SQL identifiers, auth, permissions, encryption-key behavior | P1 / M |

### Theme E — Horizontal scalability (P1)

Several subsystems assume one process. This blocks honest multi-instance claims.

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **Realtime outbox** | IMPLEMENTED (opt-in) | Harden known weaknesses first: phantom-delete (publish pre-commit, outside the record txn, `apis/realtime.go:485`), at-most-once + no replay across instance restarts, publish silently skipped when local broadcast fails. Then load-test + document HA; default-on only after | P1 / M |
| **Distributed rate limiter** | NOT_STARTED | Shared PostgreSQL/Redis strategy with documented semantics (currently in-memory per-instance, `apis/middlewares_rate_limit.go:162-189`; N instances = N× the limit) | P1 / M |
| **Cache invalidation** | PARTIAL (in-memory per-instance; cross-instance via fsnotify sentinel files in a shared `pb_data/.notify` — NOT database NOTIFY) | Replace with a DB-backed (LISTEN) invalidation channel; today cross-host deployments serve stale settings/collection rules until restart — security-relevant | P1 / M |
| **Cron coordination** | IMPLEMENTED (advisory locks; heartbeat is a warning-only presence guard, not coordination) | Test + document the fail-open edge (lock-acquisition error → job runs unguarded on every instance, `core/cron_guard.go:45-67`) and lock-connection death mid-run; document single-fire semantics honestly | P1 / S |
| **Multi-instance file storage** | PARTIAL (local `<DataDir>/storage` by default; S3 opt-in via `Settings().S3`, `core/base.go:701-715`) | Local storage breaks multi-instance (other instances 404 on files). Document the S3 requirement for any N-instance topology; consider a startup warning when multiple heartbeats + local storage are detected | P1 / S |
| **Token revocation** | NOT_STARTED | Optional shared immediate-revocation mechanism for cross-instance logout | P2 / M |
| **Connection pooling** | IMPLEMENTED | Document validated PgBouncer transaction-mode setup end-to-end | P1 / S |
| **Multi-instance test environment** | NOT_STARTED | Reproducible N-instance integration and load environment (current tests simulate peers by inserting outbox rows directly, not by running N real instances) | P1 / M |

**Definition of success:** `LB → N PG-BASE instances → PostgreSQL` is a tested, documented deployment pattern that survives instance restarts without duplicated jobs, broken realtime, or inconsistent security behavior.

### Theme F — Backup & disaster recovery (P1)

Native `pg_dump` is solid for small/medium; production PostgreSQL needs a broader recovery story.

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **PITR/WAL archiving guidance** | NOT_STARTED | Document using pgBackRest or WAL-G for large databases | P1 / M |
| **S3-compatible backup offload** | IMPLEMENTED | Wiring + count-based retention (`CronMaxKeep`) already exist (`core/base.go:722`, `core/base_backup.go:280`); remaining work = documented lifecycle/retention policy + verification of large-object offload | P2 / S |
| **Restore drills** | NOT_STARTED | Automated or documented restore verification with integrity checks | P1 / M |
| **Restore verification** | PARTIAL (restore success currently gated by a single `count(_collections) >= 1` check, `core/backup_pg_import.go:78-84`) | Post-restore integrity checks: per-table counts vs source, expected schema present, settings sanity — a partial restore must fail loudly | P1 / S |
| **RPO/RTO guidance** | NOT_STARTED | Define expectations for dump-based vs WAL/PITR workflows | P1 / S |
| **Upgrade runbook** | NOT_STARTED | PostgreSQL major-version upgrades and PG-BASE application upgrades | P1 / M |

### Theme G — Observability & operability (P1)

Metrics exist; production users need enough visibility to diagnose across instances and PostgreSQL.

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **Structured request logs** | NOT_STARTED | Configurable log sinks (JSON, etc.) | P1 / M |
| **OpenTelemetry tracing** | NOT_STARTED | Request → auth → collection → SQL path; OTLP export | P1 / L |
| **Health/readiness endpoints** | PARTIAL | `/api/health` returns a static 200 with no dependency checks (`apis/health.go:18`); add `/api/ready` with DB connectivity checks for orchestrators | P1 / S |
| **Default Grafana dashboards** | PARTIAL | `deploy/` has Prometheus + Grafana compose; need pre-built dashboards for key signals | P1 / M |
| **Alertmanager rules** | NOT_STARTED | Pool saturation, p99 latency, outbox lag, error rate, backup failures | P2 / S |

### Theme H — Testing & benchmark harness (P1)

Performance work should be measured, not assumed. Currently no perf regression coverage.

| Item | Status | Target | Priority / Effort |
|---|---|---|---|
| **Load/soak harness** | NOT_STARTED | Reproducible k6/vegeta scenarios: auth, CRUD, relations+expand, realtime fanout, concurrent writes, multi-instance, pool saturation | P1 / L |
| **CI PostgreSQL matrix** | NOT_STARTED | Test across supported PostgreSQL major versions (16/17) | P2 / M |
| **Coverage measurement & gate** | NOT_STARTED | `coverage.out` is a stale fossil (predates audit trails; 156/312 files, zero `apis/` files) and nothing is measured in CI. Add coverage measurement to CI first; gate later | P2 / S |
| **Dashboard UI tests** | NOT_STARTED | Zero automated tests for the UI, including fork-specific Audits pages (`ui/src/audits/`) | P2 / M |
| **Fuzz/property tests** | NOT_STARTED | Filter → SQL translation layer (`tools/search` + `tools/dbutils`) — the riskiest fork-diverged code | P2 / L |

### Theme I — Performance at scale (P2, evidence-gated)

Deferred items from the performance audit. All validated as real; ship only when benchmarks prove value.

| ID | Item | Decision rule | Priority |
|---|---|---|---|
| **PERF-I04** | Partition `_logs` (plain table; 6-hourly bulk DELETE → dead-tuple churn) | Ship when benchmark shows meaningful operational benefit | P2 |
| **PERF-I02** | GIN/expression indexes on JSONB multi-value columns | Validate with representative query workloads | P1/P2 |
| **PERF-I05** | Audit `DEFAULT` partition holds current-month rows, never pruned | Fix when partition growth evidence warrants it | P2 |
| **PERF-I08** | `CREATE INDEX CONCURRENTLY` path for large tables | Provide safer large-table index workflow | P1/P2 |
| **PERF-I07** | Random 15-char TEXT primary keys (random B-tree insert, bloat) | Preserve compatibility; do not change without compelling evidence | — |

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

## 7. Sequencing & exit criteria

### Phase 0 — Ship the wedge (P0)

Split into three sprints so trust wins land early and maintainer load stays bounded:

**Sprint 0a — Trust & trial path (small, days-scale):**

1. Security policy fix (`.github/SECURITY.md`) + CI hygiene quick wins (`-race`, lint, scheduled scans, SHA-pin lychee).
2. One-command provision (Docker / curl script) + agent-facing quick start + positioning doc.
3. Reproducible releases (GoReleaser, published GitHub releases) + upstream drift decision (documented, deliberate).

**Sprint 0b — Differentiator:**
4. pgvector minimal: native field type + vector indexes + top-K similarity query API (hybrid search deferred to P1).
5. MCP server v1 (`pgbase mcp`, read-mostly toolset) + deterministic CLI contract.

**Sprint 0c — Migration:**
6. Migration CLI (`pgbase migrate --from pocketbase`) + compatibility matrix (conversion engine already exists; this is wrapper + verification work).

**Exit criteria:** a developer (or agent) can go from zero to a running PG-BASE backend with a vector collection in under 10 minutes. A PocketBase app can migrate with minimal code changes.

### Phase 1 — Credibility (P0/P1)

1. CVE/advisory triage SLA + versioning/upgrade policy.
2. Compatibility test suite in CI.
3. Published positioning & comparison doc.
4. Security regression tests.
5. Agent-facing quick start guide.

**Exit criteria:** release cadence established; a public page explains where PG-BASE wins and proves it with reproducible evidence.

### Phase 2 — Scale out (P1)

1. Distributed rate limiter (shared Postgres/Redis store).
2. Realtime outbox hardening (phantom-delete fix, replay-on-restart) — only then evaluate default-on.
3. DB-backed settings/collections invalidation (replace the fsnotify sentinel mechanism).
4. Multi-instance topology documented + load-tested (incl. the S3 file-storage requirement).
5. PgBouncer and pool-sizing guidance published.
6. Load/soak harness built and running.

**Exit criteria:** `LB → N instances → Postgres` is a tested, documented deployment pattern.

### Phase 3 — Operational maturity (P1)

1. OpenTelemetry tracing + structured logs.
2. S3 backup offload + PITR/WAL guidance.
3. Restore drills with recorded RPO/RTO.
4. Default Grafana dashboards + Alertmanager rules.
5. Health/readiness endpoints for orchestrators.

**Exit criteria:** operators can diagnose, recover, and restore with documented procedures.

### Phase 4 — Ecosystem (P1/P2)

1. Webhooks with retries + DLQ.
2. Starter templates for common stacks.
3. SDK compatibility documentation.
4. Background-job primitive (if evidence supports it).
5. PostgreSQL extension guidance (pg_trgm, PostGIS).
6. GIN JSONB optimization (benchmark-gated).

**Exit criteria:** starter templates exist; first community contributions from outside maintainers.

---

## 8. Quality gates

Every roadmap item should have:

- A test or verification strategy.
- A definition of done.
- Documentation impact.
- A rollback or failure-handling plan where applicable.

**Promotion rule:** promote a P2 item to P1 only when at least one of these is available:

1. Evidence from a real production deployment.
2. Reproducible benchmark data.
3. Repeated community requests with a clear use case.
4. A concrete security, reliability, or compatibility requirement.

**Performance rule:** every performance change ships with a reproducible before/after result.

**Compatibility rule:** every compatibility change includes the corresponding PocketBase behavior and a regression test.

---

## 9. Risk register

| Risk | Impact | Mitigation |
|---|---|---|
| Upstream PocketBase security CVE | High | Formal advisory tracking, triage SLA, security backports |
| Fork drift (upstream moves faster than backport) | High | Drift decision documented; consider overlay architecture |
| Compatibility drift | High | Compatibility suite, matrix, upstream review, release regression gates |
| Migration data loss or corruption | High | Dry-run, verification, backups, idempotent steps, rollback documentation |
| Multi-instance inconsistency | High | Reproducible HA environment, failure tests, explicit semantics |
| Cross-host settings/collections staleness (cache invalidation requires a shared `pb_data`) | High | DB-backed invalidation channel (Theme E); document the shared-data-dir limitation until it ships |
| Misdirected vulnerability reports (`SECURITY.md` points upstream) | Medium | Fork the policy with a PG-BASE contact channel (Theme D, P0/S) |
| Phase 0 overload vs maintainer capacity | High | Sprint split with early trust wins; hybrid search and MCP admin tools deferred |
| Supabase absorbs the wedge | High | Win self-hosters on stack weight + cost; win agents on single-binary deployability |
| Performance work without user value | Medium | Benchmark gate and P2 promotion rule |
| Excessive product scope | High | Preserve the strategic wedge; reject broad platform expansion |
| Maintainer burnout | High | Small milestones, clear ownership, delayed optional work |

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

Open a discussion or issue referencing the relevant theme and item:

- `Theme A / pgvector field type`
- `Theme B / MCP server`
- `Theme C / Migration CLI`
- `Theme D / Upstream drift decision`
- `Theme E / Distributed rate limiter`

Requirements:

- Compatibility changes include the corresponding PocketBase behavior and a regression test.
- Performance changes include a reproducible benchmark or load-test scenario.
- Scale-out changes include a multi-instance test plan.
- Migration changes include verification and failure-handling behavior.
- Priority changes explain the evidence behind the change.

See [`contributing.md`](./contributing.md), [`developing.md`](./developing.md), and [`production.md`](./production.md) for implementation and operational guidance.

---

*This document supersedes all prior roadmap documents. The previous versions remain in git history for reference.*

# Production Readiness Matrix

<DocMeta audience="Contributor" status="living document" verified="v0.6.0" />

> Per-capability production status — implementation, tests, failure behavior, and risk — verified against the tree, not against the roadmap's own claims. This is the evidence base for phase-exit decisions and the v1.0.0 gate (`evidence/records/DRR-0005.md`).
>
> First audit: 2026-09-27 (v0.6.0, DRR-0005 session). Re-audited at every phase exit — a phase is done when its rows are green with evidence, not when its code merges.

## Legend

| Status | Meaning |
|---|---|
| ✅ | Implemented and tested |
| 🟡 | Implemented, insufficiently validated |
| 🟠 | Partial / architectural gap |
| 🔴 | Missing |
| ⚪ | Deferred by decision — not required by current positioning |

## A. Core platform

| Capability | Implementation / tests (anchor) | Production risk | Status |
|---|---|---|---|
| PostgreSQL engine, dual pool, schema ownership | `core/db_connect.go`, full DB-backed suite; dual-pool ceiling 90 conns | — | ✅ |
| Auth / collections / rules | full auth suite incl. per-endpoint rate-limit scenarios | — | ✅ |
| File storage (local) | layered traversal protection: record-file match → name regex → `escapeKey` (`apis/file.go`, `core/field_file.go`, fileblob) — all tested | — | ✅ |
| File storage (S3) | vendored SigV4 client (`tools/filesystem/internal/s3blob`); **tests are stub-based only** | S3 in production has never been exercised against a real provider; local + multi-instance = W-05 | 🟠 |
| Realtime (SSE + outbox) | opt-in outbox; delete published pre-commit (**W-01**), no replay across restart (**W-02**), slow consumers dropped not backpressured, no outbox lag metrics | lost/spurious events under failure; flows doc marks it experimental before default-on | 🟠 |

## B. Trust (Phase 0 — complete)

| Capability | Implementation / tests | Production risk | Status |
|---|---|---|---|
| Security baseline (SSRF, caps, quoting) | post-connect dial check (anti DNS-rebinding), 32 MiB download caps, identifier quoting — all tested (`tools/security`, `tools/filesystem`) | no allowlist mode; `NewUnsafeFileFromURL` is a standing footgun | ✅ |
| Secret scanning | gitleaks full history + CI workflow | — | ✅ |
| Audit trail | write trail + redaction tested; **partition cron untested**; DEFAULT partition never pruned (**W-09**); read trail drop-on-full accepted (**W-07**) | unbounded DEFAULT-partition growth; the pruning path is never exercised by tests | 🟡 |
| Backup + offline restore CLI | round-trip tests + archive-derived gate (W-08) + env parity (W-14) | — | ✅ |
| Restore verification | pre-restore TOC gate, stderr classification, completeness, semantic sanity (`core/backup_pg_verify.go`) | the SQLite import path has no equivalent verification gate (tracked in Phase 1) | ✅ |
| Restore drills | `last_verified_backup` loop shipped, gauge + `_params` persistence | executed twice so far; cadence starts now | 🟡 |
| Readiness / liveness | `/api/ready` data-pool + `_collections` probe; split tested (`apis/ready_test.go`) | aux pool and S3 not probed (minor) | ✅ |
| Metrics | HTTP, pool, tx, backup, realtime clients/drops (`apis/metrics.go`) | outbox lag / broadcast latency land with Phase 3 outbox hardening | ✅ |
| Pool + layered timeouts | role-level statement/lock timeouts, fail-fast connect, recycle — tested + experiment B | — | ✅ |
| Graceful shutdown | real-SIGTERM test + `pg_stat_activity` backends-gone assert (`apis/serve_shutdown_test.go`) | drain window is 1 s vs 5 m WriteTimeout — in-flight uploads are cut; no slow-client drain test | 🟡 |
| Upgrade/rollback policy | documented procedure + PG 16/17 observed by CI (`pg-matrix`); rollback = restore with the old binary | "exercised in real use" is a pending DRR-0004 criterion | ✅ |
| Install / provision | checksum-verified installer, idempotent quickstart (`deploy/`) | **no CI leg executes either script** | 🟡 |
| PgBouncer | documented exec-mode + role-timeout notes (`core/db_connect.go`, developing.md) | never tested against a real pooler | 🟡 |

## C. Migration & Compatibility (Phase 1)

| Item | Findings | Status |
|---|---|---|
| SQLite import engine | e2e tested with an authentic fixture (`core/backup_sqlite_import_test.go`); **no dry-run, no verification report, no dedicated CLI**; mid-import failure leaves committed schema with rolled-back records | 🟠 |
| Compatibility suite | [Checklists §3](./methodology/checklists.md): 0/12 rows ticked | 🔴 |
| Migration verification report | not started | 🔴 |
| Migration failure-recovery playbook | per-stage transactions only | 🔴 |
| Endpoint reference | not started | 🔴 |

## D. Production Validation (Phase 2 — workstream, gates v1.0.0)

| Item | Findings | Status |
|---|---|---|
| Load harness / capacity benchmark | zero Go benchmarks, zero load tooling; experiments A–E are one-shot manual records | 🔴 |
| SLO / capacity envelope | RPO/RTO qualitative only; latency measured without targets; capacity = pool math | 🔴 |
| Failure-injection automation | A–E exist as manual L3 records; no network partition, storage failure, or reconnect-storm scenario; not automated | 🟡 |
| Burn-in / soak | DRR-0004 time criterion only; no dogfood workload or runbook | 🔴 |
| Concurrency correctness | Checklists §1 unticked; **W-16 open** — inherited data race, Level B decision pending | 🔴 |
| Request-log retention | DELETE-based cleanup shipped (6 h cron, MaxDays gate, autovacuum tuning); DELETE bloat + 6 h window remain; no partition path | 🟡 |
| Audit DEFAULT pruning (W-09) | never pruned | 🔴 |
| GIN JSONB filter indexes | multi-value filters full-scan today | 🔴 |

## E. HA & Scale (Phase 3 — in the v1.0.0 gate per DRR-0005)

| Item | Findings | Status |
|---|---|---|
| Multi-instance topology test | heartbeat detection only; `LB → N instances → PG` never tested (incl. PgBouncer leg) | 🔴 |
| Realtime outbox hardening | replay-on-restart (W-02) and the default-on path not built; W-01 fix is ordered into Phase 1 | 🔴 |
| Distributed rate limiter | in-memory map per process — N instances = N× the limit | 🔴 |
| Cross-instance cache invalidation | fs-sentinel works only on a shared filesystem (**W-04**, security-relevant staleness); DB LISTEN is the target, not the current mechanism | 🟠 |
| Cron guard (W-03) | fails open on lock-acquisition error → duplicated jobs on every instance | 🟠 |
| Multi-instance files (W-05) | S3 path exists but is stub-tested; local-storage multi-heartbeat warning unimplemented | 🟠 |
| Replica / HA guidance | not started; boundary per §3 of the [roadmap](./roadmap.md): guidance, not a failover manager | 🔴 |

## F. Deferred by decision

| Item | Decision |
|---|---|
| pgvector / full-text / hybrid search | ⚪ Phase 4 — stability-first ordering (DRR-0005); the quickstart pre-creates the extension so the stack stays valid |
| MCP server | ⚪ Phase 5 — CLI + `docs/agents.md` are the agent interface today |

## Re-audit cadence

- Every phase exit re-audits the rows that phase claims to close; the PR flips statuses with evidence links (L0–L3 per `evidence/README.md`).
- A row may not be flipped to ✅ without a test or experiment record that observes the claimed behavior.
- New capabilities enter the matrix in the same PR that ships them — a capability that is not in the matrix is not claimed.

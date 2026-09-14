# Failure Modes

<DocMeta audience="Operator" status="living document" verified="v0.5.2 (923e860)" />

> Codified failure scenarios and the system's expected behavior, plus the **A–E failure classification** that gates every debugging session.
>
> **The critical rule — classify before you fix:** do not jump from a failing test to a code change. Classify the failure below first. Fixing the wrong layer is how guard-rails stay porous.
>
> Guaranteed behavior claims below are supported by code + the evidence in `evidence/experiments/` (linked per row where a failure experiment has been executed).

## 1. Failure-mode table

| Failure | Expected system behavior | Anchor / evidence |
|---|---|---|
| PG unavailable | Requests fail predictably and do not hang indefinitely | `connect_timeout=10s` fail-fast dial; experiment A |
| PG restarts | App can recover/reconnect | `ConnMaxLifetime 30m` recycles stale sockets; pgx reconnect on next acquire |
| Connection pool exhausted | Requests bounded by timeout | `SetMaxOpenConns` 80/10; wait counters observable; experiment B |
| Query too slow | Query/request terminates per timeout policy | query timeout 30s + `statement_timeout 60s` |
| Lock contention | Lock wait is bounded | `lock_timeout 30s`; experiment C |
| Migration fails | Startup/upgrade does not silently continue in invalid state | advisory-xact-lock + transactional Up(); experiment D |
| Realtime disconnects | Client reconnects without corrupting primary data | SSE idle timeout 5 min, client reconnect; EPHEMERAL only |
| Backup fails | Failure is visible; backup not treated as valid | backup API error + metrics (add `last_verified_backup`) |
| Restore fails | Recovery reports failure clearly | restore error path; **weak gate W-08** |
| App crashes | DB remains consistent | transactional writes, WAL |
| Process restarts | Existing data remains usable | data in PG, not in-memory; settings reload from DB |
| Invalid input | Request rejected without partial persistence | validation layer before tx commit |

## 2. Known weaknesses (verified in code, 2026-09-08 audit)

These are real, observable gaps. Each carries a classification (A–E) so the fix lands in the right layer.

| ID | Weakness | Location | Class | Impact | Status |
|---|---|---|---|---|---|
| W-01 | Realtime outbox delete published pre-commit → failed delete still notifies peers | `apis/realtime.go:485` | **B — design failure** | peers notified of state that never happened | roadmap Task 32 |
| W-02 | Outbox delivery is at-most-once; no replay across instance restarts | `apis/realtime_outbox_listener.go:61-70` | **B — design failure** | events lost during downtime | Task 33 |
| W-03 | Cron guard fails open on lock-acquisition error → job runs unguarded on every instance | `core/cron_guard.go:45-67` | **C — missing guard rail** | duplicated jobs under DB errors | Task 38 |
| W-04 | Cross-instance cache invalidation is fsnotify-file-based, not DB-backed → stale settings/schemas | `core/notify_watcher.go` | **B — design failure** | security-relevant staleness | Task 34 |
| W-05 | Local file storage breaks multi-instance topologies | `core/base.go:701-715` | **B — design failure** | 404s on files stored on another instance | Task 39 |
| W-06 | Test env fallbacks (`PGTEST_PASSWORD`/`PGTEST_SSLMODE`) reachable in production code | `core/realtime_outbox.go:282-289` | **A — implementation failure** | test config leaks into prod paths | fix + P0 |
| W-07 | Audit read-trail drops under burst (500-slot buffer) — deliberate, undocumented | `core/audit_writer.go:54-61` | **E — incorrect acceptance criteria** | silent data loss under load | document + Task 45 |
| W-08 | Restore success gating too weak (`count(_collections) >= 1` passes partial restores) | `core/backup_pg_import.go:78-84` | **E — incorrect acceptance criteria** | partial restore treated as success | Task 45 |
| W-09 | Audit `DEFAULT` partition never pruned → retained forever | `core/audit_hooks.go:494-539` | **D — missing observability / C** | unbounded growth | PERF-I05 |
| W-10 | Cold boot on an empty DB failed: `RunAllMigrations` spawned the aux transaction (advisory-locked) and the data transaction on **different pool connections**; their interleaved catalog DDL (`CREATE EXTENSION pgcrypto` vs `CREATE TABLE _logs`) contended past the 30 s lock_timeout → `pgcrypto extension error`. Reproduced 3/3 with **default pool settings** on a fresh DB (2026-09-12, gate E5 harness). Fresh installs (`docker-compose.prod.yml`, quickstart) were **non-deterministic** | **Fixed (2026-09-12)** — `core/migrations_runner.go:runMigrationTx` now runs the whole migration set in a **single transaction/connection** (`RunInSingleTx`, `core/db_tx.go`); regression test `core/coldboot_migration_test.go` (Bootstrap + RunAllMigrations on a truly empty DB = 0.5 s, was 30 s lock timeout); CLI `migrate up` on empty DB verified; evidence record `evidence/experiments/EXPERIMENT-D-20260912-155751.md` (blocked → bounded failure → recover, deterministic fingerprint) | **B/C — design + missing guard rail** | fixed | guard rail **G-DB-10** added (migration DDL applies on a single connection) |

## 3. Failure classification (A–E)

When a test fails or a runbook step misbehaves, classify the failure **before** touching code:

| Type | Pattern | Response |
|---|---|---|
| **A — Implementation failure** | Design ✓, guard rail ✓, implementation ✗ | Fix implementation |
| **B — Design failure** | Implementation ✓, but design was insufficient | Change the platform design (PLATFORM.md / invariant) |
| **C — Missing guard rail** | System entered a dangerous state that was not prevented | Create a new guard rail (GUARDRAILS.md) |
| **D — Missing observability** | System failed but operators could not determine why | Add measurement (OBSERVABILITY.md) |
| **E — Incorrect acceptance criteria** | Checklist passed but real-world behavior was unsafe | Redesign evaluation criteria (CHECKLISTS.md) |

**Release-blocking rule:** any failure classified B/C/D/E implies a non-code layer of the system must change (design doc, guard rail, measurement, or checklist) in the same PR that fixes the symptom. A "fix" that only changes code while the classification is B+C+D is not a fix.

## 4. Controlled failure experiments

Observability must be proven through controlled failure, not assumed. The five experiments and their expected results:

| Exp | Scenario | Expected result | Verdict |
|---|---|---|---|
| **A** | PG outage → requests → recovery | no infinite hangs; bounded errors; deterministic recovery | **PASS** — record `evidence/experiments/EXPERIMENT-A-20260912-161420.md` (real container stop; bounded 400 responses; app alive; recovery to 200) |
| **B** | Pool saturation under concurrent load | predictable degradation near the boundary; wait/error observable | **PASS** — record `evidence/experiments/EXPERIMENT-B-20260912-154253.md` (32/32 bounded responses; app alive) |
| **C** | Hold + conflicting lock → lock wait/timeout | bounded lock wait; correct timeout response | **PASS** — record `evidence/experiments/EXPERIMENT-C-20260912-155209.md` (bounded 30 s lock timeout under ACCESS EXCLUSIVE; recovery to 200; app alive) |
| **D** | Migration fails → restart → verify state | reproducible DB state; recovery procedure works | **PASS** — record `evidence/experiments/EXPERIMENT-D-20260912-155751.md` (blocked migrate fails cleanly in ~30 s; recovered + fresh fingerprints identical; 15 internal tables) |
| **E** | Backup → destroy → restore → verify | valid schema + data + auth after restore | **PASS** — record `evidence/experiments/EXPERIMENT-E-20260912-160628.md` (backup→destroy→restore round-trip; _collections/data/superuser intact) |

Gate: **before roadmap Phase 0 (Sprint 0b) unblocks, experiments A–E must be executed once** with L2–L3 verification, verdicts recorded, and this table's `Verdict` column filled in.

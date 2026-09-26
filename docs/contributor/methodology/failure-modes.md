# Failure Analysis

<DocMeta audience="Contributor" status="stable" verified="v0.5.4" />

> Codified failure scenarios and the system's expected behavior, plus the **A–E failure classification** that gates every debugging session.
>
> **The critical rule — classify before you fix:** do not jump from a failing test to a code change. Classify the failure below first. Fixing the wrong layer is how guardrails stay porous.
>
> Guaranteed behavior claims below are supported by code and the evidence in `evidence/experiments/` (linked per row where a failure experiment has been executed).

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
| Backup fails | Failure is visible; backup not treated as valid | backup API error + `pgbase_backup_failure_total` (`last_verified_backup`: Phase 0 restore drills) |
| Restore fails | Recovery reports failure clearly | restore error path — archive-derived verification gate (`core/backup_pg_verify.go`, W-08 closed) |
| App crashes | DB remains consistent | transactional writes, WAL |
| Process restarts | Existing data remains usable | data in PG, not in-memory; settings reload from DB |
| Invalid input | Request rejected without partial persistence | validation layer before tx commit |

## 2. Known weaknesses (verified in code, 2026-09-08 audit)

These are real, observable gaps. Each carries a classification (A–E) so the fix lands in the right layer.

| ID | Weakness | Location | Class | Impact | Status |
|---|---|---|---|---|---|
| W-01 | Realtime outbox delete published pre-commit → failed delete still notifies peers | `apis/realtime.go:485` | **B — design failure** | peers notified of state that never happened | [Phase 4](../roadmap.md) / realtime outbox — publish after commit |
| W-02 | Outbox delivery is at-most-once; no replay across instance restarts | `apis/realtime_outbox_listener.go:61-70` | **B — design failure** | events lost during downtime | [Phase 4](../roadmap.md) / realtime outbox — replay on restart |
| W-03 | Cron guard fails open on lock-acquisition error → job runs unguarded on every instance | `core/cron_guard.go:45-67` | **C — missing guard rail** | duplicated jobs under DB errors | [Phase 4](../roadmap.md) / load harness |
| W-04 | Cross-instance cache invalidation is fsnotify-file-based, not DB-backed → stale settings/schemas | `core/notify_watcher.go` | **B — design failure** | security-relevant staleness | [Phase 4](../roadmap.md) / cache invalidation |
| W-05 | Local file storage breaks multi-instance topologies | `core/base.go:737-754` | **B — design failure** | 404s on files stored on another instance | [Phase 4](../roadmap.md) / file storage |
| W-06 | Test env fallbacks (`PGTEST_PASSWORD`/`PGTEST_SSLMODE`) reachable in production code | `core/realtime_outbox.go` `realtimeOutboxListenerConfig` | **A — implementation failure** | test config leaks into prod paths | **Fixed (2026-09-25)** — fallback removed; listener credentials come only from the `PB_POSTGRES_*`-resolved config. Regression tests `core/realtime_outbox_test.go` (`TestRealtimeOutboxListenerConfigIgnoresTestEnv`) + `core/hard_rules_test.go` (static `PGTEST_` scan, hard rule 5) |
| W-07 | Audit read-trail drops under burst (500-slot buffer) — deliberate, warning-logged | `core/audit_writer.go:54-61` | **E — incorrect acceptance criteria** | silent data loss under load | closed — acceptance defined in [audit-design.md](../../architecture/audit-design.md) |
| W-08 | Restore success gating too weak (`count(_collections) >= 1` passes partial restores) | `core/backup_pg_import.go` `verifyRestoredDatabase` | **E — incorrect acceptance criteria** | partial restore treated as success | **Fixed (2026-09-26)** — gate is now archive-derived (`core/backup_pg_verify.go`): pre-restore TOC validation (a corrupt archive is rejected BEFORE the destructive restore), stderr classification with a narrow benign allowlist, TOC table/index completeness (missing index = post-data did not finish), and semantic sanity (`_collections`, `_params` settings row, superuser password). Regression tests `core/backup_pg_verify_test.go` + `core/backup_pg_scenario_test.go`; drill re-run `evidence/experiments/EXPERIMENT-E-20260926-011658.md` |
| W-09 | Audit `DEFAULT` partition never pruned → retained forever | `core/audit_hooks.go:494-539` | **D — missing observability / C** | unbounded growth | [Phase 4](../roadmap.md) / audit DEFAULT retention |
| W-10 | Cold boot on an empty DB failed: `RunAllMigrations` spawned the aux transaction (advisory-locked) and the data transaction on **different pool connections**; their interleaved catalog DDL (`CREATE EXTENSION pgcrypto` vs `CREATE TABLE _logs`) contended past the 30 s lock_timeout → `pgcrypto extension error`. Reproduced 3/3 with **default pool settings** on a fresh DB (2026-09-12, gate E5 harness). Fresh installs (`docker-compose.prod.yml`, quickstart) were **non-deterministic** | **Fixed (2026-09-12)** — `core/migrations_runner.go:runMigrationTx` now runs the whole migration set in a **single transaction/connection** (`RunInSingleTx`, `core/db_tx.go`); regression test `core/coldboot_migration_test.go` (Bootstrap + RunAllMigrations on a truly empty DB = 0.5 s, was 30 s lock timeout); CLI `migrate up` on empty DB verified; evidence record `evidence/experiments/EXPERIMENT-D-20260912-155751.md` (blocked → bounded failure → recover, deterministic fingerprint) | **B/C — design + missing guard rail** | fixed | guard rail **G-DB-10** added (migration DDL applies on a single connection) |
| W-11 | Listener host/port override query always failed at parse analysis (`COALESCE(inet_server_addr(), '')` coerces the empty literal to `inet` → `invalid input syntax for type inet: ""`) and the error was swallowed by `err == nil` → the "override host/port from the live connection" branch was dead code; the doc comment promised behavior the code never had, and a custom `DBConnect` was silently not honoured for host/port | `core/realtime_outbox.go` `realtimeOutboxListenerConfig` | **A+B+D — implementation + design + missing observability** | dead code + silent failure; the override design itself was unimplementable — the server-side view of the connection (`inet_server_addr()`/`inet_server_port()`) is never a dialable client address in NAT/port-mapped topologies | **Fixed (2026-09-25, fixed-by-removal)** — `evidence/records/DRR-0001.md`: override branch removed; the listener dials stored-config host/port, aligned with the `pgConnInfo` backup-path precedent (`core/backup_pg_export.go`); user/dbname still follow the live connection; regression test `core/realtime_outbox_test.go` (`TestRealtimeOutboxListenerConfigIgnoresTestEnv`) asserts host/port equality with the stored config |
| W-12 | `instanceHeartbeatGuard.init` reused the previous goroutine's `stopCh`/`done` on re-bootstrap **without** the terminate chain (`Bootstrap → ResetBootstrapState → Bootstrap` — ResetBootstrapState never runs cleanup): two goroutines shared one `done` → double `close(done)` panic ("close of closed channel", recovered by `FireAndForget`) at terminate, plus the previous goroutine leaked (ticking forever against a reset app). The constructor pre-created the channels, so terminate-without-ever-bootstrap blocked forever on `<-done` | `core/instance_heartbeat.go` `init` | **A — implementation failure** | recovered panic per churn cycle + leaked goroutine; invisible because `go test` hides the output of passing packages (the bug shipped silently behind a green suite) | **Fixed (2026-09-26)** — state machine `running`: `stopCh`/`done` are non-nil exactly while a goroutine is live (the constructor no longer pre-creates them); `init` stops-and-drains the previous goroutine before spawning a fresh one (each goroutine closes only its own `done`); `cleanup` is guarded by `running` (terminate without bootstrap can no longer block). Regression test `core/instance_heartbeat_test.go` (`TestInstanceHeartbeatGuardBootstrapChurn`); release gate added in [checklists §6](./checklists.md) (suite `-v` output free of `RECOVERED FROM PANIC`, except the deliberate `tools/routine` recover drill); reproduction `go test ./core/ -run TestBootstrapStateConcurrentAccess -count=1 -v`: 4× recovered panic → 0 |

## 3. Failure classification (A–E)

When a test fails or a runbook step misbehaves, classify the failure **before** touching code:

| Type | Pattern | Response |
|---|---|---|
| **A — Implementation failure** | Design ✓, guardrail ✓, implementation ✗ | Fix implementation |
| **B — Design failure** | Implementation ✓, but design was insufficient | Change the platform design (Platform Design / invariant) |
| **C — Missing guardrail** | System entered a dangerous state that was not prevented | Create a new guardrail (Quality Guardrails) |
| **D — Missing observability** | System failed but operators could not determine why | Add measurement (Observability) |
| **E — Incorrect acceptance criteria** | Checklist passed but real-world behavior was unsafe | Redesign evaluation criteria (Evaluation Checklists) |

**Release-blocking rule:** any failure classified B/C/D/E implies a non-code layer of the system must change (design doc, guardrail, measurement, or checklist) in the same PR that fixes the symptom. A "fix" that only changes code while the classification is B+C+D is not a fix.

## 4. Controlled failure experiments

Observability must be proven through controlled failure, not assumed. The five experiments and their expected results:

| Exp | Scenario | Expected result | Verdict |
|---|---|---|---|
| **A** | PG outage → requests → recovery | no infinite hangs; bounded errors; deterministic recovery | **PASS** — record `evidence/experiments/EXPERIMENT-A-20260912-161420.md` (real container stop; bounded 400 responses; app alive; recovery to 200) |
| **B** | Pool saturation under concurrent load | predictable degradation near the boundary; wait/error observable | **PASS** — record `evidence/experiments/EXPERIMENT-B-20260912-154253.md` (32/32 bounded responses; app alive) |
| **C** | Hold + conflicting lock → lock wait/timeout | bounded lock wait; correct timeout response | **PASS** — record `evidence/experiments/EXPERIMENT-C-20260912-155209.md` (bounded 30 s lock timeout under ACCESS EXCLUSIVE; recovery to 200; app alive) |
| **D** | Migration fails → restart → verify state | reproducible DB state; recovery procedure works | **PASS** — record `evidence/experiments/EXPERIMENT-D-20260912-155751.md` (blocked migrate fails cleanly in ~30 s; recovered + fresh fingerprints identical; 15 internal tables) |
| **E** | Backup → destroy → restore → verify | valid schema + data + auth after restore | **PASS** — record `evidence/experiments/EXPERIMENT-E-20260912-160628.md` (backup→destroy→restore round-trip; _collections/data/superuser intact); re-run on the hardened W-08 gate 2026-09-26: `evidence/experiments/EXPERIMENT-E-20260926-011658.md` (no false alarm on a valid restore, L2); re-run with the `last_verified_backup` loop 2026-09-26: `evidence/experiments/EXPERIMENT-E-20260926-083213.md` (G-REL-04: timestamp persisted + gauge > 0 after boot, L2) |

Experiments A–E have been executed once (all PASS, L3). Records live in `evidence/experiments/`. Remaining trust work is [Phase 0](../roadmap.md).

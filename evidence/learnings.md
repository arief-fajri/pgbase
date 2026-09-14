# Learnings

> Cross-session learning log. The 10-step Definition of Done (step 10: *what did we learn?*)
> appends here so knowledge accumulates between AI sessions and training runs.
> Format per entry: date · area · what happened · what changed in the system model/docs.

## 2026-09-12 — Method 20209: System-thinking framework adoption

- Replaced the single framework document with the system doc set: PLATFORM / GUARDRAILS /
  FAILURE-MODES / OBSERVABILITY / CHECKLISTS / DISASTER-RECOVERY / UPSTREAM
  (`.feature-plan/PG-BASE-System-Thinking-Framework.md` retired after being split).
- Added the AI decision-authority layer (G-AI-01…08): Level A / B1 (24 h window) / B2
  (explicit confirm) — AI autonomy is bounded by the same guard rails as human contributors.
- Inaugurated `evidence/` with L0–L3 verification policy and experiment/DRR templates.
- Roadmap Sprint 0b is **held** until the methodology gate (experiments A–E executed once
  with L2–L3 verification) and the doc restructure land (E1–E8).

<!-- New entries go above this line. -->

## 2026-09-12 — Cold-boot migration race (W-10): classification + single-connection fix

- **Event:** experiment B (and CLI `migrate up`) failed on a truly empty DB with
  `pgcrypto extension error ... lock timeout` 3/3. `pg_locks`/`pg_stat_activity` showed
  TWO pool connections in flight: the aux transaction creating `_logs` (advisory-locked)
  and the data transaction running `CREATE EXTENSION pgcrypto` — on cold DBs the two
  streams interleave catalog DDL on ONE goroutine and deadlock until role-level
  `lock_timeout` (30 s) cancels the run. Warm DBs/template-cloned test DBs never trigger
  it, which is why the suite had been green.
- **Classified:** B (design: nested aux+data transactions) + C (missing guard rail). Fix
  therefore had to include non-code layers (FAILURE-MODES W-10, guard rail G-DB-10,
  invariant I17) in the same change.
- **Change in the system model/docs:** migrations now apply in a SINGLE transaction/
  connection — added `core/db_tx.go: RunInSingleTx/createTxAppSingleDB`, rewrote
  `migrations_runner.go: runMigrationTx` to use it; the advisory lock stays xact-scoped
  so `-p 4` parallel test processes still serialize. Regression test
  `core/coldboot_migration_test.go` (Bootstrap+RunAllMigrations on empty DB: 0.5 s).
- **Evidence lessons:** (1) test DBs seeded via `CREATE DATABASE ... TEMPLATE` mask any
  cold-start behavior — a cold-boot test must start from a genuinely empty catalog;
  (2) `pg_dump` v18 emits a random `\restrict` marker that breaks schema fingerprints —
  filter it out; (3) `kill $PID` on a `psql ... &` subshell orphans the real `psql` (run
  `pg_terminate_backend` instead); (4) health endpoints that only do `SELECT 1`/no SQL
  cannot exercise lock or outage paths — probes must read a real table; (5) CLI env must
  be passed UNQUOTED to `env`; (6) `docker compose -p` project name must match the stack
  (here `tests`, not the default the harness shipped with).
- **Experiments A–E (gate E5) executed once, all PASS (L3):**
  A `.../EXPERIMENT-A-20260912-161420.md`, B `...B-20260912-154253.md`,
  C `...C-20260912-155209.md`, D `...D-20260912-155751.md`, E `...E-20260912-160628.md`
  (FAILURE-MODES §4 verdict matrix filled).
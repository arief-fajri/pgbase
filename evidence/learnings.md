# Learnings

> Cross-session learning log. The 10-step Definition of Done (step 10: *what did we learn?*)
> appends here so knowledge accumulates between AI sessions and training runs.
> Format per entry: date · area · what happened · what changed in the system model/docs.

## 2026-09-25 — W-06 closed: test env fallback out of production code (PR-1)

- **Event:** Phase 0 production-path hygiene: removed the `PGTEST_PASSWORD`/`PGTEST_SSLMODE` fallback
  from `core/realtime_outbox.go` (`OpenRealtimeOutboxListener`); listener credentials now come only
  from the config resolved via `PB_POSTGRES_*` at init time.
- **Lesson 1 (evidence quality):** the first proposed regression test — "assert the listener DSN
  never contains `PGTEST_PASSWORD`" — was a tautology: `buildDSN` is a pure function of its config
  and never reads env, so the test could not detect a W-06 regression (L2 rule: the artifact must
  prove the claim). The real seam is the config assembly, now extracted as
  `realtimeOutboxListenerConfig` and tested with `t.Setenv` poisoning. A static scan
  (`core/hard_rules_test.go`) is the mechanical enforcement of hard rule 5 / G-DB-05 — and it
  immediately caught its own production comment mentioning the token, which is how enforcement
  should behave.
- **Lesson 2 (finding, NOT fixed here):** the listener's host/port override query
  `SELECT COALESCE(inet_server_addr(), '') ...` always fails (`invalid input syntax for type inet:
  ""` — the empty literal is cast to `inet` at parse time) and the error is swallowed by
  `err == nil`, so the "override host/port from the live connection" path is dead code. The
  listener silently keeps the stored config host/port. Side effect: Docker Desktop macOS works by
  accident (localhost:5433), while a custom `DBConnect` pointing elsewhere is not honoured for
  host/port despite the comment. Class A (wrong COALESCE typing), but a fix changes behaviour and
  reddens `apis/realtime_outbox_listener_test.go` on macOS (container IP `inet_server_addr()` is
  not routable from the host) — needs its own classified decision (candidate W-11 row + design
  choice: drop the override or cast `host(inet_server_addr())::text`).
- **Docs:** `failure-modes.md` W-06 → Fixed (anchor re-pointed from the deleted lines to
  `realtimeOutboxListenerConfig`); roadmap Phase 0 row deleted (shipped items are named in the
  "Most of this is done" sentence); G-DB-05 enforcement now names the static scan.

## 2026-09-25 — Docs: sprint IDs retired, roadmap is the only schedule

- **Event:** Live docs still cited Sprint 0b, numbered tasks, and PERF/Theme/PGB codes after the roadmap moved to phases.
- **What changed:** Citations now name a phase item, or the promise was deleted. OpenTelemetry, PITR/WAL as a PG-BASE feature, default Grafana/Alertmanager rules, and an in-process `CREATE INDEX CONCURRENTLY` path are in "what we will not build", not a backlog. Phase 0 gained the PostgreSQL 16/17 matrix (inside versioning). Phase 1 gained the endpoint reference. Phase 4 gained JSONB filter indexes, `_logs` retention, and audit `DEFAULT` partition pruning.
- **Lesson:** A gap that does not match positioning must be deleted, not relabeled "not scheduled". A historical learning line that still names a dead sprint reads as current status.
- **Follow-up:** Open technical debt is now a phase item, not "not a roadmap item". Phase 0 gained production-path hygiene (W-06), secret scanning (G-SEC-01), diagnostic metrics, and reliability tests. Phase 4 outbox and load harness absorbed realtime metrics and concurrency tests. DocMeta `verified` moved from v0.5.2 to v0.5.4 (`9af5929`). Line anchors that drifted after the v0.5.3 import-order fixes (`core/base.go`, realtime listener, TrustedProxy) were re-pointed; a range check is not a semantic check.

## 2026-09-12 — Method 20209: System-thinking framework adoption

- Replaced the single framework document with the system doc set: PLATFORM / GUARDRAILS /
  FAILURE-MODES / OBSERVABILITY / CHECKLISTS / DISASTER-RECOVERY / UPSTREAM
  (`.feature-plan/PG-BASE-System-Thinking-Framework.md` retired after being split).
- Added the AI decision-authority layer (G-AI-01…08): Level A / B1 (24 h window) / B2
  (explicit confirm) — AI autonomy is bounded by the same guard rails as human contributors.
- Inaugurated `evidence/` with L0–L3 verification policy and experiment/DRR templates.
- The pre-phase methodology gate held further roadmap work until experiments A–E
  were executed once with L2–L3 verification and the doc restructure landed (E1–E8).
  Sprint identifiers were retired on 2026-09-25; the roadmap is phase-based.

## 2026-09-17 — Guard rail: GHCR `:latest` gated on quality + publish event

- **Event:** v0.5.4 tag push triggered `release.yaml`; the `docker` job published
  `ghcr.io/arief-fajri/pgbase:latest` immediately, in parallel with the
  test/race/lint jobs. A user could `docker pull :latest` from an unvalidated
  build.
- **Classified:** C (missing guard rail). Design intent existed (draft release
  gate), but the Docker image tag lacked enforcement — system entered a
  forbidden state.
- **Fix:** (1) Added `needs: [goreleaser, race, lint]` to the `docker` job;
  (2) removed `:latest` from the tag-push step, keeping only the immutable
  `:vX.Y.Z` tag; (3) created `docker-latest.yaml` triggered by
  `release: [published]` that re-tags the validated image as `:latest`.
- **Guardrails added:** G-CI-01 (artifacts gated on quality jobs) and G-CI-02
  (`:latest` == last published release).
- **Documentation:** `releasing.md` §4 and `developing.md` §15 updated to
  reflect the two-workflow contract.
- **DRR:** `evidence/records/DRR-0000.md` (B2, confirmed by maintainer).
- **Lessons:** (1) A guard rail without enforcement is a suggestion, not a rail.
  The `needs:` keyword is the enforcement mechanism — without it, parallel
  jobs have no ordering guarantee. (2) `:latest` should always mean "last
  published validated release", not "last tag pushed". (3) Retagging by digest
  (pull → tag → push) is simpler and more deterministic than rebuilding on
  the publish event.

<!-- New entries go above this line. -->

## 2026-09-16 — Documentation restructure: public/internal separation

- **Event:** Complete documentation restructure to separate public-facing docs from internal contributor docs.
- **Change:** Moved 14 files to new locations, created 3 new files, updated 50+ cross-references.
  - Public docs remain at `docs/` root (index, getting-started, comparison, fork-deltas, collections, flows, reference)
  - Internal docs moved to `docs/contributor/` (developing, contributing, releasing, roadmap, upstream, methodology)
  - Deployment docs moved to `docs/deployment/` (production, single-host, disaster-recovery)
  - Architecture docs promoted to top-level sidebar (overview, backend-layers, audit-design)
  - Created getting-started.md, reference/api-overview.md, contributor/index.md
- **Tone adjustments:** Renamed "Guard Rails" → "Quality Guardrails", "Failure Modes" → "Failure Analysis", "Checklists" → "Evaluation Checklists". Softened language in methodology docs. Removed PGB-xxx internal codes from deployment docs.
- **Verification:** `npm run docs:build` success, `make docs-check` clean, `go build ./...` clean, 0 broken links.
- **Evidence lessons:** (1) Relative path updates in markdown are error-prone — systematic grep + batch edit is essential. (2) VitePress dead link checker catches broken refs at build time — valuable safety net. (3) File renames via `git mv` preserve history; content edits via `edit` tool are faster than full rewrites for targeted changes.

## 2026-09-16 — Realtime outbox listener shutdown race + custom model resolution

- **Event:** Both CI jobs (`goreleaser` and `race`) failed in `apis` package tests.
  - Race job: nil pointer dereference panic in `RealtimeOutboxEventsAfter` (realtime_outbox.go:180)
    — `app.NonconcurrentDB()` returned nil because `ResetBootstrapState()` nil-ed `dataDB`
    while the outbox listener goroutine was still executing `processPending()`.
  - Goreleaser job: `TestRealtimeRecordResolve/custom_model_struct` expected 3 events
    (create/update/delete) but only received 1 (create) — custom model broadcast skipped
    for update/delete because `FindCachedCollectionByNameOrId` couldn't find
    collections created after bootstrap.
- **Classified:** A (implementation failure) for both — design was correct (listener should
  stop gracefully; custom models should resolve), but implementation had bugs.
- **Root cause 1 (shutdown race):** `processPending()` had zero `stopCh` checks. The
  `Cleanup()` sequence (OnTerminate → stop() → close stopCh → ResetBootstrapState → nil
  dataDB) had a window where the goroutine was in `processPending()` calling
  `NonconcurrentDB()`. Under `-race` overhead, this window was wide enough to trigger.
- **Root cause 2 (custom model resolution):** `realtimeResolveRecord()` and
  `realtimeResolveRecordCollection()` used only `FindCachedCollectionByNameOrId()` which
  relies on the bootstrap-time cache. Collections created dynamically (via API or test
  `Save()`) weren't in the cache, causing silent nil-return and broadcast skip.
- **Change in system model/docs:** Added `stopCh` checks before and inside `processPending()`
  loop. Added `FindCollectionByNameOrId` (direct DB lookup) as fallback when cache misses.
  Increased test timeout from 250ms to 2s for CI stability.
- **Evidence lessons:** (1) Goroutine shutdown patterns must check the stop channel at every
  meaningful entry point, not just in select loops — a synchronous function called from the
  goroutine is also a critical check point. (2) `FindCachedCollectionByNameOrId` is NOT a
  superset of `FindCollectionByNameOrId` — it returns `sql.ErrNoRows` for post-bootstrap
  collections. Any code path that resolves collections for user-created models must fall back
  to a direct DB lookup. (3) `t.Setenv` is process-wide and can leak env vars to parallel
  tests — avoid relying on it for feature flags that affect background goroutines.
- **Classification note:** Both fixes are internal, reversible, guard-rail-safe, and
  contract-safe → Level A (decide and execute). No PLATFORM/GUARDRAILS/OBSERVABILITY
  doc changes needed.

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

## 2026-09-25 — Positioning: application backend, not a fork tracker

- Public identity is a self-hosted PostgreSQL application backend. PocketBase is the DX
  inspiration and an acquisition path, not a release-tracking target.
- Removed `FORK_STRATEGY.md`, `docs/fork-deltas.md`, and `docs/contributor/upstream.md`
  with no redirects. Caller-visible behavior now lives in `docs/reference/api-contract.md`.
- Roadmap is phases 0–5 (trust, migration, PostgreSQL advantage, agent interface, scale,
  ecosystem). Migration is Phase 1, not the product thesis.

## 2026-09-25 — Decision authority: B1 and B2 merged

- Level B1 (apply after 24 h of silence) is retired. Former B1 and B2 cases are one
  Level B: explicit human confirm. Silence is not approval.
- Level A is unchanged. Historical DRRs keep their original labels.
# Learnings

> Cross-session learning log. The 10-step Definition of Done (step 10: *what did we learn?*)
> appends here so knowledge accumulates between AI sessions and training runs.
> Format per entry: date · area · what happened · what changed in the system model/docs.

## 2026-09-26 — Phase 0 restore drills shipped: last_verified_backup loop (PR-6)

- **Event:** G-REL-04 closed end to end: a restore that passes the verification gate persists an
  RFC3339 UTC timestamp to `_params` (`last_verified_backup`), the boot reloads it into the in-memory
  mirror, and `/metrics` exposes `pgbase_backup_last_verified_timestamp_seconds` (gauge,
  **0 = never verified** — always emitted so `time() - metric > cadence` alerts even when no drill
  ever ran). The drill (`EXPERIMENT-E.sh`) proves the full loop: restore → row → boot → gauge.
- **Lesson 1 (drill accounting):** the verified restore writes a NEW `_params` row after `pg_restore`,
  so the drill's PRE/POST `_params` row-count equality broke by exactly one. Fixed by excluding the
  metadata row from the counts — a drill that feeds a signal must account for the signal's own writes.
- **Lesson 2 (drift repaid):** `disaster-recovery.md` §4 still described the restore gate as
  "count(_collections) >= 1 — known weak point" — stale since PR-4 (two PRs of drift). Caught only
  because PR-6 touched that doc anyway. Each PR must close the docs rows it affects in the same
  commit; the PR-4 lesson repeated.
- **Design kept honest:** the timestamp is written best-effort (a failed write warns but does not flip
  a successful restore into a failure — observability metadata must not own the restore verdict);
  rejected restores never write; an unparseable persisted value degrades to 0, never a boot error.

## 2026-09-26 — Phase 0 diagnostic metrics shipped (PR-5)

- **Event:** The G-REL-01 gap (timeout and rollback counters) closed: `pgbase_db_query_timeout_total` /
  `pgbase_db_lock_timeout_total` / `pgbase_db_tx_rollback_total` (label `db`), backup lifecycle counters
  (`pgbase_backup_attempts/success/failure_total`, `_duration_seconds`, `_last_size_bytes`), and
  `pgbase_http_requests_total{method,route,status}` next to the existing histogram.
- **Lesson 1 (honest boundaries):** "reconnect" from the roadmap target text is NOT countable at the
  `database/sql` layer — the driver exposes no reconnect events, and counting them would mean pgx tracer
  surgery on the production connect path. Skipped honestly: pool dynamics are already visible via
  `pgbase_db_wait_count_total`/`open_connections`; the outbox LISTEN reconnect belongs to Phase 4 realtime
  observability. The roadmap row text was edited rather than shipping a half-honest counter.
- **Lesson 2 (shallow-clone traps):** diagnostic counters MUST live behind a pointer field on BaseApp —
  `createTxApp` shallow-clones the app (`clone := *app`), so value-type counters would give every
  transaction clone its own invisible counters (same trap as bootstrapMu, documented there).
- **Lesson 3 (classification choke points):** error classification lives where the bound is applied:
  `queryTimeoutHook` (record reads, data) + the three model write-execute boundaries in `db.go` + the
  transaction boundaries in `db_tx.go`. Server-side events classify by SQLSTATE (`57014` statement
  timeout, `55P03` lock timeout — the `pgconn.PgError` pattern already existed in `validators/db.go`);
  40P01 deadlocks are deliberately uncounted (outside the guard-rail pairing). Raw builder queries are
  not classified — documented in the metric Help, not silently implied.
- **Follow-up:** PR-6 (restore drills + `last_verified_backup`) now has the backup-metrics groundwork;
  the metric lands there with G-REL-04.

## 2026-09-26 — W-08 closed: restore gate is now archive-derived (PR-4)

- **Event:** The old restore success gate was `count(_collections) >= 1` — a restore that dropped
  every other table still "succeeded". Replaced with an archive-derived gate
  (`core/backup_pg_verify.go`): pre-restore TOC validation (a corrupt archive is rejected BEFORE
  the destructive restore — the live DB survives), stderr classification with a narrow benign
  allowlist, TOC table/index completeness, and semantic sanity (`_collections`, `_params`
  settings row, superuser password).
- **Lesson 1 (design):** a manifest of row counts captured around pg_dump is racy — backups run
  concurrently with live traffic, so valid restores would false-alarm (alarm fatigue destroys
  trust in the gate). Expectations must be derived from the archive itself (`pg_restore --list`),
  which is race-free and version-agnostic (an old backup is only required to restore what it
  actually contains).
- **Lesson 2 (design):** state-only checks cannot distinguish "table restored" from "old table
  never dropped" (same names). The deterministic fingerprint of a restore that did not finish is
  the **post-data section**: indexes are created after ALL table data, so a missing TOC index is
  the completion marker.
- **Lesson 3 (benign noise inventory):** the strict stderr classifier immediately surfaced the
  real benign families the old swallow-everything behavior had been hiding on this machine:
  `unrecognized configuration parameter "transaction_timeout"` (pg_dump 18.3 client → PG 16.15
  server SET preamble) and `cannot drop inherited constraint "_audits_default_pkey" of relation
  "_audits_default"` (the --clean DROP of partition children's inherited pkeys). TOC parsing also
  has two-word types beyond TABLE DATA: `TABLE ATTACH` and `INDEX ATTACH` (partition attach
  steps, not objects). Each benign pattern is allowlisted narrowly, with the observed instance
  documented in the code.
- **Verified:** round-trip test green (benign allowlist validated against the routine
  in-place restore); corrupt-archive regression green (live data survives rejection); drill
  EXPERIMENT-E re-run PASS at L2 (no false alarm on a valid restore).

## 2026-09-26 — Phase 0 readiness: /api/ready shipped (PR-3, DRR-0002)

- **Event:** The readiness gap was not in the code but in two deployment artifacts: the compose-prod
  healthcheck and the quickstart wait loop both probed liveness-only `/api/health`, so a container
  counted "healthy" during a full database outage. Fixed by adding the missing endpoint
  (`apis/ready.go`, `GET /api/ready` 200/503) and switching both probes to it.
- **What changed:** one bounded probe query through the normal data pool
  (`SELECT (SELECT count(*) FROM "_collections")`, 5 s bound) — proves pool acquire + server answer
  + core schema readable in a single round trip; 503 carries a **static** message (driver errors
  can contain host/user details — logged server-side only, a security-relevant regression-tested
  decision); `/api/health` deliberately unchanged (liveness).
- **Lesson 1:** the "DB is down" test does not need a dead server: closing the app's data pool
  (`app.DB().(*dbx.DB).DB().Close()`) fails queries fast ("sql: database is closed") while the
  process keeps serving — exactly the state that distinguishes readiness (503) from liveness (200).
  `ResetBootstrapState` tolerates an already-closed pool, so the scenario cleanup stays intact.
- **Lesson 2:** `app.DB()` returns the `dbx.Builder` interface — reaching the `*sql.DB` handle
  needs a type assertion (`apis/metrics.go` `sqlDBFromBuilder` precedent), not a method call.
- **Follow-up:** PR-4 (restore verification hardening) is next; PR-7 reliability tests should assert
  `/api/ready` turns 503 during a query-timeout window (bounded probe) once those tests exist.

## 2026-09-25 — Phase 0 docs truth: secret scanning was shipped, not "Not started" (PR-2)

- **Event:** Pre-Phase-0 audit assumed secret scanning was unbuilt and planned to build it. The
  tree said otherwise: `.gitleaks.toml` + the `secret-scan` job (gitleaks v8.24.3, full history,
  PR/push/schedule/dispatch) have been in place since `b4106df`. The gap was in the *docs state*,
  not the code.
- **What changed:** roadmap §4 gained a Shipped inventory row (with anchors) and dropped the
  "Not started" Phase 0 row; G-SEC-01 flipped (gap)→✅ with the workflow as its enforcement;
  checklists §4 "No secrets committed" ticked, §6 annotation re-pointed (the release gate itself
  stays unticked — §6 is a per-release matrix, not a standing posture).
- **Lesson:** executing a phase plan against a stale roadmap builds work that already exists.
  Audit the tree before planning an item; the §4 inventory (what exists) and the §5 phase table
  (what remains) are different documents answering different questions — a shipped capability
  belongs in §4 with anchors, not in a phase table.
- **Follow-up:** the same drift risk exists for the remaining Phase 0 rows (readiness, restore
  verification, metrics…) — each PR must close its row in the same commit, or the roadmap lies
  again.

## 2026-09-25 — W-11 closed: listener host/port override removed (DRR-0001)

- **Event:** The outbox listener's host/port override query never worked:
  `COALESCE(inet_server_addr(), '')` coerces the empty literal to `inet` at parse
  analysis → the query always fails (`invalid input syntax for type inet: ""`) and the
  `err == nil` guard swallowed the error, so the override branch was dead code and the
  listener always dialed the stored-config host/port. Classified **A** (COALESCE
  typing) + **B** (the override design itself: `inet_server_addr()`/`inet_server_port()`
  report the server-side view of the connection, which is never a dialable client
  address in NAT/port-mapped topologies — Docker Desktop, managed PG, pgbouncer) +
  **D** (silent failure).
- **Decision (DRR-0001, Level B, confirmed by maintainer):** fixed-by-removal — the
  listener dials stored-config host/port, aligned with the `pgConnInfo` backup-path
  precedent (`core/backup_pg_export.go`); user/dbname still follow the live connection
  so a custom `DBConnect` is honoured for the database name. Fixing the cast was
  rejected: it would make the listener dial unroutable container-internal addresses
  (and redden the macOS e2e test) in exactly the topologies where a custom
  `DBConnect` matters.
- **Lesson 1 (design):** a dial target cannot be derived from the server's view of the
  connection — client and listener share one process/network namespace, and server
  introspection cannot recover mapped ports or routable addresses. Stored config is
  the only universal source for host/port; live-connection introspection is valid only
  for identity fields the server owns (database, user, schema).
- **Lesson 2 (evidence quality):** removing dead code still needs a regression test —
  `TestRealtimeOutboxListenerConfigIgnoresTestEnv` now asserts
  `cfg.Host/Port == app.dbConfig.Host/Port`, so the removal cannot be "fixed back"
  into an environment-dependent listener without a red test.
- **Docs:** failure-modes.md W-11 row added; the W-06 entry's Lesson 2 flipped to
  resolved.

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
- **Lesson 2 (finding, resolved 2026-09-25 — W-11 / DRR-0001):** the listener's host/port override query
  `SELECT COALESCE(inet_server_addr(), '') ...` always fails (`invalid input syntax for type inet:
  ""` — the empty literal is cast to `inet` at parse time) and the error is swallowed by
  `err == nil`, so the "override host/port from the live connection" path is dead code. The
  listener silently keeps the stored config host/port. Side effect: Docker Desktop macOS works by
  accident (localhost:5433), while a custom `DBConnect` pointing elsewhere is not honoured for
  host/port despite the comment. Class A (wrong COALESCE typing) + B (the override design itself:
  the server-side view is not a dialable client address in NAT/port-mapped topologies) + D
  (silent failure) — resolved by removal, see the W-11 entry above.
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
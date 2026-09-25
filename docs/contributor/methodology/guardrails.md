# Quality Guardrails

<DocMeta audience="Contributor" status="stable" verified="v0.5.4" />

> Guardrails are the constraints that prevent PG-BASE from entering unacceptable states.
>
> - A **test** answers: *did we observe the expected behavior?*
> - A **guardrail** answers: *what behavior is never allowed?*
>
> Each entry names its **enforcement mechanism** so the constraint is not a recommendation but a check. System model and invariants: [Platform Design](./platform-design.md). Failure classification: [Failure Analysis](./failure-modes.md).

## 0. How to read the enforcement column

- **code** — enforced by program logic (timeouts, bounds, guards).
- **config** — enforced by shipped defaults / env handling (secure defaults).
- **CI** — enforced by CI jobs; if marked `(gap)` the job must still be added.
- **process** — enforced by the definition-of-done / review workflow (`AGENTS.md`, PR/issue templates).
- **test** — covered by an existing automated test; `(gap)` means a test must still be written.

A guard rail is **live** only when its enforcement mechanism exists and is exercised. `(gap)` entries are tracked in the roadmap; a PR that must rely on a gapped guard rail should state the gap explicitly.

## 1. Data integrity (G-DATA)

```text
G-DATA-01  All multi-step writes that must be atomic must use an appropriate transaction boundary.
           Enforcement: code — app.Save / db_tx wrapper; test (transaction rollback tests) ✅
G-DATA-02  A failed transaction must not partially persist state.
           Enforcement: code — SAVEPOINT-guarded audit hooks; test — failed writes leave no partial rows ✅
G-DATA-03  Schema changes must be migration-controlled.
           Enforcement: code + config — collections live in PG tables synced via SyncRecordTableSchema;
           system DDL versioned in migrations/ ✅
G-DATA-04  Destructive migrations require explicit review.
           Enforcement: process — DoD step 4 (guard rails apply) + Level B confirm (G-AI-02) ✅
G-DATA-05  Migrations must be safe to execute during controlled deployment and must have a defined
            failure/recovery path.
            Enforcement: code — advisory-xact-lock serialized, transactional Up(); Failure Analysis
            experiment D defines recovery. Test: experiment D PASS
            (`evidence/experiments/EXPERIMENT-D-20260912-155751.md`) ✅
G-DATA-06  Application code must not rely on undefined PostgreSQL behavior.
           Enforcement: code review + PG 16/17 matrix (Phase 0 versioning, gap) ⚠️
```

## 2. PostgreSQL (G-DB)

```text
G-DB-01  Connection pools must have explicit maximum sizes.
         Enforcement: code — data 80 / aux 10 (SetMaxOpenConns); env override ✅
G-DB-02  Total possible application connections must be compatible with the PostgreSQL server's
         connection capacity.
         Enforcement: config — single-instance ceiling 90 < stock max_connections 100; sizing math below;
         multi-instance guidance in production.md ✅ (documented)
G-DB-03  Database operations must have bounded execution time.
         Enforcement: code — query timeout 30s, statement_timeout 60s, write deadline ✅
G-DB-04  Lock waits must be bounded.
         Enforcement: code — lock_timeout 30s (role level, PgBouncer-safe) ✅
G-DB-05  Database credentials must come from protected configuration.
         Enforcement: config — PB_POSTGRES_* env / secret store; .env gitignored; production code
         never reads the test-harness env prefix (static scan, core/hard_rules_test.go) ✅
G-DB-06  Production database traffic must use the intended TLS policy.
         Enforcement: code+config — sslmode=prefer default, non-loopback sslmode=disable warns;
         production requires require/verify-full ✅
G-DB-07  Database failures must not cause unbounded request blocking.
         Enforcement: code — connect_timeout 10s, bounded waits; experiment A PASS
         (`evidence/experiments/EXPERIMENT-A-20260912-161420.md`) ✅
G-DB-08  Connection pool exhaustion must be observable.
         Enforcement: code — pgbase_db_wait_count_total / wait_duration metrics; alert on
         rate(wait_count[5m]) > 0 ✅
G-DB-09  Migrations must be serialized across processes.
         Enforcement: code — pg_advisory_xact_lock on a fixed key (migrationsAdvisoryLockKey)
         spanning the whole migration transaction ✅
G-DB-10  Migration DDL must apply on a single connection.
          Enforcement: code — runMigrationTx uses App.RunInSingleTx (data + aux point at the same
          transaction/connection); regression test core/coldboot_migration_test.go; proven by
          Failure Analysis W-10 fix ✅
```

**Connection capacity math** (from PLATFORM P1):

```text
(Application instances × pool size) + Other PostgreSQL clients  <  PostgreSQL max_connections
```

Leave explicit capacity for administrative/maintenance connections. Example: 1 instance with data 80 + aux 10 = 90, leaving ~10 headroom on a default 100-conn server; 2 instances needs a server tuned above 180+headroom.

## 3. API compatibility (G-API)

```text
G-API-01  Declared API behavior must not change without an explicit contract update.
          Enforcement: process + test — docs/reference/api-contract.md is the contract; compat suite (roadmap Phase 1, gap) ⚠️
G-API-02  Status code changes require explicit review.
          Enforcement: process — DoD + Level B confirm (contract change) ✅
G-API-03  Response schema changes require explicit review.
          Enforcement: process — DoD + Level B confirm ✅
G-API-04  Authentication and authorization semantics must not change silently.
          Enforcement: process + test — api-contract §7; compat tests (gap) ⚠️
G-API-05  Filtering, sorting, pagination, and validation semantics require compatibility tests.
           Enforcement: test — tools/search translation; compatibility suite (Phase 1, gap) ⚠️
```

## 4. Security (G-SEC)

```text
G-SEC-01  No secrets in source control.
          Enforcement: CI — gitignore + code review + gitleaks full-history scan on every PR/push
          (job secret-scan in .github/workflows/security-scan.yaml; rules .gitleaks.toml) ✅
G-SEC-02  Production secrets must be supplied through protected configuration.
          Enforcement: config — env / secret store; PB_ENCRYPTION_KEY via --encryptionEnv ✅
G-SEC-03  Metrics must not be publicly exposed unintentionally.
          Enforcement: code — metrics listener loopback-only default; non-loopback requires
          PB_METRICS_EXPOSE=true ✅
G-SEC-04  PostgreSQL must not be publicly exposed unintentionally.
          Enforcement: config — prod compose never publishes the DB port ✅
G-SEC-05  Production containers/processes should run with least privilege.
          Enforcement: config — non-root image, systemd hardening; DB role is not superuser ✅
G-SEC-06  Administrative/superuser operations require explicit access controls.
          Enforcement: code+config — superuser-only API groups, SuperuserIPs whitelist,
          TrustedProxy guard ✅
G-SEC-07  Rate limiting must protect sensitive endpoints.
          Enforcement: code+config — per-route rate limits; login OTP 5/180s hardcoded;
          must be enabled in settings for production ✅
G-SEC-08  TLS configuration must be explicit in production.
          Enforcement: config+code — HSTS opt-in (PB_HSTS), sslmode warnings, production runbook ✅
```

## 5. Reliability (G-REL)

```text
G-REL-01  No external dependency call may block forever.
          Enforcement: code — layered timeouts (30s query / 60s stmt / 10s dial / 5m HTTP) ✅
G-REL-02  PostgreSQL outage must result in bounded failure.
          Enforcement: code + test — connect_timeout fail-fast; experiment A PASS
          (`evidence/experiments/EXPERIMENT-A-20260912-161420.md`) ✅
G-REL-03  Transient PostgreSQL recovery must not require manual process restart unless explicitly
          documented.
          Enforcement: code — pgx reconnect on pool recycle (ConnMaxLifetime 30m); PgBouncer notes ✅
G-REL-04  Backup is not considered valid until restoration has been verified.
          Enforcement: process — restore drill (Phase 0) + last_verified_backup signal
          (gap — metric to add) ⚠️
G-REL-05  Recovery procedures must be executable by an operator who did not write the original feature.
          Enforcement: process + docs — docs/deployment/disaster-recovery.md; drill per gate ⚠️
G-REL-06  Data corruption must be treated as a release-blocking failure.
          Enforcement: process — failure classification C/D; Failure Analysis release gate ✅
```

## 6. CI / release gates (G-CI)

```text
G-CI-01  Distribution artifacts (:latest Docker tag, release binaries) must not
         be published until all quality gates (tests, race, lint) have passed.
         Enforcement: CI — release.yaml job `docker` depends on [goreleaser,
         race, lint] via `needs:`; :latest is only promoted by docker-latest.yaml
         on the `release: [published]` event ✅
G-CI-02  The :latest tag must always equal the last published GitHub release.
         Enforcement: CI — docker-latest.yaml re-tags the validated immutable
         image from the release event, not from the tag push ✅
```

## 7. Upgrade (G-UPG)

```text
G-UPG-01  Every caller-visible behavior change must be written in the API contract.
          Enforcement: process + docs — docs/reference/api-contract.md ✅
G-UPG-02  Dependency advisories (Go, npm) must be triaged.
          Enforcement: process + CI — weekly security-scan (govulncheck, npm audit) ✅
G-UPG-03  Schema changes must be versioned.
          Enforcement: code — migrations/ + _migrations history ✅
G-UPG-04  An upgrade must have a documented rollback/recovery strategy.
           Enforcement: process + docs — upgrade runbook (Phase 0 versioning, gap) ⚠️
G-UPG-05  Existing data must remain readable after a supported upgrade.
           Enforcement: test — migration round-trip tests; PG 16/17 matrix (Phase 0, gap) ⚠️
G-UPG-06  Breaking changes must be explicit.
           Enforcement: process — versioning policy (Phase 0, gap) + Level B confirm ✅⚠️
```

## 8. AI decision authority (G-AI)

AI agents perform most routine development for this project. Their autonomy is bounded by the same guard rails, plus explicit decision authority:

```text
G-AI-01  An AI agent may decide and execute (Level A) only changes that do not cross any G-DATA /
         G-API / G-SEC / G-REL / G-UPG guard rail and do not change the public contract.
G-AI-02  Anything that is not Level A is Level B — Explicit human confirm. That includes former
          B1 cases (dependency security fix, reversible non-contract refactor, non-destructive
          schema change) and former B2 cases (API contract, destructive migration, new dependency,
          security-posture change). Open a DRR. Do not apply the change until a human confirms it.
G-AI-03  There is no silent-approval window. Absence of objection is not confirmation. A Level B
          change stays blocked until the DRR records an explicit human confirm.
G-AI-04  When in doubt raise, never lower the classification: an AI must not reclassify a Level B
         decision as Level A.
G-AI-05  Every non-trivial AI change must complete the 10-step Definition of Done (AGENTS.md §DoD),
         referencing the invariant and guard-rail IDs it touches.
G-AI-06  An AI must classify a failure (A–E, failure-modes.md §3) BEFORE changing code. It may never
         jump directly from a failing test to a code fix without recording the classification.
G-AI-07  An AI that produces evidence (failure experiment, restore drill, benchmark) must record it in
         evidence/ with the L0–L3 verification level; claims without a traceable artifact are not
         evidence.
G-AI-08  The roadmap and this file are the AI's contract: when executing an item, an AI must follow
         the item's declared invariant / guard rail / observation / test / evidence header.
```

**Level mapping** (decision authority, per the AI-driven governance decision):

| Level | Meaning | Example | Action |
|---|---|---|---|
| **A** | Decide & execute | code structure, new tests for existing invariants, a metric that crosses no guard rail, running experiments | Execute, record |
| **B** | Explicit human confirm | dependency fix, non-contract refactor, API contract change, destructive migration, new dependency, security exposure | DRR → blocked until a human confirms |

## 9. Guard-rail gaps (currently open)

These guardrails are defined but their enforcement is `(gap)`; they are tracked in [Evaluation Checklists](./checklists.md) and the roadmap:

| Guard rail | Missing enforcement | Linked roadmap item |
|---|---|---|
| G-REL-01 | timeout and rollback counters | Phase 0 / diagnostic metrics |
| G-API-01/04/05 | compatibility test suite | Phase 1 / compatibility suite |
| G-REL-04 | restore drill + `last_verified_backup` metric | Phase 0 / restore drills |
| G-UPG-04 | upgrade runbook | Phase 0 / versioning |
| G-UPG-05 | PG 16/17 CI matrix | Phase 0 / versioning |
| G-UPG-06 | versioning & upgrade policy | Phase 0 / versioning |

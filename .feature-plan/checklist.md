# PG-BASE — Development Checklist

> Derived from `roadmap.md` · Last updated: 2026-09-09 · Baseline: v0.5.2 (`923e860`)

---

## Phase 0 — Ship the Wedge (P0)

### Sprint 0a: Trust & Trial Path

- [x] **1. Fork `.github/SECURITY.md`**
  - Replace PocketBase contact (`support@pocketbase.io`) with PG-BASE channel
  - Acceptance: SECURITY.md references PG-BASE contact, not upstream
  - ✅ 2026-09-11: GitHub Private Vulnerability Reporting as the only channel; fork-vs-upstream routing; supported-versions table. Committed on `phase-0`.

- [x] **2. CI: add `-race` to Go tests**
  - Acceptance: `go test -race ./...` passes in CI pipeline
  - ✅ 2026-09-11: dedicated parallel `race` job in basebuild (blocking). Verified locally: full suite green under `-race`.

- [x] **3. CI: add `golangci-lint`**
  - Acceptance: Lint passes cleanly on all Go files
  - ✅ 2026-09-11: `lint` job (blocking), golangci-lint v2.6.2 via checksum-verified `go install`. 162 pre-existing issues fixed first (rebrand broke import order in ~154 files).

- [x] **4. CI: schedule `govulncheck` + `npm audit`**
  - Acceptance: Scheduled workflow runs weekly, fails on critical/high CVEs
  - ✅ 2026-09-11: `security-scan.yaml` weekly Monday 06:00 UTC + manual dispatch. govulncheck (symbol-level) + npm audit (ui full tree; docs prod-only — vitepress dev chain has no-fix advisories). Verified clean locally.

- [x] **5. CI: SHA-pin `lychee-action` in `docs.yaml`**
  - Acceptance: Action pinned by commit SHA, not version tag
  - ✅ 2026-09-11: pinned to e7477775 (v2.9.0).

- [x] **6. CI: fix link-check config**
  - Set `failIfEmpty: true`, remove ignored URL patterns
  - Acceptance: Link check runs and fails on broken URLs
  - ✅ 2026-09-11: `failIfEmpty: true`; Pages ignore removed; loopback + pocketbase.io ignores kept (documented, genuinely uncheckable). Scanned docs now carry checkable URLs (github.com, Pages, tooling) — verified all 200.

- [x] **7. One-command provision (Docker / curl)**
  - `docker run` or `curl | sh` boots PG-BASE + Postgres
  - Acceptance: Returns working URL in <10 minutes from zero
  - ✅ 2026-09-11: BOTH paths. `deploy/quickstart.sh` (curl | sh, compose stack w/ pgvector-ready PG16) + `install.sh` (checksum-verified binary). E2E verified: superuser auth OK, dashboard 200. NOTE: ghcr image publishes on the next `v*` tag — quickstart needs that release first.

- [x] **8. Agent-facing quick start guide**
  - Minimal guide for AI coding agents
  - Acceptance: Agent (or human) can provision backend from guide alone
  - ✅ 2026-09-11: `docs/agents.md` (prompt template, CLI contract, boundaries) + root `AGENTS.md` (repo-contributor agents).

- [x] **9. Positioning & comparison doc**
  - Honest comparison vs PocketBase, Supabase, postgrebase, pg-pocketbase
  - Acceptance: Published with reproducible evidence
  - ✅ 2026-09-11: `docs/comparison.md` — verified-against-repo table, "when NOT to choose" section, dated verification stamp.

- [x] **10. Reproducible releases (GoReleaser + GitHub)**
  - `.goreleaser.yaml` already exists; publish at least one GitHub release
  - Acceptance: GitHub release with binary artifacts downloadable
  - ✅ Pre-existing: 6 published releases (v0.1.0–v0.5.2) with 10 assets each. Known gap: v0.5.1 tag has no published release.

- [x] **11. Document upstream drift decision**
  - Choose: hard fork vs build-tag overlay vs sync contract
  - Acceptance: Documented choice with maintenance-cost estimate
  - ✅ 2026-09-11: `FORK_STRATEGY.md` (repo root) — stay hard fork; watch→triage→act; security SLA; adopt/skip/diverge rules; revisit triggers; ~1-2 days/month cost estimate. Cross-linked from SECURITY.md + contributing-releasing.md.

---

### Sprint 0b: Differentiator

- [ ] **12. pgvector field type**
  - `vector` column type with dimension + distance metric options
  - Exposed through PocketBase-style schema/API
  - Acceptance: Create field, insert records, query via REST API

- [ ] **13. Vector indexes (IVFFlat / HNSW)**
  - Acceptance: Index creation works; trade-offs documented (accuracy vs speed vs memory)

- [ ] **14. Similarity query API**
  - `sort=embedding.<metric>` with filter + top-K through REST API
  - Acceptance: No custom SQL required; documented in API docs

- [ ] **15. MCP server v1 (`pgbase mcp`)**
  - Read-mostly tools: list collections, schema, query records, auth
  - Acceptance: Agent can discover and use schema/data through MCP

- [ ] **16. Deterministic CLI contract**
  - Ensure `serve`, `migrate`, `backup`, `restore`, `superuser` are idempotent
  - Acceptance: All subcommands flag-documented, idempotent, safe for agent use

---

### Sprint 0c: Migration

- [ ] **17. Migration CLI (`pgbase migrate --from pocketbase`)**
  - Wrapper around existing `core/backup_sqlite_import.go` engine
  - Acceptance: Validation, dry-run, progress output, safe failure handling

- [ ] **18. Compatibility test suite**
  - Automated tests: PB REST API, auth, filters, relations, expand, realtime, files, SDK behavior
  - Acceptance: Suite runs in CI, representative PocketBase app passes

- [ ] **19. Compatibility matrix**
  - Document supported PB behavior, JS/Dart SDK versions, PG versions, known differences
  - Acceptance: Matrix published, covers common use cases

- [ ] **20. Migration verification**
  - Pre/post counts, relation checks, auth checks, checksums, compatibility report
  - Acceptance: Partial or corrupt migration fails loudly with actionable error

- [ ] **21. Migration playbook**
  - Minimal-code migration guide with examples and failure recovery
  - Acceptance: Followable by developer without additional help

**Phase 0 Exit Criteria:**
> Developer/agent goes zero → running PG-BASE + vector collection in <10 minutes.
> PocketBase app migrates with minimal code changes, repeatable & verifiable.

---

## Phase 1 — Credibility (P0/P1)

- [ ] **22. CVE/advisory triage SLA**
  - Monitor PB releases + Go/npm advisories
  - Acceptance: Documented SLA; triage and backport within timeframe

- [ ] **23. Versioning & upgrade policy**
  - Acceptance: Versioning scheme, breaking-change policy, security-backport policy defined

- [ ] **24. Compatibility test suite in CI**
  - Acceptance: Suite blocks release on failure

- [ ] **25. Published positioning & comparison doc**
  - Acceptance: Public, with reproducible benchmarks or evidence

- [ ] **26. Security regression tests**
  - Tests for: SSRF, file/download limits, SQL identifiers, auth, permissions, encryption-key
  - Acceptance: Tests exist and run in CI

- [ ] **27. Agent-facing quick start (published)**
  - Acceptance: Guide live, tested by external user or agent

- [ ] **28. Hybrid search (FTS + vector RRF)**
  - New sort grammar in `tools/search`; reciprocal rank fusion
  - Acceptance: FTS + vector combined search works through API

- [ ] **29. Embedding-on-write hook**
  - Webhook/jsvm hook auto-populates vectors on record create/update
  - Acceptance: Vector field populated automatically on write

- [ ] **30. Full-text search guidance**
  - PostgreSQL FTS examples and integration patterns
  - Acceptance: Documented patterns cover common use cases

**Phase 1 Exit Criteria:**
> Release cadence established; public comparison page with reproducible evidence.

---

## Phase 2 — Scale Out (P1)

- [ ] **31. Distributed rate limiter**
  - Shared PG/Redis store
  - Acceptance: N instances = configured limit, not N× the limit

- [ ] **32. Realtime outbox: fix phantom-delete**
  - Publish delete events post-commit, inside record txn
  - Acceptance: Failed delete does not notify peers

- [ ] **33. Realtime outbox: replay-on-restart**
  - Events during downtime replayed from DB on instance start
  - Acceptance: Instance restart does not lose events

- [ ] **34. DB-backed settings/collections invalidation**
  - Replace fsnotify sentinel with DB-backed (LISTEN) channel
  - Acceptance: Cross-host deployments no longer serve stale data

- [ ] **35. Multi-instance topology docs + load test**
  - Document S3 requirement; startup warning for local storage + N heartbeats
  - Acceptance: Documented deployment pattern with load-test results

- [ ] **36. PgBouncer + pool-sizing guidance (publish)**
  - Already implemented; needs end-to-end documentation
  - Acceptance: Validated PgBouncer transaction-mode setup documented

- [ ] **37. Load/soak harness**
  - k6/vegeta scenarios: auth, CRUD, relations+expand, realtime fanout, concurrent writes, multi-instance, pool saturation
  - Acceptance: Reproducible scenarios, results recorded

- [ ] **38. Cron coordination: document fail-open edge**
  - Lock-acquisition error → job runs unguarded on every instance
  - Acceptance: Edge case documented with mitigation guidance

- [ ] **39. Multi-instance file storage (S3 requirement)**
  - Document S3 as required for N-instance; consider startup warning
  - Acceptance: Operators informed before hitting 404s

- [ ] **40. Token revocation (cross-instance)**
  - Optional shared immediate-revocation mechanism
  - Acceptance: Cross-instance logout revokes tokens immediately

**Phase 2 Exit Criteria:**
> `LB → N instances → Postgres` is a tested, documented deployment pattern
> that survives instance restarts without duplicated jobs, broken realtime,
> or inconsistent security behavior.

---

## Phase 3 — Operational Maturity (P1)

- [ ] **41. OpenTelemetry tracing**
  - Request → auth → collection → SQL path; OTLP export
  - Acceptance: Traces exported; queryable in Jaeger/Tempo/Grafana

- [ ] **42. Structured request logs**
  - Configurable log sinks (JSON, etc.)
  - Acceptance: JSON logs emitted; configurable via env/settings

- [ ] **43. PITR/WAL archiving guidance**
  - Document pgBackRest or WAL-G for large databases
  - Acceptance: Documented workflow, tested on representative DB

- [ ] **44. Restore drills**
  - Automated or documented restore verification
  - Acceptance: Restore + integrity check repeatable

- [ ] **45. Restore verification (hardened)**
  - Post-restore: per-table counts, expected schema, settings sanity
  - Acceptance: Partial restore fails loudly with actionable error

- [ ] **46. RPO/RTO guidance**
  - Define expectations for dump-based vs WAL/PITR workflows
  - Acceptance: Documented with concrete numbers

- [ ] **47. Upgrade runbook**
  - PostgreSQL major-version upgrades + PG-BASE app upgrades
  - Acceptance: Step-by-step runbook with rollback plan

- [ ] **48. Default Grafana dashboards**
  - Pre-built dashboards for key signals in `deploy/`
  - Acceptance: Dashboards importable, show meaningful data

- [ ] **49. Alertmanager rules**
  - Pool saturation, p99 latency, outbox lag, error rate, backup failures
  - Acceptance: Rules defined, tested, documented

- [ ] **50. Health/readiness endpoints**
  - `/api/ready` with DB connectivity checks for orchestrators
  - Acceptance: Liveness ≠ readiness; orchestrators can use both

**Phase 3 Exit Criteria:**
> Operators can diagnose, recover, and restore with documented procedures.

---

## Phase 4 — Ecosystem (P1/P2)

- [ ] **51. Webhooks with retries + DLQ**
  - Retry with exponential backoff, dead-letter queue, idempotency
  - Acceptance: Failed webhook retried; DLQ inspectable

- [ ] **52. Starter templates**
  - Next.js, Vue, Svelte deployment templates
  - Acceptance: "Deploy PG-BASE + stack in 5 minutes" guides

- [ ] **53. SDK compatibility docs**
  - JS and Dart SDK compatibility with version matrix
  - Acceptance: Documented, covers common operations

- [ ] **54. Background-job primitive**
  - Deliberately small, not a workflow platform
  - Acceptance: Basic enqueue/dequeue/retry works

- [ ] **55. PostgreSQL extension guidance**
  - pg_trgm, PostGIS usage patterns
  - Acceptance: Documented with examples

- [ ] **56. GIN JSONB optimization**
  - Benchmark-gated; representative workload validation
  - Acceptance: Before/after benchmark proves value

- [ ] **57. Dashboard UI tests**
  - Automated tests for `ui/src/audits/` and fork-specific pages
  - Acceptance: Tests run in CI, cover critical paths

- [ ] **58. Fuzz/property tests**
  - Filter → SQL translation layer (`tools/search` + `tools/dbutils`)
  - Acceptance: Fuzzing finds no crashes on malformed input

- [ ] **59. CI PostgreSQL matrix**
  - Test across PG 16/17
  - Acceptance: Tests pass on all supported PG versions

- [ ] **60. Coverage measurement + gate**
  - CI measures coverage; gate after baseline
  - Acceptance: Coverage reported per PR; gate enforced

- [ ] **61. JS VM hooks parity + docs**
  - jsvm hooks documented and tested
  - Acceptance: Hooks match documented behavior

- [ ] **62. Docker Compose templates**
  - Deployment templates before K8s/Helm
  - Acceptance: Templates boot correctly with documented config

**Phase 4 Exit Criteria:**
> Starter templates exist; first community contributions from outside maintainers.

---

## Known Weaknesses — Action Items

| # | Issue | Linked Task | Priority |
|---|-------|-------------|----------|
| 63 | Phantom deletes (outbox delete published pre-commit) | Task 32 | P1 |
| 64 | Outbox at-most-once (no replay on restart) | Task 33 | P1 |
| 65 | Cron guard fails open on lock-acquisition error | Task 38 | P1 |
| 66 | Cache invalidation fsnotify-based, not DB-backed | Task 34 | P1 |
| 67 | Local file storage breaks multi-instance | Task 39 | P1 |
| 68 | `PGTEST_PASSWORD`/`PGTEST_SSLMODE` in prod code | — | P0 |
| 69 | Audit DEFAULT partition never pruned | — | P2 |
| 70 | Restore success gating too weak (`count >= 1`) | Task 45 | P1 |
| 71 | `.github/SECURITY.md` verbatim upstream | Task 1 | P0 |
| 72 | CI: no `-race`, no lint, no scheduled vuln scans | Tasks 2-6 | P0 |
| 73 | UI has zero automated tests | Task 57 | P2 |
| 74 | Minor debt: ignored errors, dead code, hardcoded DSN | — | Low |

---

## Summary

| Phase | Items | Priority | Est. Effort |
|-------|-------|----------|-------------|
| Sprint 0a | 11 | P0 | Days |
| Sprint 0b | 5 | P0 | Weeks |
| Sprint 0c | 5 | P0 | Weeks |
| Phase 1 | 9 | P0/P1 | Weeks |
| Phase 2 | 10 | P1 | Weeks |
| Phase 3 | 10 | P1 | Weeks |
| Phase 4 | 12 | P1/P2 | Weeks |
| Weaknesses | 12 | P0-P2 | Mixed |
| **Total** | **~74** | | |

---

*Checklist derived from `roadmap.md`. Update this file as tasks are completed.*

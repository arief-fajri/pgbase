# PG-BASE — Development Roadmap

<DocMeta audience="All" status="living document" verified="v0.5.2 (923e860)" />

A forward-looking plan for **pgbase**, the PostgreSQL fork of PocketBase. This
document captures where the project stands today (as of `v0.5.2`), the gaps that
remain, and a phased roadmap toward a production-grade, horizontally scalable,
maintainable backend.

> This is a living document. Items are grouped by theme and tagged with a
> rough priority (**P0** blocker / **P1** high / **P2** nice-to-have) and effort
> (**S**/**M**/**L**). Nothing here is a commitment to a date.

---

## 1. Where we are today (baseline)

The fork has completed its foundational work: a full SQLite → PostgreSQL port
with the test suite green, a security audit with nearly all findings remediated,
and several rounds of performance hardening.

**Shipped (v0.1.0 → v0.5.2):**

- **Complete PostgreSQL port** — `pgx` v5 + `dbx`, JSONB columns/path access,
  `timestamptz` dates, `pgcrypto` id defaults, `information_schema`/`pg_catalog`
  introspection. All SQLite-only code paths removed. (v0.1.0)
- **Audit trails** — `_audits` (data changes) + `_audit_reads` (read access),
  month-partitioned, superuser API + dashboard. (v0.2.0)
- **Portable + native backups** — legacy SQLite import (v0.22/v0.23) and native
  `pg_dump`/`pg_restore` as the default format, with offline CLI. (v0.3.0, v0.4.0)
- **Performance batch** — env-tunable connection pools, functional `LOWER()`
  identity indexes, batched log writes, bulk MFA/OTP cleanup, gzip on `/api`,
  expand N+1 removal, realtime access memoization, index-diff sync, bounded
  writes, cron advisory locks. (v0.5.0)
- **Opt-in observability** — Prometheus `/metrics` on a dedicated listener. (v0.5.0)
- **Opt-in cross-instance realtime** — DB-backed outbox with `NOTIFY`, cursor
  paging, origin stamping. (v0.5.0)
- **Security hardening** — SSRF guard, download size caps, HTTPS-only asset
  updates with checksum verification, SQL identifier quoting, pinned CI actions
  and Docker digests, `sslmode=prefer` default, mandatory encryption-key guidance.
  (v0.5.1 rebrand, v0.5.2)
- **Production docs** — [production runbook](./production), `docker-compose.prod.yml`
  (Caddy auto-HTTPS), monitoring stack under `deploy/`.

**Health snapshot:** build + full suite green (`-p 4`); `govulncheck ./...` and
`npm audit` clean; ~70k LOC Go (excl. UI), 217 test files.

---

## 2. Guiding principles

1. **API compatibility with PocketBase** stays a hard constraint — clients and
   SDKs should not need to change.
2. **Postgres-native, not SQLite-emulated** — lean into PG strengths
   (partitioning, GIN, MVCC, `LISTEN/NOTIFY`) rather than porting SQLite habits.
3. **Safe defaults, opt-in complexity** — single-instance stays zero-config;
   multi-instance / scale features are opt-in until proven.
4. **No silent divergence from upstream** — track PocketBase upstream so we can
   pull security fixes and features.

---

## 3. Roadmap themes

### Theme A — Horizontal scalability (multi-instance) — **P1**

This is the single biggest gap. Several subsystems assume one process.

| Item | Status today | Target | Prio / Effort |
|---|---|---|---|
| **Realtime outbox** | Opt-in (`PB_REALTIME_OUTBOX`), off by default | Harden, load-test, document HA topology; consider default-on once battle-tested | P1 / M |
| **Rate limiter** | In-memory per-instance (`store.Store` + map) → N instances = N× the configured limit | Shared backend (Postgres table or Redis) or clearly documented per-instance semantics | P1 / M |
| **Cached collections / settings** | Per-instance in-memory cache, reloaded via file-watch + `NOTIFY` | Verify cache-invalidation propagates across instances under load | P1 / M |
| **Cron coordination** | `pg_try_advisory_lock` guards (done) + instance heartbeat | Verify single-fire under N instances; document leader semantics | P2 / S |
| **Session / token revocation** | Stateless JWT | Optional shared revocation list for immediate logout across instances | P2 / M |

**Deliverable:** a "Running multiple instances" section in the [production runbook](./production) with a
tested topology (LB → N app instances → shared PG), plus the outbox and rate
limiter promoted from experimental.

---

### Theme B — Performance & schema at scale — **P1/P2**

Deferred items from the performance audit (all validated as real, paused pending
scope decisions).

| ID | Item | Status | Prio / Effort |
|---|---|---|---|
| **PERF-I04** | Partition `_logs` (currently a plain table; retention = 6-hourly bulk DELETE → dead-tuple churn/bloat) | RANGE-partition like `_audits` → DROP partition; or lighter per-table autovacuum tuning (partial: `logs_autovacuum_tuning` migration exists) | P1 / L |
| **PERF-I02** | GIN / expression indexes on JSONB multi-value columns (every multi-value filter/expand = full scan) | (a) allow `USING GIN (col jsonb_path_ops)` in validation+sync; (b) rewrite multi-value filters to `@>` containment so GIN is used | P2 / L |
| **PERF-I05** | Audit `DEFAULT` partition holds current-month rows and is never pruned | Pre-create current-month partition + migrate `DEFAULT` rows | P2 / M |
| **PERF-I08** | User index-add on populated collections uses blocking `CREATE INDEX` inside a tx | Out-of-tx `CREATE INDEX CONCURRENTLY` path (documented escape hatch exists in [developing](./developing) #5b) | P2 / L |
| **PERF-I07** | Random 15-char TEXT primary keys (random B-tree insert, bloat, no time-order) | Architectural/upstream; awareness only, likely won't change (API contract) | P2 / — |

**Deliverable:** a load-test harness (see Theme E) to quantify each before/after,
so these ship with evidence rather than intuition.

---

### Theme C — Upstream tracking & maintenance — **P0/P1**

A hard fork of PocketBase ~v0.23 accrues drift risk. This is the highest-risk
long-term item even though it isn't a feature.

- **P0** — Establish an **upstream-CVE / security-advisory tracking process**:
  watch PocketBase releases + Go/npm advisories, triage what applies to the fork,
  backport fixes. Currently ad-hoc.
- **P1** — **Feature-parity review** against PocketBase v0.24+ — decide per
  feature: adopt, skip, or diverge. Document the divergence explicitly.
- **P1** — **Dependency update cadence** — scheduled `govulncheck` + `npm audit`
  in CI (fail the build on new called-vulns), Dependabot/Renovate for the pinned
  digests and Go modules.
- **P2** — Document the fork's **compatibility matrix**: PocketBase JS/Dart SDK
  versions known to work, PostgreSQL server versions supported (16+).

---

### Theme D — Observability maturity — **P2**

Metrics exist; the next step is depth and correlation.

- **P2** — **Structured request logging** improvements + configurable log sinks.
- **P2** — **OpenTelemetry tracing** (opt-in) — span the request → DB query path;
  export OTLP. High value for diagnosing multi-instance latency.
- **P2** — **More metrics**: per-collection query latency, cache hit/miss,
  outbox lag, backup duration/size, migration timings.
- **P2** — Ship default **Grafana dashboards** + Alertmanager rules in `deploy/`
  (pool saturation, p99 latency, outbox lag, error rate).

---

### Theme E — Testing, quality & release engineering — **P1**

- **P1** — **Load/soak testing harness** — reproducible k6/vegeta scenarios
  (auth, CRUD, realtime fanout, expand-heavy) to gate the Theme B perf items and
  catch regressions. Currently no perf regression coverage.
- **P1** — **Release hygiene** — tags are local-only and the project is still
  `v0.x`; formalize: push tags, cut real GitHub releases via goreleaser, keep the
  per-branch versioning discipline already in use.
- **P2** — **CI matrix** across supported PostgreSQL major versions (16/17).
- **P2** — **Coverage gate** in CI (a `coverage.out` already exists but isn't
  enforced).
- **P2** — **Fuzz/property tests** for the filter → SQL translation layer (the
  riskiest fork-diverged code, `tools/search` + `tools/dbutils`).

---

### Theme F — Backup / disaster recovery — **P2**

Native `pg_dump` backups are solid for small/medium; scale needs more.

- **P2** — **PITR / WAL archiving** guidance (pgBackRest / WAL-G) for large DBs
  where full `pg_dump` per backup is too heavy.
- **P2** — **Incremental / differential** backup story (or explicitly defer to
  managed Postgres snapshots and document it).
- **P2** — **Automated off-machine offload** — first-class S3 backup destination
  wiring + retention, beyond the manual "copy archives off the machine" note.
- **P2** — **Restore drills** — a documented, tested restore verification step.

---

### Theme G — Developer & operator experience — **P2**

- **P2** — **Connection-pooling guide** maturation — validated PgBouncer
  transaction-mode setup end-to-end (the `default_query_exec_mode` toggle exists;
  needs a tested recipe).
- **P2** — **Migration tooling** — smoother authoring/preview of collection
  migrations; document the `pb_migrations` workflow for the PG fork.
- **P2** — **Kubernetes** deployment example (Helm chart or manifests) alongside
  the existing docker-compose paths.
- **P2** — **JS VM (`jsvm`) hooks** parity + docs for the fork.

---

## 4. Suggested sequencing

A pragmatic order that front-loads risk reduction and unblocks the rest:

1. **Now → next (P0/P1 foundation)**
   - Theme C: upstream-CVE tracking process + CI `govulncheck`/`npm audit` gate.
   - Theme E: release hygiene (push tags, real releases) + load-test harness.
2. **Scale-out (P1)**
   - Theme A: rate-limiter shared store + realtime-outbox hardening → publish the
     multi-instance topology.
   - Theme B: `_logs` partitioning (I04) — highest ops-pain perf item — gated by
     the new load harness.
3. **Depth (P2)**
   - Theme B: GIN JSONB (I02), audit-partition fix (I05), `CONCURRENTLY` (I08).
   - Theme D: OpenTelemetry tracing + shipped Grafana dashboards.
   - Theme F: PITR/WAL guidance + S3 offload automation.
4. **Polish (P2)**
   - Theme G: PgBouncer recipe, K8s example, migration tooling, SDK matrix.

---

## 5. Explicitly out of scope (for now)

- Re-adding SQLite as a supported backend (the fork's premise is PG-only).
- Changing the random TEXT primary-key scheme (PERF-I07) — breaks the PocketBase
  API contract; revisit only with a strong scale justification.
- Diverging the REST API from PocketBase compatibility.

---

## 6. How to contribute to the roadmap

Open a discussion/issue referencing the theme and item ID (e.g. *"Theme B /
PERF-I04"*). Perf items should come with a load-test scenario; scale-out items
should include a multi-instance test plan. See [Contributing](./contributing) and [Developing](./developing).

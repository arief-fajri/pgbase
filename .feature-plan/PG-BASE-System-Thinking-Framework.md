# PG-BASE System Thinking Framework

> Engineering framework for evolving PG-BASE from a PocketBase fork into a production-grade, PostgreSQL-native backend platform.

---

## 1. Purpose

PG-BASE started from a simple idea:

> Replace PocketBase's SQLite persistence layer with PostgreSQL.

That framing is useful for the first prototype, but it is too narrow for a serious project.

PG-BASE should be treated as a system with:

- PostgreSQL as a first-class infrastructure dependency
- REST API
- Authentication and authorization
- Realtime subscriptions
- File storage
- Administration UI
- Database migrations
- Backups and recovery
- Observability
- Security controls
- Deployment and upgrade lifecycle
- Compatibility with upstream PocketBase behavior where compatibility is promised

The goal of this document is to establish a feedback loop across four core phases:

```text
┌────────────────────────-┐
│ 1. PLATFORM DESIGN      │  desired outcomes, architecture, invariants
└───────────┬─────────────┘
            ↓
┌───────────────────────-─┐
│ 2. GUARD RAILS          │  constraints: data, security, reliability, compat
└───────────┬─────────────┘
            ↓
        IMPLEMENT
            ↓
┌───────────────────────-─┐
│ 3. OBSERVE & MEASURE    │  metrics, logs, failure experiments
└───────────┬─────────────┘
            ↓
┌──────────────────────-──┐
│ 4. EVALUATE             │  checklists, acceptance criteria
└───────────┬─────────────┘
            ↓
        PASS / FAIL
        ↙        ↘
     PASS        FAIL
      ↓            ↓
   RELEASE      LEARN
                   ↓
           PLATFORM DESIGN (loop back)
```

The important principle is:

> A failed test is not merely a coding problem. It is feedback about the system design.

---

## 2. System Model

### 2.1 System boundary

PG-BASE is a self-hostable backend platform that provides:

```text
Clients
  │
  ├── REST API
  ├── Realtime
  └── Admin/UI
        │
        ▼
     PG-BASE
        │
        ├── Authentication
        ├── Authorization
        ├── Collections / Records
        ├── File Storage
        ├── Realtime
        ├── Hooks / Jobs
        └── Backup / Restore
        │
        ▼
   PostgreSQL
```

External dependencies are part of the system model:

- PostgreSQL
- reverse proxy / TLS termination
- object/file storage where applicable
- PgBouncer where deployed
- Prometheus-compatible monitoring
- container/runtime environment
- upstream PocketBase source and behavior

### 2.2 Desired outcomes

PG-BASE should satisfy these high-level outcomes:

1. **PostgreSQL-native** — PostgreSQL is not an adapter hidden behind a SQLite abstraction. Connection pooling, transactions, locks, timeouts, migrations, and recovery are explicit concerns.
2. **PocketBase-compatible where promised** — existing application behavior should remain compatible unless a change is intentionally documented.
3. **Self-hostable** — a developer/operator can run the platform without a proprietary control plane.
4. **Secure by default** — production defaults should minimize accidental exposure.
5. **Observable** — operators can determine whether the system is healthy and why it is degrading.
6. **Recoverable** — database failure, restart, backup restoration, and migration failure have defined behavior.
7. **Upgrade-safe** — application and database upgrades have explicit compatibility and recovery paths.
8. **Predictable under load** — resource limits and saturation behavior are bounded and measurable.

---

## 3. Phase 1 — Platform Design

Platform Design answers:

> What system are we building, what are its boundaries, and what properties must remain true?

### 3.1 Architecture principles

**P1 — PostgreSQL is a first-class dependency.**
Do not treat PostgreSQL as simply a replacement driver. The architecture must explicitly account for: connection pool sizing, PostgreSQL `max_connections`, connection acquisition latency, query timeout, lock timeout, transaction behavior, migrations, schema ownership, backup/restore, PostgreSQL version compatibility, PgBouncer behavior, failure/reconnection behavior.

**P2 — Preserve application semantics.**
Changing the persistence layer must not unintentionally change: authentication behavior, authorization behavior, filtering, sorting, pagination, API response structure, HTTP status semantics, realtime behavior, file behavior, validation behavior.

**P3 — Fail boundedly.**
Every potentially blocking external operation should have a bounded lifecycle — e.g. HTTP request timeout, database connection acquisition timeout, query timeout, lock timeout, graceful shutdown timeout, realtime connection lifecycle.

**P4 — Data correctness takes priority over availability.**
When there is a conflict:

```text
Corrupt data  <  Temporary request failure
```

The system should prefer rejecting or timing out an operation rather than committing an invalid or partially persisted state.

**P5 — Operational behavior is part of the product.**
Production behavior is not separate from application correctness. The following are product properties: startup behavior, shutdown behavior, migration behavior, backup behavior, restore behavior, failure behavior, observability, security posture.

### 3.2 Core system invariants

**Data invariants**

```text
I1. A failed transaction must not leave a partial transaction state.
I2. A successful write must satisfy all relevant database/application invariants.
I3. Destructive schema changes must be explicit and migration-controlled.
I4. Database schema state must be knowable and reproducible.
I5. Backup data considered valid must be restorable.
```

**API invariants**

```text
I6. API behavior must remain compatible with the declared compatibility contract.
I7. Breaking behavior must be explicit, documented, and versioned where necessary.
I8. HTTP status codes and error semantics must be deterministic.
```

**Security invariants**

```text
I9.  Secrets must not be committed to source control.
I10. Production database access must not be unintentionally public.
I11. Metrics endpoints must not expose sensitive information publicly.
I12. Production processes should run with least privilege.
```

**Resource invariants**

```text
I13. Database connections are bounded.
I14. Long-running operations have explicit timeouts.
I15. Realtime resource usage is bounded or observable.
I16. Shutdown releases resources predictably.
```

### 3.3 Data flow model

For every critical operation, document:

```text
Request → Authentication → Authorization → Validation → Business logic
        → Transaction boundary → PostgreSQL → Commit → Response / Realtime event
```

A critical design question:

> At what point does the system consider the operation successful?

For database-backed operations, the answer should normally be tied to a successful transaction commit, not merely to execution of an in-memory operation.

### 3.4 Failure-mode model

At minimum, design and test these failure modes:

| Failure | Expected system behavior |
|---|---|
| PostgreSQL unavailable | Requests fail predictably and do not hang indefinitely |
| PostgreSQL restarts | Application can recover/reconnect |
| Connection pool exhausted | Requests are bounded by timeout |
| Query too slow | Query/request terminates according to timeout policy |
| Row/table lock contention | Lock wait is bounded |
| Migration fails | Startup/upgrade does not silently continue in an invalid state |
| Realtime disconnects | Client can reconnect without corrupting primary data |
| Backup fails | Failure is visible and backup is not treated as valid |
| Restore fails | Recovery procedure reports failure clearly |
| Application crashes | Database remains consistent |
| Process restarts | Existing data remains usable |
| Invalid input | Request is rejected without partial persistence |

---

## 4. Phase 2 — Guard Rails

Guard rails are constraints that prevent the system from entering unacceptable states. They are not the same as tests:

- A **test** answers: *Did we observe the expected behavior?*
- A **guard rail** answers: *What behavior is never allowed?*

### 4.1 Data integrity guard rails

```text
G-DATA-01  All multi-step writes that must be atomic must use an appropriate transaction boundary.
G-DATA-02  A failed transaction must not partially persist state.
G-DATA-03  Schema changes must be migration-controlled.
G-DATA-04  Destructive migrations require explicit review.
G-DATA-05  Migrations must be safe to execute during controlled deployment and must have a defined failure/recovery path.
G-DATA-06  Application code must not rely on undefined PostgreSQL behavior.
```

### 4.2 PostgreSQL guard rails

```text
G-DB-01  Connection pools must have explicit maximum sizes.
G-DB-02  Total possible application connections must be compatible with the PostgreSQL server's connection capacity.
G-DB-03  Database operations must have bounded execution time.
G-DB-04  Lock waits must be bounded.
G-DB-05  Database credentials must come from protected configuration.
G-DB-06  Production database traffic must use the intended TLS policy.
G-DB-07  Database failures must not cause unbounded request blocking.
G-DB-08  Connection pool exhaustion must be observable.
```

Connection capacity should be reasoned about as a system:

```text
(Application instances × pool size) + Other PostgreSQL clients  <  PostgreSQL max_connections
```

Leave explicit capacity for administrative/maintenance connections.

### 4.3 API compatibility guard rails

```text
G-API-01  Declared PocketBase-compatible endpoints must not change behavior without an explicit compatibility decision.
G-API-02  Status code changes require explicit review.
G-API-03  Response schema changes require explicit review.
G-API-04  Authentication and authorization semantics must not silently diverge.
G-API-05  Filtering, sorting, pagination, and validation semantics require compatibility tests.
```

### 4.4 Security guard rails

```text
G-SEC-01  No secrets in source control.
G-SEC-02  Production secrets must be supplied through protected configuration.
G-SEC-03  Metrics must not be publicly exposed unintentionally.
G-SEC-04  PostgreSQL must not be publicly exposed unintentionally.
G-SEC-05  Production containers/processes should run with least privilege.
G-SEC-06  Administrative/superuser operations require explicit access controls.
G-SEC-07  Rate limiting must protect sensitive endpoints.
G-SEC-08  TLS configuration must be explicit in production.
```

### 4.5 Reliability guard rails

```text
G-REL-01  No external dependency call may block forever.
G-REL-02  PostgreSQL outage must result in bounded failure.
G-REL-03  Transient PostgreSQL recovery must not require manual process restart unless explicitly documented.
G-REL-04  Backup is not considered valid until restoration has been verified.
G-REL-05  Recovery procedures must be executable by an operator who did not write the original feature.
G-REL-06  Data corruption must be treated as a release-blocking failure.
```

### 4.6 Upgrade guard rails

```text
G-UPG-01  Every intentional divergence from upstream PocketBase must have a reason.
G-UPG-02  Upstream changes must be traceable.
G-UPG-03  Schema changes must be versioned.
G-UPG-04  An upgrade must have a documented rollback/recovery strategy.
G-UPG-05  Existing data must remain readable after a supported upgrade.
G-UPG-06  Breaking changes must be explicit.
```

Recommended artifact — `UPSTREAM.md`:

```text
Current upstream: PocketBase <version>

Divergences:
1. SQLite → PostgreSQL
2. ...
3. ...

Reason: ...
Compatibility impact: ...
```

---

## 5. Implementation Loop (between Guard Rails and Observation)

Implementation is intentionally not a separate design phase — it should happen in small, observable increments, using this loop:

```text
Choose one system property
      ↓
Define invariant
      ↓
Define guard rail
      ↓
Implement
      ↓
Add observation/measurement
      ↓
Add automated test
      ↓
Evaluate
      ↓
Record learning
```

Avoid implementing large features without defining how their correctness will be observed.

---

## 6. Phase 3 — Observe & Measure

Observability answers:

> What is actually happening inside the system?

The objective is not to collect every possible metric — it is to make important system behavior explainable.

### 6.1 HTTP — RED model

For major API surfaces, observe:

- **Rate** — requests/sec
- **Errors** — 4xx/sec, 5xx/sec, error ratio
- **Duration** — p50, p95, p99

Useful dimensions: endpoint, method, status code, application version.
Avoid uncontrolled high-cardinality labels such as raw user IDs or arbitrary URLs.

### 6.2 PostgreSQL / pool metrics

```text
DB connections: open · in use · idle · maximum · wait count · wait duration
DB behavior:    query timeout · lock timeout · connection failure · transaction rollback
```

Diagnostic chain:

```text
Latency increase → DB wait increase? → Pool saturation? → Slow query? → Lock contention? → External PostgreSQL issue?
```

Metrics should support diagnosis, not merely dashboards.

### 6.3 Realtime metrics

- connected clients
- active subscriptions
- connection/disconnection rate
- dropped messages
- broadcast latency where measurable
- abnormal reconnect behavior

### 6.4 Backup and recovery metrics

- backup attempt count
- backup success/failure
- backup duration
- backup size
- last successful backup
- last successful restore verification

A particularly useful operational signal is `last_verified_backup_timestamp`, which is more meaningful than `last_backup_timestamp`, because a created backup is not necessarily a usable backup.

### 6.5 Failure experiments

Observability must be tested through controlled failure.

**Experiment A — PostgreSQL outage**
Start PG-BASE → stop PostgreSQL → send requests → observe latency/errors → start PostgreSQL → send requests again → verify recovery.
*Expected:* no infinite request hangs, errors are bounded, no data corruption, recovery behavior is deterministic.

**Experiment B — Pool saturation**
Concurrent requests → exhaust DB pool → observe request latency, wait count, wait duration, error rate.
*Question:* does the system degrade predictably near its resource boundary?

**Experiment C — Lock contention**
Transaction A holds a lock; Transaction B attempts a conflicting operation → observe lock wait, timeout, response.

**Experiment D — Migration failure**
Migration starts → migration fails → restart application → verify database state → verify recovery procedure.

**Experiment E — Backup restoration**
Production-like database → create backup → destroy test database → restore backup → run integrity checks → run application tests.

---

## 7. Phase 4 — Evaluate / Checklist

Checklists should be split by system boundary. A single giant checklist becomes difficult to maintain and eventually stops being useful.

### 7.1 Core correctness checklist

```text
[ ] Application starts against a fresh PostgreSQL database
[ ] Migrations complete successfully
[ ] Application restarts successfully
[ ] CRUD operations work
[ ] Transactions are atomic
[ ] Failed writes do not partially persist
[ ] Concurrent writes behave correctly
[ ] Database constraints are enforced
[ ] Shutdown releases database resources
[ ] Startup failure is explicit and diagnosable
```

### 7.2 PostgreSQL checklist

```text
[ ] Fresh database works
[ ] Existing database works
[ ] Migration state is reproducible
[ ] Migrations fail safely
[ ] Transaction rollback works
[ ] Query timeout works
[ ] Lock timeout works
[ ] Connection acquisition timeout works
[ ] Pool saturation is bounded
[ ] Pool saturation is observable
[ ] PostgreSQL restart is recoverable
[ ] Network interruption is recoverable
[ ] TLS configuration works in production mode
[ ] PgBouncer transaction pooling works if supported
```

### 7.3 API compatibility checklist

```text
[ ] Authentication behavior is compatible
[ ] Authorization behavior is compatible
[ ] REST endpoints are compatible
[ ] HTTP status codes are compatible
[ ] Response structures are compatible
[ ] Error structures are compatible
[ ] Filtering is compatible
[ ] Sorting is compatible
[ ] Pagination is compatible
[ ] Validation is compatible
[ ] Realtime behavior is compatible
[ ] File behavior is compatible
```

Where possible, every item above should become an automated compatibility test:

```text
Reference behavior
      │
      ├── PocketBase
      └── PG-BASE
            ↓
      Compare observable behavior
```

Do not require implementation-level equality — require the promised external behavior to be equivalent.

### 7.4 Security checklist

```text
[ ] No secrets committed
[ ] Production secrets supplied securely
[ ] TLS enabled/configured
[ ] Database not unintentionally public
[ ] Metrics not unintentionally public
[ ] Process/container uses least privilege
[ ] Rate limiting enabled for sensitive endpoints
[ ] Administrative endpoints protected
[ ] CORS explicitly configured
[ ] Trusted proxy configuration reviewed
[ ] Encryption key/configuration reviewed
[ ] Backup/restore operations protected
```

### 7.5 Disaster recovery checklist

```text
[ ] Backup can be created
[ ] Backup failure is observable
[ ] Backup can be stored outside the application host
[ ] Backup can be restored
[ ] Restore produces a valid schema
[ ] Restore preserves application data
[ ] Restore preserves authentication data
[ ] Restore preserves required files/configuration
[ ] Restore procedure is documented
[ ] Restore procedure has been executed successfully
[ ] Last verified backup is observable
```

Define explicit recovery targets once PG-BASE reaches production (example only):

```text
RPO ≤ 15 minutes   (maximum acceptable data loss)
RTO ≤ 1 hour       (maximum acceptable recovery time)
```

The actual values must be determined by the product's operational requirements.

### 7.6 Release checklist

A release should not be considered ready merely because unit tests pass.

```text
[ ] Unit tests pass
[ ] Integration tests pass
[ ] PostgreSQL compatibility tests pass
[ ] API compatibility tests pass
[ ] Realtime tests pass
[ ] Security checks pass
[ ] Migration tests pass
[ ] Backup/restore verification passes
[ ] Failure experiments for affected subsystem pass
[ ] Observability exists for affected subsystem
[ ] Documentation updated
[ ] Upstream divergence reviewed
[ ] Rollback/recovery procedure exists
```

---

## 8. Feedback Loop & Failure Classification

The core system-thinking mechanism ties the four phases together:

```text
PLATFORM DESIGN → GUARD RAILS → IMPLEMENT → OBSERVE & MEASURE → EVALUATE
                                                                      ↓
                                                                 PASS / FAIL
                                                                 ↙        ↘
                                                              PASS        FAIL
                                                               ↓            ↓
                                                            RELEASE      LEARN
                                                                           ↓
                                                                 REDESIGN SYSTEM
                                                                           │
                                                                           └──→ PLATFORM DESIGN
```

The critical rule:

> **Do not automatically jump from failure to code changes.** First classify the failure.

### 8.1 Failure classification

| Type | Pattern | Response |
|---|---|---|
| **A — Implementation failure** | Design ✓, Guard rail ✓, Implementation ✗ | Fix implementation |
| **B — Design failure** | Implementation ✓, but design was insufficient | Change the platform design |
| **C — Missing guard rail** | System entered a dangerous state that was not prevented | Create a new guard rail |
| **D — Missing observability** | System failed but operators could not determine why | Add measurement |
| **E — Incorrect acceptance criteria** | Checklist passed but real-world behavior was unsafe | Redesign the evaluation criteria |

---

## 9. Definition of Done for a System Change

For any non-trivial change, use this sequence:

```text
1.  What system property are we changing?
2.  What desired outcome should change?
3.  What invariant must remain true?
4.  Which guard rails apply?
5.  How will we observe the behavior?
6.  How will we test it?
7.  What failure modes are relevant?
8.  What is the acceptance checklist?
9.  What evidence proves it works?
10. What did we learn after implementation?
```

A change is complete only when there is sufficient evidence, not merely when the code compiles.

---

## 10. Recommended Repository Artifacts

```text
docs/
├── PLATFORM.md          — system boundary, architecture, desired outcomes, invariants, dependencies, data flows
├── GUARDRAILS.md         — non-negotiable constraints
├── OBSERVABILITY.md      — metrics, logs, dashboards, alerts, diagnostic signals, failure experiments
├── CHECKLISTS.md         — release, security, compatibility, database, migration, backup/restore checklists
├── FAILURE-MODES.md      — known failure scenarios and expected behavior
├── DISASTER-RECOVERY.md  — backup, restore, RPO, RTO, recovery runbooks
└── UPSTREAM.md           — PocketBase version, divergence map, compatibility decisions, upgrade strategy
```

---

## 11. Initial Priorities for PG-BASE

Do not try to complete every item simultaneously.

**Phase 1 — Establish the system model**
```text
[ ] Define system boundary
[ ] Define desired outcomes
[ ] Document architecture
[ ] Document PostgreSQL dependency model
[ ] Document upstream PocketBase relationship
[ ] Define initial invariants
```

**Phase 2 — Protect correctness**
```text
[ ] Data integrity guard rails
[ ] Transaction tests
[ ] Migration tests
[ ] PostgreSQL failure tests
[ ] API compatibility tests
```

**Phase 3 — Establish observability**
```text
[ ] HTTP RED metrics
[ ] DB pool metrics
[ ] DB timeout metrics
[ ] Realtime metrics
[ ] Backup/restore signals
[ ] Useful structured logs
```

**Phase 4 — Establish operational confidence**
```text
[ ] Failure experiments
[ ] Backup/restore drills
[ ] Upgrade tests
[ ] Security checklist
[ ] Production release checklist
```

**Phase 5 — Continuous feedback**

```text
Design → Guard Rail → Implementation → Observation → Automated Evaluation → Evidence → Learning → Update Design
```

---

## 12. Final Principle

The purpose of this framework is not to make development slower. It is to prevent PG-BASE from evolving into a system where:

```text
features ↑  complexity ↑  dependencies ↑  failure modes ↑
```

while:

```text
understanding ↓  observability ↓  confidence ↓
```

The desired direction is:

```text
System complexity ↑ + System understanding ↑ + Observability ↑ + Automated evidence ↑ = Controlled complexity
```

The ultimate goal is not:

> "PG-BASE has many tests."

The goal is:

> **"We understand the important behavior of PG-BASE, we know the boundaries it must not cross, we can observe when it approaches those boundaries, and we have evidence that it behaves correctly when those boundaries are tested."**

That is the core of applying system thinking to PG-BASE.

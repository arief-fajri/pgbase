# Platform Design

<DocMeta audience="Contributor" status="stable" verified="v0.5.2" />

> Canonical system model for PG-BASE. This document defines what system we are building, what its boundaries are, and what properties must remain true. Guard rails are codified separately in [Quality Guardrails](./guardrails.md), failure behavior in [Failure Analysis](./failure-modes.md), observability in [Observability](./observability.md).
>
> Related deep-dives: [System Overview](../../architecture/overview.md), [Backend Layers](../../architecture/backend-layers.md).

## 1. System boundary

PG-BASE is a self-hostable backend platform providing:

```text
Clients
  │
  ├── REST API
  ├── Realtime (SSE)
  └── Admin UI (dashboard SPA)
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

- PostgreSQL 16+ (the storage engine; pgcrypto must exist before first boot)
- reverse proxy / TLS termination
- object/file storage where applicable (S3)
- PgBouncer where deployed (transaction or session pooling)
- Prometheus-compatible monitoring
- container / runtime environment
- upstream PocketBase source and behavior (see [Upstream Tracking](../upstream.md))

The runnable entrypoint is `examples/base`; the root Go package is a library. The root package boots a dual-pool PostgreSQL connection set through `PB_POSTGRES_*` env vars (see `core/db_connect.go`, `core/base.go`).

## 2. Desired outcomes

PG-BASE must satisfy these high-level outcomes. Every non-trivial change should be traceable to one or more of them:

1. **PostgreSQL-native** — PostgreSQL is not an adapter hidden behind a SQLite abstraction. Connection pooling, transactions, locks, timeouts, migrations, and recovery are explicit concerns.
2. **PocketBase-compatible where promised** — existing application behavior remains compatible unless a change is intentionally documented ([fork-deltas.md](../../fork-deltas.md) is the contract).
3. **Self-hostable** — a developer/operator can run the platform without a proprietary control plane.
4. **Secure by default** — production defaults minimize accidental exposure.
5. **Observable** — operators can determine whether the system is healthy and why it is degrading.
6. **Recoverable** — database failure, restart, backup restoration, and migration failure have defined behavior.
7. **Upgrade-safe** — application and database upgrades have explicit compatibility and recovery paths.
8. **Predictable under load** — resource limits and saturation behavior are bounded and measurable.

## 3. Architecture principles

**P1 — PostgreSQL is a first-class dependency.** Not a swapped driver. The architecture explicitly accounts for:

- **Connection pooling**: two pools — `dataDB` (default 80 open / 15 idle) and `auxDB` (default 10 / 3). Single-instance app ceiling = 90 connections, below stock `max_connections=100`. Effective single-instance math: `(instances × pool) + other clients < PG max_connections`, leaving room for admin/maintenance connections.
- **Timeouts** (layered): client-side write deadline + query timeout (default 30s, `core/db_retry.go`), server backstop via role-level `statement_timeout 60s` / `lock_timeout 30s` (`core/db_connect.go`, PgBouncer-safe because applied at the role), `connect_timeout 10s` fail-fast dial, connection recycle (`ConnMaxLifetime 30m`, `ConnMaxIdleTime 3m`).
- **Transactions**: multi-step writes use a transaction; `withWriteDeadline` bounds client writes; role settings bound server execution.
- **Migrations**: PostgreSQL-only DDL in `migrations/`, auto-applied at boot under `pg_advisory_xact_lock`; user Go/JS migrations in `pb_migrations`.
- **Schema ownership**: every collection = a real PostgreSQL table; `SyncRecordTableSchema` reconciles JSONB field definitions.
- **Backup/restore**: native `pg_dump -Fc` / `pg_restore --clean --no-owner --no-privileges`; legacy SQLite import for migration.
- **Version compatibility**: PG 16+ supported; pg_dump/pg_restore client major ≥ server major.
- **PgBouncer**: transaction pooling requires the pgx client setting `default_query_exec_mode=exec|simple_protocol` (named prepared statements do not survive backend changes); role-level timeouts still hold.
- **Failure/reconnection behavior**: `connect_timeout` bounds dials; the outbox `LISTEN` connection is a dedicated pgx conn (not pooled, not PgBouncer transaction mode).

**P2 — Preserve application semantics.** Changing the persistence layer must not unintentionally change: auth behavior, authorization, filtering, sorting, pagination, API response structure, HTTP status semantics, realtime behavior, file behavior, validation behavior. Enforced by the compatibility contract in `fork-deltas.md` and G-API guard rails.

**P3 — Fail boundedly.** Every potentially blocking external operation has a bounded lifecycle: HTTP read/write timeouts (5 min server defaults), query timeout (30s), statement timeout (60s), lock timeout (30s), connection acquisition timeout (`connect_timeout=10s`), graceful shutdown, realtime connection idle timeout (5 min).

**P4 — Data correctness takes priority over availability.**

```text
Corrupt data  <  Temporary request failure
```

The system prefers rejecting or timing out an operation over committing invalid or partially persisted state.

**P5 — Operational behavior is part of the product.** Startup, shutdown, migration, backup, restore, failure, observability, and security posture are product properties, not afterthoughts. See [Observability](./observability.md), [Disaster Recovery](../../deployment/disaster-recovery.md), and [Failure Analysis](./failure-modes.md).

## 4. Core system invariants

Invariants are split by boundary. Each one names where it is enforced. Violations are release-blocking (see [Evaluation Checklists](./checklists.md)).

### 4.1 Data invariants

```text
I1. A failed transaction must not leave a partial transaction state.
I2. A successful write must satisfy all relevant database/application invariants.
I3. Destructive schema changes must be explicit and migration-controlled.
I4. Database schema state must be knowable and reproducible.
I5. Backup data considered valid must be restorable.
```

Enforcement anchors: `core/db_tx.go` (transaction wrapper), `migrations/` (versioned DDL + `_migrations` history), `core/migrations_runner.go` (advisory-lock serialized, transactional), `core/backup_pg_import.go` (restore path).

### 4.2 API invariants

```text
I6. API behavior must remain compatible with the declared compatibility contract.
I7. Breaking behavior must be explicit, documented, and versioned where necessary.
I8. HTTP status codes and error semantics must be deterministic.
```

Enforcement anchors: `docs/fork-deltas.md` (the contract), G-API guardrails, `apis/router` error semantics.

### 4.3 Security invariants

```text
I9.  Secrets must not be committed to source control.
I10. Production database access must not be unintentionally public.
I11. Metrics endpoints must not expose sensitive information publicly.
I12. Production processes should run with least privilege.
```

Enforcement anchors: `.gitignore`, `docker-compose.prod.yml` (DB never published), metrics listener loopback-guard (`PB_METRICS_EXPOSE`), non-root image, G-SEC guard rails.

### 4.4 Resource invariants

```text
I13. Database connections are bounded.
I14. Long-running operations have explicit timeouts.
I15. Realtime resource usage is bounded or observable.
I16. Shutdown releases resources predictably.
I17. Cold-start determinism: applying migrations to an empty database is
     single-connection and deterministic (no cross-connection catalog DDL).
     Observation: core/coldboot_migration_test.go (Bootstrap + RunAllMigrations
     on a truly empty DB); guard rails G-DB-09/G-DB-10; proven 2026-09-12 in
     FAILURE-MODES W-10.
```

Enforcement anchors: `SetMaxOpenConns` (data 80 / aux 10), layered timeouts above, realtime subscriber queue (32) with drop counters, graceful shutdown on SIGINT/SIGTERM.

## 5. Data flow model

For every critical operation:

```text
Request → Authentication → Authorization → Validation → Business logic
        → Transaction boundary → PostgreSQL → Commit → Response / Realtime event
```

**Critical design question: at what point does the system consider the operation successful?** For database-backed operations the answer is tied to a successful transaction commit, not to an in-memory operation. This is material for the realtime outbox (publish after commit — currently a known weakness, see [Failure Analysis](./failure-modes.md) W-01) and for audit writes (guarded by SAVEPOINT inside the record transaction).

## 6. Failure-mode model

Failure modes and their expected behavior are codified in [Failure Analysis](./failure-modes.md), including the A–E failure classification (implementation / design / missing guard rail / missing observability / incorrect acceptance criteria). When a test fails, classify the failure there **before** changing code.

## 7. Deep dives

- [System Overview](../../architecture/overview.md) — components, layers, flowchart
- [Backend Layers](../../architecture/backend-layers.md) — bootstrap chain, dual-pool, middleware order, migrations runner
- [Audit Trail](../../architecture/audit-design.md) — audit/read-partitioning details
- [Fork Deltas](../../fork-deltas.md) — the public compatibility contract
- [Quality Guardrails](./guardrails.md) — the never-allowed constraints that operationalize this model

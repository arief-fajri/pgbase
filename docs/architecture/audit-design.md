# Architecture: Audit Trail Design

<DocMeta audience="Contributor" status="stable" verified="v0.5.2 (923e860)" />

## 1. Two tables, one concern: write must never wait for read

The audit system is split into `_audits` (write trail) and `_audit_reads` (read trail). This is **not** normalisation — it is a deliberate isolation of two workloads with fundamentally different latency and volume profiles.

| Aspect | `_audits` (write trail) | `_audit_reads` (read trail) |
|---|---|---|
| Events | `create`, `update`, `delete` | `view`, `list` |
| Content | Per-field diffs (`changes` JSONB) + full record `snapshot` | Metadata only: filter/sort/page/total — **never record content** |
| Write path | Synchronous, best-effort INSERT (SAVEPOINT-guarded in tx) | Asynchronous batched writer (buffered channel, 3 s flush, 200 rows/batch) |
| Blocking | INSERT may briefly block the caller | Never blocks — events **dropped** if buffer full (capacity 500) |
| Retention | `Audit.RetentionDays` | `Audit.ReadRetentionDays` (independently configurable) |
| Redaction | Hidden/password fields stripped from snapshot + diffs | N/A (no content stored) |

## 2. Why not one table?

### 2a. Volume asymmetry

A busy app may perform 10–100 writes/second but thousands of reads/second. Merging both into a single table would mean:

- Write audit INSERTs compete for the same buffer/page space as read audit INSERTs, increasing write amplification on the hot write path.
- Retention cleanup (partition drops) would be forced into a single policy — you cannot expire read logs aggressively while keeping write logs longer.

### 2b. Write-path guarantees

Write audits are inserted **synchronously** after the record operation commits (`core/audit_hooks.go:57–98`). Inside a transaction, the INSERT runs inside a `SAVEPOINT pb_audit` (`core/audit_hooks.go:276–302`) so a failure rolls back only the audit, not the data change. This guarantee is only possible because the write audit path is simple: one INSERT, no batching, no channel.

Read audits use a **batched async writer** (`core/audit_writer.go:30–154`): events are enqueued to a buffered channel (capacity 500) and flushed every 3 seconds or per 200 rows via `RunInTransaction`. If the buffer is full, the event is dropped with a warning log — a read must **never** block or fail the request handler (`core/audit_hooks.go:117–145`).

Coupling these two write paths into one table would force the read-path drop-on-full semantics onto the write path, or vice versa. Neither is acceptable.

### 2c. Index and query divergence

Write audits are queried by `(collection_name, record_id, created DESC)` — the compound index `idx_audits_coll_rec` supports per-record history lookups and the `changes.*` JSONB path filters.

Read audits are queried by `(collection_name, created DESC)` with free-text search on the `filter` string. The index `idx_audit_reads_coll` is simpler because there is no per-record lookup pattern.

A single table would need a superset of indexes, increasing write amplification and vacuum pressure on the already-hot read-audit volume.

### 2d. Partition lifecycle

Both tables are RANGE-partitioned by month on `created`, but their retention is independent:

```go
// core/audit_hooks.go:184–200
if s.RetentionDays > 0 {
    dropOldAuditPartitions(app, AuditsTableName, s.RetentionDays)
}
if s.ReadRetentionDays > 0 {
    dropOldAuditPartitions(app, AuditReadsTableName, s.ReadRetentionDays)
}
```

Read logs can be retained for 7 days while write logs keep 90. A single table forces one retention policy.

## 3. Write trail internals (`_audits`)

### Schema

```sql
-- migrations/1787237001_audits_init.go:9–46
CREATE TABLE IF NOT EXISTS "_audits" (
    "id"              TEXT DEFAULT ('r'||lower(encode(gen_random_bytes(7),'hex'))) NOT NULL,
    "collection_name" TEXT NOT NULL,
    "record_id"       TEXT NOT NULL,
    "event"           TEXT NOT NULL,
    "auth_id"         TEXT DEFAULT '' NOT NULL,
    "auth_collection" TEXT DEFAULT '' NOT NULL,
    "source"          TEXT DEFAULT '' NOT NULL,
    "changes"         JSONB DEFAULT '{}'::jsonb NOT NULL,
    "snapshot"        JSONB DEFAULT '{}'::jsonb NOT NULL,
    "user_ip"         TEXT DEFAULT '' NOT NULL,
    "user_agent"      TEXT DEFAULT '' NOT NULL,
    "created"         TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    PRIMARY KEY ("id", "created")
) PARTITION BY RANGE ("created");
```

### Hook execution order

1. `OnRecord{Create,Update,Delete}Request` — stores the actor (auth_id/ip/ua) keyed by `*Record` pointer in `auditActors` (`core/audit_hooks.go:100–115`).
2. `OnRecord{Create,Update,Delete}Execute` — after `e.Next()` commits:
   - Top-level writes: plain autocommit INSERT via `NonconcurrentDB()`.
   - In-transaction writes: SAVEPOINT-wrapped INSERT so failure cannot poison the outer PG transaction (`core/audit_hooks.go:265–302`).

### Redaction

`redactFields()` (`core/audit_hooks.go:304–319`) strips hidden and password fields from both `snapshot` and `changes` before insertion. Secrets never reach the audit trail.

## 4. Read trail internals (`_audit_reads`)

### Schema

```sql
-- migrations/1787237001_audits_init.go:50–74
CREATE TABLE IF NOT EXISTS "_audit_reads" (
    "id"              TEXT DEFAULT ('r'||lower(encode(gen_random_bytes(7),'hex'))) NOT NULL,
    "collection_name" TEXT NOT NULL,
    "record_id"       TEXT DEFAULT '' NOT NULL,
    "event"           TEXT NOT NULL,
    "auth_id"         TEXT DEFAULT '' NOT NULL,
    "auth_collection" TEXT DEFAULT '' NOT NULL,
    "source"          TEXT DEFAULT 'request' NOT NULL,
    "filter"          TEXT DEFAULT '' NOT NULL,
    "sort"            TEXT DEFAULT '' NOT NULL,
    "page"            INTEGER DEFAULT 0 NOT NULL,
    "per_page"        INTEGER DEFAULT 0 NOT NULL,
    "total_items"     INTEGER DEFAULT 0 NOT NULL,
    "user_ip"         TEXT DEFAULT '' NOT NULL,
    "user_agent"      TEXT DEFAULT '' NOT NULL,
    "created"         TIMESTAMPTZ DEFAULT NOW() NOT NULL,
    PRIMARY KEY ("id", "created")
) PARTITION BY RANGE ("created");
```

### Batched writer (`core/audit_writer.go`)

```text
Request handler → enqueue(AuditRead) → buffered channel (500)
                                            ↓
                              ticker (3 s) or explicit Flush()
                                            ↓
                              writeBatch() → RunInTransaction → INSERT 200 rows
```

- `enqueue()` is non-blocking: `select { case buf <- r: default: drop }`.
- `Flush()` drains the channel and writes in batches of 200 via `app.RunInTransaction()` on the MAIN pool.
- `start()` launches the ticker goroutine on `OnServe`; `stop()` flushes and closes on `OnTerminate`.
- Tests call `FlushAuditReads(app)` for deterministic draining without relying on the ticker.

### Self-exclusion

Both audit tables exclude themselves and each other from auditing (`core/audit_hooks.go:440`), preventing infinite recursion.

## 5. Partition management

Both tables use native PostgreSQL RANGE partitioning by month on `created` with a mandatory `DEFAULT` partition so inserts never fail before the monthly cron creates a named partition.

| Cron job | Schedule | Action |
|---|---|---|
| `__pbAuditsPartition__` | `0 0 * * *` | Pre-creates next 2 monthly partitions for both tables |
| `__pbAuditsCleanup__` | `0 0 * * *` | Drops named partitions older than retention days |

Partition creation is idempotent (`IF NOT EXISTS`). The DEFAULT partition catches any inserts that land before the named partition exists.

## 6. API surface

| Endpoint | Handler | Table | Notes |
|---|---|---|---|
| `GET /api/audits` | `auditsList()` | `_audits` | Omits `snapshot` in list for perf; filterable by `changes.*` paths |
| `GET /api/audits/{id}` | `auditsView()` | `_audits` | Returns full audit including snapshot |
| `GET /api/audits/reads` | `auditReadsList()` | `_audit_reads` | Search/filter on metadata fields |

All endpoints are superuser-only (`apis/audits.go:12–34`).

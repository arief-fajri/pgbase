# Observability

<DocMeta audience="Contributor" status="stable" verified="v0.5.4" />

> Observability answers *what is actually happening inside the system*. The objective is not to collect every metric — it is to make important system behavior *explainable*, and to prove that guardrails hold.
>
> Enabling and scraping guidance: [Production Runbook §8](../../deployment/production.md#8-monitoring-prometheus).

## 1. HTTP — RED model

For major API surfaces, observe:

- **Rate** — requests/sec
- **Errors** — 4xx/sec, 5xx/sec, error ratio
- **Duration** — p50, p95, p99

Dimensions: endpoint (matched route template), method, status code, application version. **Avoid uncontrolled high-cardinality labels** (raw user IDs, arbitrary URLs).

Current state: a request **duration histogram** (`pgbase_http_request_duration_seconds`) and a per-status **request counter** (`pgbase_http_requests_total`, same `method`/`route`/`status` labels) exist. Error ratio: `sum(rate(pgbase_http_requests_total{status=~"5.."}[5m])) / sum(rate(pgbase_http_requests_total[5m]))` (or derive it from the histogram count series). Deployed alert: `p99` rise (see `deploy/prometheus.rules.yml`).

## 2. PostgreSQL / pool metrics

```text
DB connections: open · in use · idle · maximum · wait count · wait duration
DB behavior:    query timeout · lock timeout · transaction rollback   ← counters below
DB behavior:    connection failure / reconnect                        ← not exposed by database/sql (Phase 4 note below)
```

Existing (`db` label = `data` | `aux`):

| Metric | Type | Meaning |
|---|---|---|
| `pgbase_db_open_connections` | gauge | live open connections |
| `pgbase_db_in_use_connections` | gauge | connections in use |
| `pgbase_db_idle_connections` | gauge | idle connections |
| `pgbase_db_max_open_connections` | gauge | pool ceiling (0 = unlimited) |
| `pgbase_db_wait_count_total` | counter | acquisitions that had to wait — **pool saturation signal** |
| `pgbase_db_wait_duration_seconds_total` | counter | time blocked waiting on the pool |
| `pgbase_db_query_timeout_total` | counter | queries aborted by a timeout (client deadline or server `statement_timeout` 57014) — classified on the record/model paths |
| `pgbase_db_lock_timeout_total` | counter | statements aborted by the server `lock_timeout` (55P03) — classified on the record/model paths |
| `pgbase_db_tx_rollback_total` | counter | top-level transactions that rolled back (nested reuse the outer one) |

Remaining gap ([Phase 4](../roadmap.md)): reconnect events are NOT counted — the `database/sql` layer does not expose them (pool dynamics are visible via `wait_count`/`open_connections`); the outbox LISTEN reconnect belongs to realtime observability. Raw builder queries (outside the record/model paths) are not classified.

**Diagnostic chain** (use when latency or errors appear):

```text
Latency increase → DB wait increase? → Pool saturation? → Slow query? → Lock contention? → External PostgreSQL issue?
```

Deployed alert to start with: `rate(pgbase_db_wait_count_total[5m]) > 0` (pool saturation), `production.md` §8.

## 3. Realtime metrics

Existing:

| Metric | Type | Meaning |
|---|---|---|
| `pgbase_realtime_connected_clients` | gauge | connected SSE clients |
| `pgbase_realtime_dropped_messages` | gauge | messages dropped on full client buffers (queue 32) |

Gaps ([Phase 4](../roadmap.md) realtime outbox): active subscriptions, connection/disconnection rate, broadcast latency, outbox lag, abnormal reconnect behavior. Realtime resource usage must stay bounded **or observable** (invariant I15) — today the drop counter is that bound.

## 4. Backup and recovery metrics

Required signal set (framework §6.4):

```text
backup attempt count · success/failure · duration · size
last successful backup · last successful restore verification
```

Existing: `pgbase_backup_attempts_total`, `pgbase_backup_success_total`, `pgbase_backup_failure_total`, `pgbase_backup_duration_seconds` (sum; average = / attempts), `pgbase_backup_last_size_bytes` (gauge, last successful archive), `pgbase_backup_last_verified_timestamp_seconds` (gauge; unix timestamp of the last restore that passed the verification gate — G-REL-04; **0 = never verified**, alert on `time() - this > your drill cadence`). The timestamp is persisted to `_params` (`last_verified_backup`, RFC3339 UTC) by the restore path and reloaded at boot; the drill that feeds it is `evidence/experiments/EXPERIMENT-E.sh`. See the [disaster-recovery checklist](../../deployment/disaster-recovery.md).

## 5. Guard-rail → metric correlation

The metrics above exist to prove guard rails. Map (existence = observable):

| Guard rail | Proving metric | Status |
|---|---|---|
| G-DB-08 pool exhaustion observable | `pgbase_db_wait_count_total`, `wait_duration` | ✅ |
| G-REL-01 no call blocks forever | `pgbase_db_query_timeout_total`, `pgbase_db_tx_rollback_total` | ✅ |
| I15 realtime bounded/observable | `pgbase_realtime_dropped_messages` | ✅ |
| G-REL-04 backup valid only if restored | `pgbase_backup_last_verified_timestamp_seconds` | ✅ |
| G-API-02/03 status/schema stable | HTTP error-ratio + version label | ⚠️ partial |

A subsystem that gains a new guard rail must also gain the metric that makes the rail observable — that pairing is part of the implementation loop.

## 6. Metrics vs dashboards

Metrics support **diagnosis**, not merely dashboards. When a metric exists but cannot answer "why is it degrading", that is a missing-observability failure (class D in [Failure Analysis](./failure-modes.md)) — add the diagnostic view, not another chart. Alert first on pool waits and p99, using the operator's Prometheus. An optional compose file is documented in the [production runbook](../../deployment/production.md#8-monitoring-prometheus). PG-BASE does not ship default Grafana dashboards or Alertmanager rules.

## 7. Log & audit streams (what to watch)

| Stream | Table / API | Notes |
|---|---|---|
| Request logs | `_logs` on **aux** DB (`core/log_model.go:9`), `GET /api/logs` + `/logs/stats` (`apis/logs.go:13,39`) | Plain table; retention `__pbLogsCleanup__ 0 */6 * * *` (`core/base.go:1611`); bulk DELETE bloat; range-partition retention is [Phase 4](../roadmap.md) |
| Data audits | `_audits` (+ `_audit_reads` metadata-only), `GET /api/audits` (+ `/{id}`, `/reads`) superuser-only (`apis/audits.go:12-34`) | Month-RANGE partitioned + DEFAULT (`migrations/1787237001_audits_init.go:25-67`); next-2-months pre-created (`core/audit_hooks.go:455-475`); retention jobs `__pbAuditsPartition/Cleanup__ 0 0 * * *`; changes/snapshot redacted |
| Dashboard | `#/logs` (chart/list/preview) + `#/audits` (Data-changes / Read-access tabs) + `#/settings/audit` | v0.2.0 audit UI |

**W-09** ([Failure Analysis](./failure-modes.md) §2): the `_audits` DEFAULT partition is never auto-pruned — partition retention assumes pre-created partitions are actively used. Watch for unbounded growth on the DEFAULT partition. Pruning it is [Phase 4](../roadmap.md).

# Upstream Status

<DocMeta audience="Contributor" status="living document" verified="v0.5.2 (923e860)" />

> Live status of the PG-BASE → PocketBase relationship. This is the thin snapshot doc; the **policy** is `FORK_STRATEGY.md` in the repo root (hard-fork decision, security SLA, adopt/skip/diverge rules) and the **contract** is [fork-deltas.md](./fork-deltas.md). This page answers only: *which upstream are we on, and what currently diverges?*

## 1. Current upstream

| Item | Value |
|---|---|
| Baseline | **PocketBase v0.39.11** (initial commit `0b3dac2`, 2026-08-13) |
| Upstream status | has since moved to the v0.40.x line |
| Tracking process | watch → triage → act; recorded in `CHANGELOG.md` "Upstream tracking" notes |
| Security SLA | Critical 48h triage / ≤7d patch; see FORK_STRATEGY §5 |

## 2. Divergence map

| Area | Nature | Anchor |
|---|---|---|
| Storage engine | pgx v5 + forked `dbx` — **replaces** SQLite layer | `core/db_connect.go`, `third_party/dbx/` |
| System migrations | PostgreSQL-native DDL (JSONB, timestamptz, partitions, pgcrypto, outbox) | `migrations/` |
| Audit trails | `_audits` / `_audit_reads`, month-partitioned | `core/audit_hooks.go` |
| Backups | native `pg_dump`/`pg_restore` + legacy SQLite import | `core/backup_pg_*.go` |
| Security hardening | SSRF guard, download caps, identifier quoting, pinned CI/digests | `tools/security/`, `.github/workflows/` |
| Scale/ops | realtime outbox (opt-in), Prometheus metrics, pool tuning | `core/realtime_outbox.go`, `apis/metrics.go` |
| Behavior deltas | the 9 documented public-API deltas | [fork-deltas.md](./fork-deltas.md) |
| Tests | database-per-test harness | `tests/app.go` |
| Router surface enumeration | additive `RouterGroup.Routes()` (no upstream equivalent); backs the API-compat baseline | `tools/router/group.go`, `apis/api_surface_baseline_test.go` |
| Single-bucket DB access | additive `App.RunInSingleTx` (no upstream equivalent): data + aux operations share one pool connection/transaction; required by single-connection migrations (G-DB-10), so DDL never interleaves across connections | `core/db_tx.go`, `core/migrations_runner.go` |

**Rule (FORK_STRATEGY §3):** every intentional divergence must be (a) documented in `fork-deltas.md` when it affects the public surface, (b) covered by a test, and (c) noted in the porting checklist when we touch the same file for an upstream backport.

## 3. What this page is not

This is not the place to record *decisions* (that is FORK_STRATEGY) or *behavior differences customers must read* (that is fork-deltas). Update the table above when the baseline moves or a divergence is adopted; keep the details one-link away.

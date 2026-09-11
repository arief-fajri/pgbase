# Fork Strategy — Upstream Tracking & Divergence Policy

> Decision record · Decided: 2026-09-11 · Owner: maintainer · Review cadence: every roadmap phase boundary, or immediately on any revisit trigger (§7)
>
> Related: [`.github/SECURITY.md`](/.github/SECURITY.md) (vulnerability routing), [`docs/fork-deltas.md`](/docs/fork-deltas.md) (the compatibility contract), [`docs/contributing-releasing.md`](/docs/contributing-releasing.md) §5 (fork duty).

## 1. Decision

**PG-BASE stays a hard fork.** Upstream PocketBase is tracked through a disciplined *watch → triage → act* process (§4) with a security backport SLA (§5) and explicit adopt/skip/diverge rules (§6) — not through rebases, sync branches, or a build-tag overlay.

Baseline: forked from upstream **PocketBase v0.39.11** (initial commit `0b3dac2`, 2026-08-13). Upstream has since moved to the v0.40.x line; the divergence below is the price of the product.

## 2. Alternatives considered

### 2a. Build-tag overlay — rejected (for now)

An overlay (à la `statewright/pg-pocketbase`) keeps the upstream tree pristine and adds PG support behind build tags, so upstream merges stay near-free. It is structurally the strongest anti-drift architecture, and it is honest to say it may become the category default.

Why rejected today: our divergence is **replacement, not addition**. The storage engine (pgx v5 + the `third_party/dbx` fork), all 16+ system migrations, the test harness (~217 files, database-per-test), the backup subsystem, and audit trails *rewrite* upstream code rather than sit beside it. Converting to an overlay now means re-doing the port — months of work to *reduce* future cost that disciplined triage already bounds. The conversion only pays off if upstream's change rate is high and our surface stays narrow; neither holds.

### 2b. Periodic rebase / formal sync contract — rejected

A rebase model assumes shared history. The fork intentionally reset history (upstream changelog dropped at v0.1.0), and every "sync" would be a manual port regardless — a contract adds process without reducing port cost. Hard fork + triage log gives the same auditability with less ceremony.

### 2c. Hard fork + tracking discipline — chosen

Matches reality: the PG engine *is* the product, not a feature flag. Cost is bounded by triage severity — security fixes are backported within SLA, bugs selectively, features by deliberate decision.

## 3. Divergence inventory (what must not drift silently)

| Area | Nature | Anchor |
|---|---|---|
| Storage engine | pgx v5 + forked dbx — **replaces** SQLite layer | `core/db_connect.go`, `third_party/dbx/` |
| System migrations | PostgreSQL-native DDL (JSONB, timestamptz, partitions, pgcrypto, outbox) | `migrations/` |
| Audit trails | `_audits` / `_audit_reads`, month-partitioned | `core/audit_hooks.go`, `core/audit_writer.go` |
| Backups | native `pg_dump`/`pg_restore` + legacy SQLite import | `core/backup_pg_*.go`, `core/backup_sqlite_import.go` |
| Security hardening | SSRF guard, download caps, identifier quoting, pinned CI/digests | `tools/security/`, `.github/workflows/` |
| Scale/ops | realtime outbox (opt-in), Prometheus metrics, pool tuning | `core/realtime_outbox.go`, `apis/metrics.go` |
| Behavior deltas | the 9 documented public-API deltas | `docs/fork-deltas.md` |
| Tests | database-per-test harness, ~217 files | `tests/app.go` |

**Rule:** every intentional divergence must be (a) documented in `docs/fork-deltas.md` when it affects the public surface, (b) covered by a test, and (c) noted in the porting checklist when we touch the same file for an upstream backport.

## 4. Upstream tracking process

On each upstream release (watch: PocketBase releases feed, GitHub advisories for `pocketbase/pocketbase`, Go/npm advisories for shared dependencies):

1. Read the release notes; classify every change: `SECURITY` / `BUG` / `FEATURE` / `BREAKING` / `INTERNAL`.
2. Act per class:
   - `SECURITY` → backport per the SLA (§5).
   - `BUG` → backport **only if** it applies to a code path we still share (SQLite-only paths are usually N/A — say so in the log).
   - `FEATURE` → adopt/skip/diverge decision (§6).
   - `INTERNAL` / refactor → skip unless it touches files we regularly backport into.
3. Record the outcome in `CHANGELOG.md` under an "Upstream tracking" note (upstream version, what was triaged, decisions) so the next audit has a trail.

**Porting rule:** port the *fix*, not the diff. Re-implement against our tree, run the relevant test package, and add a regression test when the bug could recur. Never blind-merge upstream files.

## 5. Security triage & backport SLA

| Severity (impact) | Triage within | Fix target |
|---|---|---|
| Critical — RCE, auth bypass, data exposure, supply chain | 48 h | patched release ≤ 7 days |
| High | 5 days | ≤ 30 days or next release |
| Medium / Low | next review window | next scheduled release |

- Intake and routing of reports: [`.github/SECURITY.md`](/.github/SECURITY.md).
- Upstream CVEs that also affect us: advisory + backport + release notes crediting upstream.
- CVEs in code we deleted (SQLite paths): mark **N/A** in the triage log — do not silently ignore.
- Scheduled `govulncheck` + `npm audit` (CI) cover the dependency surface between releases.

## 6. Adopt / skip / diverge rules (features & behavior changes)

- **ADOPT** — compatible with the PG-only premise, no REST API divergence, effort ≤ M.
- **SKIP** — SQLite-specific, or conflicts with a documented fork delta. `docs/fork-deltas.md` is the contract and wins by default.
- **DIVERGE deliberately** — only when a PostgreSQL-native approach is measurably better **and** the REST contract is preserved; the same PR must update `docs/fork-deltas.md` and add tests.
- **BREAKING upstream changes** — never followed blindly. API compatibility is a product feature; hold the previous API until a coordinated major version.

## 7. Revisit triggers

Re-open this decision (toward an overlay or sync model) when **any** of:

1. Upstream security-release cadence exceeds our backport capacity two releases in a row.
2. An overlay-based rival demonstrably holds parity with < 1 week lag across 3 consecutive upstream releases.
3. Actual maintenance cost (§8) exceeds 2× the estimate for 2 consecutive releases.

Otherwise the decision stands through at least Phase 2 of the roadmap.

## 8. Maintenance-cost estimate

Based on the 2026-09-08 divergence audit (`core/` `apis/` `forms/` `tools/` `ui/` are all heavily modified; ~70k LOC non-test):

| Upstream release type | Estimated effort |
|---|---|
| Patch / security release | 2–8 h (triage + targeted backport + tests) |
| Minor release | 8–24 h (full triage, selective backport, regression pass) |
| Major / breaking release (schema or API churn) | 3–5 days on a dedicated branch + compatibility review vs `docs/fork-deltas.md` |

Steady state: assume ~monthly upstream releases → budget **≈ 1–2 days/month** sustained. This is the recurring cost the decision in §1 accepts.

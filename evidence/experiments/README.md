# Controlled failure experiments — harness (gate E5)

Scenarios defined in [FAILURE-MODES.md §4](../../docs/FAILURE-MODES.md#4-controlled-failure-experiments).
Harness scripts make each scenario **bounded, reproducible, idempotent**, and log
raw evidence for L0–L3 review. They are run **by a human operator** (they stop/start
the shared test PostgreSQL container and create/drop dedicated `pgbase_exp_*` databases).

**Status (2026-09-12):** gate E5 executed once, all five PASS (L3 first runs) —
[EXPERIMENT-A](EXPERIMENT-A-20260912-161420.md), [B](EXPERIMENT-B-20260912-154253.md),
[C](EXPERIMENT-C-20260912-155209.md), [D](EXPERIMENT-D-20260912-155751.md),
[E](EXPERIMENT-E-20260912-160628.md). Raw stdout logs are **gitignored**
(`evidence/experiments/logs/`); regenerate them by re-running any harness.

## Prerequisites

```bash
docker compose -f tests/docker-compose.test.yml up -d --wait   # PostgreSQL 16 on :5433
go build -o pgbase ./examples/base                              # target binary
pg_dump / pg_restore / pg_isready / psql 16+ on PATH            # experiments E, D, all
```

Runnable without the Docker DB? only the `-n` syntax check below.

## Run an experiment

```bash
bash evidence/experiments/EXPERIMENT-A.sh     # outage -> recovery
bash evidence/experiments/EXPERIMENT-B.sh     # pool saturation (Go-free bounded harness)
bash evidence/experiments/EXPERIMENT-C.sh     # lock contention timeout
bash evidence/experiments/EXPERIMENT-D.sh     # migration failure + state replay
bash evidence/experiments/EXPERIMENT-E.sh     # backup -> destroy -> restore -> verify
```

Exit code 0 = `RESULT: PASS`; non-zero = `RESULT: FAIL`. Every run writes
`logs/<SESSION>-<ts>.log` (laydown) plus per-probe artifacts under `logs/exp-*/`.

## Evidence policy (evidence/README.md)

- **L0** raw artifacts: log file per run in `logs/`.
- **L1** an operator reads the L0 summary (`RESULT:` + key numbers) — routine level.
- **L2** a reviewer (human or agent, attacks the claim) signs the L2 block — **required for
  every first run** and every harness change after qualification.
- **L3** the result is **cross-checked by an independent method** — **required for first runs
  that underpin correctness claims** (all five here). Examples of independent methods:
  - A: `pg_stat_activity`/`pg_stat_database` counters before/after; nginx/HAProxy-side view.
  - B: second, independent probe (pgbench against the same pool budget); Grafana on wait metrics.
  - C: `pg_locks` snapshot proves the block is the external ACCESS EXCLUSIVE lock, not a hang.
  - D: independent schema fingerprint diff (`pg_dump --schema-only` vs pre-failure baseline);
    `_migrations` row count and `hash` values after replay.
  - E: `pg_restore --list` archive contents vs actual objects; login as the restored superuser
    (auth-with-password) — an independent check from `psql` counts.

## Finalizing a run (title the claim)

Per run: copy [`EXPERIMENT-template.md`](./EXPERIMENT-template.md) to
`EXPERIMENT-<ID>-<YYYYMMDD>-<run>.md`, fill Verdict + L2/L3 blocks, and update the
`Verdict` column in [FAILURE-MODES.md §4](../../docs/FAILURE-MODES.md#4-controlled-failure-experiments)
and this line in [`evidence/learnings.md`](../learnings.md).

Environment overrides (all optional):

| Variable | Default |
|---|---|
| `PB_EXP_PGBASE` | `<repo>/pgbase` |
| `PB_EXP_PGHOST` | `localhost` |
| `PB_EXP_PGPORT` | `5433` |
| `PB_EXP_PGUSER` / `PB_EXP_PGPASS` | `test` / `test` |
| `PB_EXP_PGSSLMODE` | `disable` |
| `PB_EXP_COMPOSE_PROJECT` | `tests` (the directory holding `tests/docker-compose.test.yml`) |
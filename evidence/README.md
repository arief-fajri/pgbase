# Evidence

> **An experiment without a traceable artifact is not evidence (G-AI-07).**
> This directory holds the artifacts that prove the [desired outcomes](./docs/contributor/methodology/platform-design.md#2-desired-outcomes)
> hold and the [guardrails](./docs/contributor/methodology/guardrails.md) are enforced. Claims like "restore works",
> "migrations fail safely", or "pool saturation is bounded" are only true here,
> with a dated, versioned record.

## Layout

```text
evidence/
├── experiments/     # failure-experiment records A–E (template below)
├── records/         # Decision Request Records (B1/B2) — decision number = GitHub issue number
└── learnings.md     # cross-session learning log (10-step DoD step 10 lands here)
```

## Evidence verification levels (L0–L3)

| Level | Verification | Who |
|---|---|---|
| **L0** | Script completes (exit 0) | automation |
| **L1** | + A human reads the summary before "verified" is recorded | routine default |
| **L2** | + Adversarial review: does the artifact actually prove the claim (not a tautology)? | first run of any experiment + every harness change |
| **L3** | + Independent cross-check against a second method | correctness-critical claims (e.g. restore validity, no corruption) |

- First run of any experiment = **L2 minimum**; correctness-critical first runs = **L3**.
- Drills can drop to L1 once qualified; re-qualify (L2/L3) whenever the harness or the environment (e.g. PostgreSQL major version) changes.

## Producing a record

1. Copy the matching template (`experiments/EXPERIMENT-template.md` or `records/DRR-template.md`).
2. Fill frontmatter fields (date, version, env, guard rails, verdict, level).
3. Attach raw output (log, SQL transcript, metrics snapshot) alongside — raw harness logs live under `experiments/logs/` (gitignored, regenerable); the committed artifact is this record plus any captured transcript you embed inline.
4. Record the verdict in the matrix in `docs/FAILURE-MODES.md` §4.
5. Append lessons to `learnings.md`.
6. Commit together with the code that produced it.
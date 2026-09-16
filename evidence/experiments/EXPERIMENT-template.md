# EXPERIMENT <A|B|C|D|E> — <short title>

<!--
Template: copy to evidence/experiments/EXPERIMENT-<ID>-<YYYYMMDD>-<run>.md
Scenario descriptions: docs/FAILURE-MODES.md §4
The FAILURE-MODES §4 verdict matrix must be updated when this record is finalized.
-->

## Frontmatter

- **Experiment:** A / B / C / D / E (outage / pool saturation / lock contention / migration failure / backup restoration)
- **Date:** YYYY-MM-DD
- **PG-BASE version:** <commit/tag>
- **PostgreSQL version:**
- **Topology:** single/multi-instance; PgBouncer?; storage local/S3
- **Harness version:** <link/commit of the harness script>
- **Verification level:** L0 / L1 / L2 / L3
- **Guard rails exercised:** G-#
- **Invariants exercised:** I#

## Scenario

<What did the experiment set out to prove? State the claim explicitly.>

## Procedure

<Exact commands and steps. Must be reproducible by an operator who did not author the experiment (G-REL-05).>

## Results

<Raw output attached or inline: exit codes, latency/error numbers, metric snapshots, DB state.>

## Verdict

- [ ] **PASS** — expected behavior observed
- [ ] **FAIL** — <which expected behavior was violated>

## L2/L3 review (for first runs and harness changes)

<Adversarial check: does the artifact actually prove the claim? What could make this pass falsely?
For L3: what independent method cross-checks the result?>

- Reviewer: <name / agent id>
- Date:
- Outcome:

## Learnings

<What did we learn? Append to evidence/learnings.md as well.>
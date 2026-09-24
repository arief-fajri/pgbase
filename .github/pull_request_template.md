## Summary

<What this change does — one paragraph.>

## Definition of Done (10 steps)

1. **System property changed:** <what system property are we changing?>
2. **Desired outcome:** <which PLATFORM.md §2 outcome should change?>
3. **Invariant that must remain true:** <I# reference, PLATFORM.md §4>
4. **Guard rails that apply:** <G-# references, GUARDRAILS.md — say "none crossed" explicitly for Level A changes>
5. **How the behavior is observed:** <metric / diagnostic / log — see OBSERVABILITY.md>
6. **How it is tested:** <test names / packages / commands>
7. **Relevant failure modes:** <FAILURE-MODES.md table row(s), or "none">
8. **Acceptance checklist:** <which CHECKLISTS.md section(s) apply; check the rows>
9. **Evidence it works:** <artifact path in `evidence/`, test output, benchmark — no artifact = no evidence>
10. **Learning recorded:** <what did we learn — appended to `evidence/learnings.md`>

## Decision authority

- [ ] **Level A** — internal, reversible, guard-rail-safe, contract-safe
- [ ] **Level B** (explicit human confirm required) — anything that is not Level A → DRR in `evidence/records/`, blocked until a human confirms

> If this change is in response to a failure, state the **classification (A–E)** before describing the fix (AGENTS.md §failure classification).

## Checklist

- [ ] `go test ./... -count=1 -p 4` and `-race` pass
- [ ] `golangci-lint run -c ./golangci.yml ./...` zero issues
- [ ] Docs updated (`.md` links, DocMeta, sidebar) when behavior or commands changed
- [ ] `make docs-check` clean when docs touched
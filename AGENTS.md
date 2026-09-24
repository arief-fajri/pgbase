# AGENTS.md — instructions for AI coding agents working on this repository

PG-BASE is a self-hosted, single-binary application backend for PostgreSQL. The public behavior contract is `docs/reference/api-contract.md`. Caller-visible changes must be deliberate, tested, and written there. PocketBase is the inspiration for the programming model.

**This file is the single entry point for every AI agent session.** Read it first,
before any other file.

---

## System thinking methodology

PG-BASE follows a system-thinking engineering loop. **You must use it for every non-trivial task.**

```text
PLATFORM DESIGN → GUARD RAILS → IMPLEMENT → OBSERVE → TEST → EVALUATE → RECORD LEARNING
```

Full framework details: 
[docs/contributor/methodology/platform-design.md](docs/contributor/methodology/platform-design.md) (what we are building),
[docs/contributor/methodology/guardrails.md](docs/contributor/methodology/guardrails.md) (what is never allowed),
[docs/contributor/methodology/failure-modes.md](docs/contributor/methodology/failure-modes.md) (classify before you fix),
[docs/contributor/methodology/observability.md](docs/contributor/methodology/observability.md) (prove guardrails hold),
[docs/contributor/methodology/checklists.md](docs/contributor/methodology/checklists.md) (evaluation gates).

### System thinking preflight (before starting non-trivial work)

Answer these **before writing any code**:

1. What system property are we changing?
2. What desired outcome (PLATFORM §2) should change?
3. What invariant must remain true?
4. Which guard rails (GUARDRAILS.md, G-*) apply?
5. How will we observe the behavior?
6. How will we test it?
7. What failure modes are relevant?
8. What is the acceptance checklist?
9. What evidence will prove it works?
10. What did we learn after implementation?

The answers go in the PR description — this is the **10-step Definition of Done**.

### Failure classification — classify before you fix

When a test fails or a runbook step misbehaves, **classify it before changing code**:

| Type | Pattern | Response |
|---|---|---|
| **A — Implementation failure** | Design ✓, guard rail ✓, implementation ✗ | Fix implementation |
| **B — Design failure** | Implementation ✓, design was insufficient | Update platform-design.md |
| **C — Missing guard rail** | System entered a forbidden state | Add a guardrail (guardrails.md) |
| **D — Missing observability** | System failed but operators can't tell why | Add measurement (observability.md) |
| **E — Incorrect acceptance criteria** | Checklist passed but behavior was unsafe | Update checklists.md |

**You may never jump from a failing test directly to a code change.** Record the classification (PR/issue) first.

### Decision authority

Your autonomy is bounded. Respect the levels in [guardrails.md §8](docs/contributor/methodology/guardrails.md#8-ai-decision-authority-g-ai):

| Level | What | Action |
|---|---|---|
| **A — Decide & execute** | Internal, reversible, guard-rail-safe, contract-safe | Execute, record |
| **B — Explicit human confirm** | Everything else: dependency fix, non-contract refactor, API contract, security, destructive | DRR first. Blocked until a human confirms. Silence is not approval. |

**When in doubt: raise the class, never lower it.** You must not downgrade B to A.

### Evidence and learning

- Failure experiments (A–E) produce records in `evidence/experiments/`.
- Decision Request Records (DRR) for Level B decisions go in `evidence/records/`.
- After implementing, append what you learned to `evidence/learnings.md`.
- An experiment without a traceable artifact is not evidence (G-AI-07).

---

## Read first (per task)

- Architecture: [architecture/overview.md](docs/architecture/overview.md), [architecture/backend-layers.md](docs/architecture/backend-layers.md)
- API contract: [api-contract.md](docs/reference/api-contract.md)
- Product roadmap: [roadmap.md](docs/contributor/roadmap.md)
- Full dev guide: [docs/contributor/developing.md](docs/contributor/developing.md) (single source of truth for commands)
- System model & invariants: [docs/contributor/methodology/platform-design.md](docs/contributor/methodology/platform-design.md)
- Guard rails & authority: [docs/contributor/methodology/guardrails.md](docs/contributor/methodology/guardrails.md)

## Build

```bash
go build -o pgbase ./examples/base   # the runnable entrypoint is examples/base
```

The dashboard UI is prebuilt and committed at `ui/dist` (embedded via
`ui/embed.go`). Only rebuild it when changing `ui/src`:

```bash
npm --prefix ui ci && npm --prefix ui run build   # BEFORE go build
```

## Test

Tests need a PostgreSQL 16+ instance. Standard setup:

```bash
docker compose -f tests/docker-compose.test.yml up -d --wait
PB_POSTGRES_HOST=localhost PB_POSTGRES_PORT=5433 PB_POSTGRES_USER=test \
PB_POSTGRES_PASSWORD=test PB_POSTGRES_DBNAME=pgbase_test \
  go test ./... -count=1 -p 4 -timeout=1200s        # same as `make test`
```

- Tests are DB-backed: `tests.NewTestApp()` creates a database per test via `CREATE DATABASE ... TEMPLATE`. `-p 4` is safe; do not use `-p 1`.
- The same suite must pass under `-race` (CI runs a dedicated race job).
- Raw `core.NewBaseApp` tests use the `PB_POSTGRES_*` env above, not `PGTEST_*`.
- `pg_dump`/`pg_restore` must be on PATH (v16+) or the backup round-trip test skips.

## Lint

```bash
golangci-lint run -c ./golangci.yml ./...   # must be zero issues; CI blocks on it
```

Formatting: `gofmt`/`goimports` defaults (alphabetical import order within groups — the v0.5.1 rebrand once broke this; keep the tree clean).

## Docs

- Build the docs site before opening docs PRs: `npm --prefix docs ci && npm --prefix docs run docs:build`.
- Internal doc links must keep the `.md` suffix (e.g. `[roadmap](./roadmap.md)`). VitePress strips it for routing, but the lychee CI check resolves links against the filesystem — extensionless links fail the `docs` workflow.
- `make docs-check` (markdownlint) must stay clean; new pages need a `<DocMeta>` frontmatter line and a sidebar entry in `docs/.vitepress/config.mts`.

## Code layout

- `core/` — domain: app bootstrap, collections, records, fields, auth, audit, backup
- `apis/` — REST routes and middlewares
- `forms/` — request-validation layer used by apis
- `tools/` — self-contained utilities (filter and sort SQL translation: `tools/search`)
- `migrations/` — PostgreSQL-only DDL, auto-applied at boot via advisory lock
- `third_party/dbx` — query builder (PostgreSQL dialect)
- `plugins/` — optional: jsvm hooks, migratecmd, ghupdate
- `tests/` — test harness (`tests/app.go`)
- `ui/src` — dashboard SPA (vanilla-JS custom reactive framework, not React/Svelte/Vue; 14 field types under `ui/src/fields/<type>/`)

## Hard rules

1. Never add SQLite as a storage engine.
2. Never change caller-visible API behavior without an entry in `docs/reference/api-contract.md` and a regression test.
3. New DDL goes in `migrations/` (PostgreSQL syntax; follow the existing naming `YYYYMMDDHHMMSS_name.go` + `init()` registration).
4. SQL identifiers in DDL paths must use `dbutils.DefaultDialect.QuoteIdentifier()`.
5. Env fallbacks for tests (`PGTEST_*`) must not appear in production code paths.
6. Security-relevant changes need a regression test (SSRF guard, caps, quoting, auth behaviors all have existing tests to model after).
7. **Never change a system property without defining how its correctness will be observed** (Platform Design P5).
8. **Every guardrail must have an enforcement mechanism** (guardrails.md); a guardrail without enforcement is a suggestion, not a rail.
9. **Every AI non-trivial change must complete the 10-step DoD** (above) with answers in the PR description.
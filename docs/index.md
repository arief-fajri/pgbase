---
layout: home

hero:
  name: PGBase
  text: PocketBase, powered by PostgreSQL
  tagline: A PostgreSQL-only backend-as-a-service — the PocketBase developer experience with Postgres durability, JSONB, and room to scale.
  image:
    src: /hero.svg
    alt: PGBase — a central app connected to auth, realtime, files, API rules, audit trails and backup
  actions:
    - theme: alt
      text: Fork deltas
      link: /fork-deltas
    - theme: alt
      text: Architecture
      link: /architecture/end-to-end
    - theme: alt
      text: GitHub
      link: https://github.com/arief-fajri/pgbase

features:
  - icon: 🧩
    title: Build
    details: Collections, API rules, auth, and realtime from a dashboard — the PocketBase API you already know.
    link: /collections-and-api-rules
    linkText: Collections & API rules
  - icon: 🛠️
    title: Operate
    details: Single-host Compose + Caddy, native pg_dump backups, Prometheus metrics, and a go-live checklist.
    link: /single-host-production
    linkText: Production guide
  - icon: 🧪
    title: Contribute
    details: Local dev, the test harness, linting, the release flow, and architecture deep-dives.
    link: /contributing
    linkText: Start contributing
  - icon: 🐘
    title: PostgreSQL-native
    details: pgx v5 + dbx, JSONB columns, timestamptz, pgcrypto IDs, and month-partitioned audit tables.
    link: /fork-deltas
    linkText: What changed vs upstream
  - icon: ⚡
    title: Realtime
    details: SSE subscriptions with per-message rule re-checks, plus an opt-in cross-instance outbox.
    link: /flows/realtime
    linkText: Realtime flows
  - icon: 📋
    title: Audit trails
    details: Separate write and read trails with per-field diffs and independent retention windows.
    link: /architecture/audit-design
    linkText: Audit design
---

## Conventions & badges

- **`stable`** — verified on the current tag. **`experimental`** — opt-in, still hardening (realtime outbox). **`roadmap-open`** — planned, not yet built (see the [roadmap](/roadmap)).
- File references use `path:line` (e.g. `core/base.go:43`) so you can jump straight to the source.
- Environment defaults live **only** in the [env reference](/reference/env); every other page links there.
- If a page's **Verified** badge is older than the current release, treat the code as truth and open a docs issue.

> **Live site:** <https://arief-fajri.github.io/pgbase/> — auto-deployed from `main` on docs changes. Local preview: `npm --prefix docs run docs:dev` (serves at `http://localhost:5174/pgbase/`).

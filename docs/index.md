---
layout: home

hero:
  name: PGBase
  text: Your PostgreSQL. Your backend. One binary.
  tagline: The simple, self-hosted application backend for PostgreSQL. Build with the simplicity of PocketBase. Keep the power and portability of PostgreSQL.
  image:
    src: /hero.svg
    alt: PGBase — one binary in front of PostgreSQL, exposing auth, realtime, files, API rules, and backups
  actions:
    - theme: brand
      text: Start with PostgreSQL
      link: /getting-started
    - theme: alt
      text: Migrate from PocketBase
      link: /migrate
    - theme: alt
      text: Deploy with your AI agent
      link: /agents

features:
  - icon: 🚀
    title: One binary
    details: Docker or a static binary in front of PostgreSQL you already run. No platform to operate.
    link: /getting-started
    linkText: Get started
  - icon: 🧩
    title: Application layer
    details: Collections, API rules, auth, realtime, and files from a dashboard. SQL stays available.
    link: /collections-and-api-rules
    linkText: Collections and API rules
  - icon: 🛠️
    title: Production primitives
    details: Native pg_dump backups, a runbook, Prometheus metrics, and a go-live checklist.
    link: /deployment/single-host
    linkText: Deployment guide
  - icon: 📖
    title: API contract
    details: REST and realtime you can build against here. PocketBase JS and Dart SDKs work today.
    link: /reference/api-contract
    linkText: Read the contract
  - icon: 🐘
    title: PostgreSQL stays yours
    details: JSONB, timestamptz, pgcrypto, pooling, and psql. The database is not hidden.
    link: /architecture/overview
    linkText: System overview
  - icon: 📋
    title: Audit trails
    details: Separate write and read trails with per-field diffs and independent retention windows.
    link: /architecture/audit-design
    linkText: Audit design
---

## Who this is for

| You | Start here |
|---|---|
| You already run PostgreSQL and need auth, CRUD, rules, realtime, files, and an admin UI | [Getting started](./getting-started.md) |
| You have a PocketBase data directory and want it in PostgreSQL | [Migrate](./migrate.md) — import works; a dedicated CLI does not yet |
| You are an operator | [Single-host setup](./deployment/single-host.md), [production runbook](./deployment/production.md) |
| An agent is provisioning the backend | [For agents](./agents.md) |

PG-BASE is the application layer for PostgreSQL. It is not a hosted platform, and it is not an embedded-database backend. See [Comparison](./comparison.md).

## Production shape

- **One binary + one PostgreSQL.** Optional S3 for files. Compose is the documented production path.
- **Backup and restore** use native `pg_dump` / `pg_restore`. Restore verification is still partial — see the [roadmap](./contributor/roadmap.md).
- **Secure defaults** — SSRF guards, download caps, opt-in encryption, rate limiting.
- **Observable enough to start** — Prometheus `/metrics`, request logs, pool stats. Tracing is not shipped.

## Conventions

- **`stable`** — verified on the current tag. **`experimental`** — opt-in, still hardening (realtime outbox). **`roadmap-open`** — planned, not built.
- Environment defaults live only in the [env reference](./reference/env.md).
- If a page's **Verified** badge is older than the current release, treat the code as truth and open a docs issue.

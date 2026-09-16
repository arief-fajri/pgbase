---
layout: home

hero:
  name: PGBase
  text: PostgreSQL-powered backend as a service
  tagline: PocketBase DX with PostgreSQL underneath. Self-hosted by default.
  image:
    src: /hero.svg
    alt: PGBase — a central app connected to auth, realtime, files, API rules, audit trails and backup
  actions:
    - theme: brand
      text: Getting Started
      link: /getting-started
    - theme: alt
      text: Compare
      link: /comparison
    - theme: alt
      text: GitHub
      link: https://github.com/arief-fajri/pgbase

features:
  - icon: 🚀
    title: Quick Start
    details: Get running in minutes with Docker or a single binary. One command to a working backend with PostgreSQL.
    link: /getting-started
    linkText: Get started now
  - icon: 🧩
    title: Build
    details: Collections, API rules, auth, and realtime from a dashboard — the PocketBase API you already know.
    link: /collections-and-api-rules
    linkText: Collections & API rules
  - icon: 🛠️
    title: Deploy
    details: Single-host Compose + Caddy, native pg_dump backups, Prometheus metrics, and a go-live checklist.
    link: /deployment/single-host
    linkText: Deployment guide
  - icon: 📖
    title: API Reference
    details: REST + realtime endpoints compatible with PocketBase SDKs. Full CRUD, auth, file storage, and batch operations.
    link: /reference/api-overview
    linkText: API overview
  - icon: 🐘
    title: PostgreSQL-native
    details: pgx v5 + dbx, JSONB columns, timestamptz, pgcrypto IDs, and month-partitioned audit tables.
    link: /fork-deltas
    linkText: What changed vs upstream
  - icon: 📋
    title: Audit trails
    details: Separate write and read trails with per-field diffs and independent retention windows.
    link: /architecture/audit-design
    linkText: Audit design
---

## Who is this for?

| Audience | What you'll find |
|----------|-----------------|
| **App Builders** | Quick start, API reference, collections & rules, auth flows, realtime |
| **Operators** | Deployment guides, production runbook, disaster recovery, environment variables |
| **Contributors** | Development setup, architecture overview, engineering methodology, roadmap |

## Production Ready

PG-BASE is built for production use from day one:

- **Backup/restore tested** — native `pg_dump`/`pg_restore` with automated round-trip verification
- **Race-safe** — full test suite passes under `-race` detector
- **SSRF protected** — built-in guards against server-side request forgery
- **Connection pooling** — dual-pool architecture with bounded limits and observability
- **Security hardened** — non-root containers, opt-in encryption, rate limiting, HSTS

## Conventions & badges

- **`stable`** — verified on the current tag. **`experimental`** — opt-in, still hardening (realtime outbox). **`roadmap-open`** — planned, not yet built (see the [roadmap](./contributor/roadmap.md)).
- Environment defaults live **only** in the [env reference](./reference/env.md); every other page links there.
- If a page's **Verified** badge is older than the current release, treat the code as truth and open a docs issue.

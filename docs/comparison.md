# Comparison

<DocMeta audience="All" status="living document" verified="v0.5.4" />

PG-BASE is a self-hosted PostgreSQL application backend: one binary in front of a database you already operate. It is not a smaller Supabase, and it is not a database with a new name.

> Your PostgreSQL. Your backend. One binary.

The programming model — collections, API rules, a dashboard, a single binary — is inspired by PocketBase. PostgreSQL itself is not the differentiator. Several products already sit on PostgreSQL. The difference is how little platform you have to operate to get an application backend.

## What each product is for

These are different problems, not a quality ranking. Check each project's docs before you decide.

| Product | Core promise |
|---|---|
| **PG-BASE** | Simple production backend for PostgreSQL. One binary, your database, no platform tax. |
| [PocketBase](https://github.com/pocketbase/pocketbase) | Simplest backend. Embedded database, one file, no separate Postgres to operate. |
| [Supabase](https://github.com/supabase/supabase) | Postgres development platform: many services, or a managed cloud. |
| [Appwrite](https://github.com/appwrite/appwrite) | Open-source application platform. |
| [Directus](https://github.com/directus/directus) | Data platform and data interface. |
| A backend you build yourself | Maximum control. You assemble the API, auth, rules, files, realtime, admin, backups, and ops. |

## When to choose what

**Choose PG-BASE** when you already want PostgreSQL, and you do not want to build the application layer or operate a multi-service platform. You keep `psql`, `pg_dump`, extensions, and your own backup story. PG-BASE adds REST, auth, rules, realtime, files, and a dashboard.

**Choose PocketBase** when you want an embedded database and you do not want to run PostgreSQL at all. That is a simpler deployment. It is the right tool for that job.

**Choose Supabase or Appwrite** when you want a platform: managed cloud, a large service surface, or features PG-BASE will not build (edge functions, a control plane, billing).

**Choose Directus** when the job is a data studio over an existing schema, not an application backend with its own rules model.

**Build it yourself** when you need a shape none of these fit. That is the real alternative to PG-BASE: PostgreSQL plus an ORM, an HTTP framework, an auth library, object storage, a websocket layer, and an admin UI. PG-BASE exists so that stack is optional.

## When not to choose PG-BASE

- You need a platform (edge functions, managed cloud, usage billing, an analytics product). That is out of scope.
- You do not want to operate PostgreSQL. Use an embedded-database backend instead.
- You need vector search, a dedicated PocketBase migration CLI, or a tested multi-instance topology **today**. Those are on the [roadmap](./contributor/roadmap.md), not shipped.
- You need Kubernetes as the default deploy shape. The promise is one binary plus one PostgreSQL. Compose is the documented production path.

## How we stay honest

Behavior that callers must rely on is written in the [API contract](./reference/api-contract.md) and covered by tests. Performance and positioning claims need reproducible evidence before they ship. A page that says a feature exists should point at code or a runbook, not a slogan.

# Getting Started

<DocMeta audience="All" status="stable" verified="v0.5.2" />

Get PG-BASE running in minutes. Choose the path that fits your setup.

## Option A: Docker (fastest)

One command to a running backend with PostgreSQL:

```bash
curl -fsSL https://raw.githubusercontent.com/arief-fajri/pgbase/main/deploy/quickstart.sh | sh
```

This boots PG-BASE + PostgreSQL via Docker, generates credentials, creates the first superuser, and prints the working URLs. Idempotent — re-running reuses the stack.

## Option B: Binary

Install the binary and connect to your own PostgreSQL:

```bash
curl -fsSL https://raw.githubusercontent.com/arief-fajri/pgbase/main/install.sh | sh
```

Then set the environment variables and start:

```bash
export PB_POSTGRES_HOST=localhost
export PB_POSTGRES_PORT=5432
export PB_POSTGRES_USER=pgbase
export PB_POSTGRES_PASSWORD=your_password
export PB_POSTGRES_DBNAME=pgbase
export PB_ENCRYPTION_KEY=your-32-char-key-here!!

./pgbase serve
```

## Option C: Build from source

```bash
git clone https://github.com/arief-fajri/pgbase.git && cd pgbase
go build -o pgbase ./examples/base
```

See the [Development Setup](./contributor/developing.md) for full prerequisites and instructions.

## What's next?

After your instance is running:

1. **Open the dashboard** at `http://localhost:8090/_/` (or your domain)
2. **Create your first collection** — collections are like database tables with built-in API routes
3. **Explore the REST API** — every collection gets CRUD endpoints automatically
4. **Try realtime** — subscribe to live updates via Server-Sent Events

## Key concepts

| Concept | Description |
|---------|-------------|
| **Collections** | Database tables with schema, access rules, and automatic REST API |
| **Records** | Rows in a collection — each has a 15-character ID |
| **Auth** | Built-in authentication per collection (password, OAuth2, OTP, MFA) |
| **Realtime** | Live subscriptions via SSE — get notified when data changes |
| **Files** | File storage on local disk or S3 |
| **Hooks** | Event handlers for custom logic (before/after create, update, delete) |

## Environment variables

The most important ones:

| Variable | Required | Description |
|----------|----------|-------------|
| `PB_POSTGRES_HOST` | Yes | PostgreSQL host |
| `PB_POSTGRES_PORT` | No | Default `5432` |
| `PB_POSTGRES_USER` | Yes | Database user |
| `PB_POSTGRES_PASSWORD` | Yes | Database password |
| `PB_POSTGRES_DBNAME` | Yes | Database name |
| `PB_POSTGRES_SSLMODE` | Recommended | `require` for production |
| `PB_ENCRYPTION_KEY` | Mandatory | 32-char key for encrypting settings secrets |

Full reference: [Environment Variables](./reference/env.md).

## Compare with alternatives

Not sure if PG-BASE is right for you? See [Comparison](./comparison.md) for a detailed breakdown against PocketBase, Supabase, and other options.

## Learn more

- [What's Different vs PocketBase](./fork-deltas.md) — compatibility contract
- [Collections & API Rules](./collections-and-api-rules.md) — data model reference
- [Authentication Flows](./flows/auth.md) — how auth works
- [Deployment Guide](./deployment/single-host.md) — production setup

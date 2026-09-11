# PG-BASE

PostgreSQL-powered backend as a service — a fork of [PocketBase v0.39.11](https://pocketbase.io).

## Features

- REST API (compatible with PocketBase)
- Real-time subscriptions
- Authentication & authorization
- File storage (local & S3)
- Dashboard UI
- **PostgreSQL database** (replaces SQLite)

**Why PG-BASE?** See the [comparison & positioning](https://arief-fajri.github.io/pgbase/comparison) guide — vs PocketBase, Supabase, and other PostgreSQL forks.

## Quick Start

### One command (Docker)

Boots PG-BASE + PostgreSQL with generated credentials and a first superuser:

```bash
curl -fsSL https://raw.githubusercontent.com/arief-fajri/pgbase/main/deploy/quickstart.sh | sh
```

The API + dashboard are then available at <http://localhost:8090/_/> with the
printed credentials. Deploying via an AI agent? See the
[agent quick start](https://arief-fajri.github.io/pgbase/agents).

### Docker (from a checkout)

First provide a `.env` (compose fails fast without a DB password):

```bash
cp .env.example .env      # then set PB_POSTGRES_PASSWORD, e.g. openssl rand -base64 32
docker compose up
```

The API + dashboard are then available at <http://localhost:8090/_/>. Create a superuser to log in:

```bash
docker compose exec pgbase pgbase superuser upsert admin@example.com "changeme123"
```

### Binary

```bash
# Install the latest release (checksum-verified) …
curl -fsSL https://raw.githubusercontent.com/arief-fajri/pgbase/main/install.sh | sh
# … or build from source
go build -o pgbase ./examples/base
```

The runnable entrypoint lives in `examples/base` (the root package is a library). Run with env vars or `--pg-*` flags:

```bash
./pgbase serve --http="127.0.0.1:8090"
```

Then create a superuser and open the dashboard:

```bash
./pgbase superuser upsert admin@example.com "changeme123"
# → http://127.0.0.1:8090/_/
```

### Environment Variables

| Variable | CLI flag (equivalent) | Default |
|----------|----------------------|---------|
| `PB_POSTGRES_HOST` | `--pg-host` | `localhost` |
| `PB_POSTGRES_PORT` | `--pg-port` | `5432` |
| `PB_POSTGRES_USER` | `--pg-user` | — |
| `PB_POSTGRES_PASSWORD` | `--pg-password` | — |
| `PB_POSTGRES_DBNAME` | `--pg-dbname` | — |
| `PB_POSTGRES_SSLMODE` | `--pg-sslmode` | `prefer` |

Flags take precedence over environment variables.

## Configuration

Connection settings are configured via CLI flags or environment variables (not stored in the database).

## Production

See the **[production runbook](https://arief-fajri.github.io/pgbase/production)** for a deployment guide — builds, env vars, TLS, hardening and a go-live checklist.

## Development

See the **[developer guide](https://arief-fajri.github.io/pgbase/developing)** — the single source of truth for running, testing, and contributing locally (PostgreSQL setup, dev servers, migrations, tests, lint). For the PR flow, see the [contributing guide](https://arief-fajri.github.io/pgbase/contributing).

## API

The REST API is compatible with PocketBase. Refer to the [PocketBase documentation](https://pocketbase.io/docs) for API details.

## License

This project is a fork of [PocketBase](https://github.com/pocketbase/pocketbase) and is licensed under the [MIT License](LICENSE.md).
# PG-BASE

PostgreSQL-powered backend as a service — a fork of [PocketBase v0.39.11 (WIP)](https://pocketbase.io).

## Features

- REST API (compatible with PocketBase)
- Real-time subscriptions
- Authentication & authorization
- File storage (local & S3)
- Dashboard UI
- **PostgreSQL database** (replaces SQLite)

## Quick Start

### Docker (Recommended)

```bash
docker compose up
```

The API + dashboard are then available at <http://localhost:8090/_/>. Create a
superuser to log in:

```bash
docker compose exec pgbase pgbase superuser create admin@example.com "changeme123"
```

### Binary

The runnable entrypoint lives in `examples/base` (the root package is a
library):

```bash
# Build
go build -o pgbase ./examples/base

# Run (env vars or --pg-* flags, see below)
./pgbase serve --http="127.0.0.1:8090"
```

Then create a superuser and open the dashboard:

```bash
./pgbase superuser create admin@example.com "changeme123"
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

Connection settings are configured via CLI flags or environment variables (not
stored in the database).

## Production

See **[PRODUCTION.md](PRODUCTION.md)** for a deployment runbook — builds, env
vars, TLS, hardening and a go-live checklist.

## Development

See **[DEV.md](DEV.md)** — the single source of truth for running, testing, and
contributing locally (PostgreSQL setup, dev servers, migrations, tests, lint).

## API

The REST API is compatible with PocketBase. Refer to the [PocketBase documentation](https://pocketbase.io/docs) for API details.

## License

This project is a fork of [PocketBase](https://github.com/pocketbase/pocketbase) and is licensed under the [MIT License](LICENSE.md).
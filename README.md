# PG-BASE

Your PostgreSQL. Your backend. One binary.

A self-hosted, single-binary application backend built around PostgreSQL — with the simplicity of PocketBase and the operational portability of PostgreSQL.

PG-BASE turns PostgreSQL into an application backend: REST, auth, rules, realtime, files, and a dashboard. You keep the database. There is no platform to operate.

The programming model is inspired by PocketBase. PocketBase JS and Dart SDKs work against this API today; the behavior contract is ours.

## Features

- REST API and realtime subscriptions
- Authentication and API rules
- File storage (local and S3)
- Admin dashboard
- PostgreSQL 16+ (`pgx` v5, JSONB, `timestamptz`)
- Native `pg_dump` / `pg_restore` backups
- Audit trails and Prometheus metrics

**Why PG-BASE?** See the [comparison](https://arief-fajri.github.io/pgbase/comparison).

## Quick start

### One command (Docker)

Boots PG-BASE and PostgreSQL with generated credentials and a first superuser:

```bash
curl -fsSL https://raw.githubusercontent.com/arief-fajri/pgbase/main/deploy/quickstart.sh | sh
```

The API and dashboard are then available at <http://localhost:8090/_/> with the printed credentials. Deploying via an AI agent? See the [agent quick start](https://arief-fajri.github.io/pgbase/agents).

### Docker (from a checkout)

Compose fails fast without a database password:

```bash
cp .env.example .env      # then set PB_POSTGRES_PASSWORD, e.g. openssl rand -base64 32
docker compose up
```

The API and dashboard are at <http://localhost:8090/_/>. Create a superuser to log in:

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

The runnable entrypoint is `examples/base` (the root package is a library):

```bash
./pgbase serve --http="127.0.0.1:8090"
./pgbase superuser upsert admin@example.com "changeme123"
# → http://127.0.0.1:8090/_/
```

### Environment variables

| Variable | CLI flag | Default |
|----------|----------|---------|
| `PB_POSTGRES_HOST` | `--pg-host` | `localhost` |
| `PB_POSTGRES_PORT` | `--pg-port` | `5432` |
| `PB_POSTGRES_USER` | `--pg-user` | — |
| `PB_POSTGRES_PASSWORD` | `--pg-password` | — |
| `PB_POSTGRES_DBNAME` | `--pg-dbname` | — |
| `PB_POSTGRES_SSLMODE` | `--pg-sslmode` | `prefer` |

Flags take precedence over environment variables. Connection settings are not stored in the database.

## Production

See the [production runbook](https://arief-fajri.github.io/pgbase/deployment/production).

Moving an existing PocketBase data directory? See [Migrate](https://arief-fajri.github.io/pgbase/migrate). Import works today. A dedicated migration CLI does not.

## Development

See the [developer guide](https://arief-fajri.github.io/pgbase/contributor/developing) and the [contributing guide](https://arief-fajri.github.io/pgbase/contributor/contributing).

## API

Start with the [API overview](https://arief-fajri.github.io/pgbase/reference/api-overview) and the [API contract](https://arief-fajri.github.io/pgbase/reference/api-contract). A full per-endpoint reference is not published yet.

## License

MIT. See [LICENSE.md](LICENSE.md).

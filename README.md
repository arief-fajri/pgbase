# PG-BASE

PostgreSQL-powered backend as a service — Fork of [PocketBase](https://pocketbase.io).

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
docker-compose up
```

### Binary

```bash
./pgbase serve \
  --pg-host localhost \
  --pg-port 5432 \
  --pg-user pgbase \
  --pg-password secret \
  --pg-dbname pgbase
```

### Environment Variables

```bash
PB_POSTGRES_HOST=localhost
PB_POSTGRES_PORT=5432
PB_POSTGRES_USER=pgbase
PB_POSTGRES_PASSWORD=secret
PB_POSTGRES_DBNAME=pgbase
PB_POSTGRES_SSLMODE=disable
```

## Configuration

Connection settings are configured via CLI flags or environment variables (not stored in the database).

## API

The REST API is compatible with PocketBase. Refer to the [PocketBase documentation](https://pocketbase.io/docs) for API details.

## License

This project is a fork of [PocketBase](https://github.com/pocketbase/pocketbase) and is licensed under the [MIT License](LICENSE.md).

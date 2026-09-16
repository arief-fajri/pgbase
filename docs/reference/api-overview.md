# API Overview

<DocMeta audience="Developer" status="stable" verified="v0.5.2" />

PG-BASE provides a REST API that is compatible with [PocketBase](https://pocketbase.io/docs). If you know the PocketBase API, you already know PG-BASE.

## Base URL

```text
http://localhost:8090
```

Or your deployed domain: `https://your-domain.com`

## Authentication

Most endpoints require authentication. Get a token via:

```http
POST /api/collections/{collection}/auth-with-password
```

Then include it in requests:

```http
Authorization: Bearer {token}
```

Superusers have access to all collections and admin endpoints.

## Core endpoints

### Records (CRUD)

```http
GET    /api/collections/{collection}/records           # List
GET    /api/collections/{collection}/records/{id}      # Get one
POST   /api/collections/{collection}/records           # Create
PATCH  /api/collections/{collection}/records/{id}      # Update
DELETE /api/collections/{collection}/records/{id}      # Delete
```

**Query parameters:**

| Parameter | Example | Description |
|-----------|---------|-------------|
| `filter` | `filter=status="active"` | SQL-like filter syntax |
| `sort` | `sort=-created` | Sort field (prefix `-` for descending) |
| `page` | `page=2` | Page number |
| `perPage` | `perPage=50` | Items per page (max 300) |
| `expand` | `expand=author` | Expand relations |
| `fields` | `fields=id,name,email` | Select specific fields |

### Authentication

```http
POST /api/collections/{collection}/auth-with-password   # Password login
POST /api/collections/{collection}/auth-refresh         # Refresh token
POST /api/collections/{collection}/auth-logout          # Logout
POST /api/collections/{collection}/request-otp          # Request OTP code
POST /api/collections/{collection}/auth-mfa             # MFA verification
```

See [Authentication Flows](../flows/auth.md) for details.

### Realtime (SSE)

```http
GET  /api/realtime          # SSE stream
POST /api/realtime          # Subscribe to records
```

See [Realtime Flows](../flows/realtime.md) for details.

### File storage

```http
GET /api/files/{collection}/{recordId}/{filename}      # Download
POST /api/files/{collection}/{recordId}                # Upload (multipart)
```

### Batch operations

```http
POST /api/batch
```

Execute multiple operations in a single request.

## Superuser-only endpoints

| Endpoint | Description |
|----------|-------------|
| `/api/collections` | Manage collections (create/update/delete) |
| `/api/settings` | App settings |
| `/api/logs` | Request logs |
| `/api/audits` | Audit trail |
| `/api/backups` | Backup management |
| `/api/crons` | Cron jobs |
| `/api/health` | Health check |
| `/api/sql` | Direct SQL execution |
| `/api/collections/_superusers/*` | Superuser auth |

## Dashboard

The admin dashboard is available at `/_/` and provides a UI for managing collections, records, files, settings, and more.

## Differences from PocketBase

PG-BASE is compatible with PocketBase, but there are [9 documented differences](../fork-deltas.md) including:

- PostgreSQL-only storage engine (no SQLite)
- Case-insensitive identity indexes
- Editor HTML sanitization
- Single-instance realtime defaults
- Batch transaction semantics

## Full API documentation

For complete API details, see the [PocketBase API docs](https://pocketbase.io/docs) — the API is intentionally compatible.

## SDKs

Use the official PocketBase SDKs:

- [JavaScript/TypeScript](https://github.com/pocketbase/js-sdk)
- [Dart/Flutter](https://github.com/pocketbase/dart-sdk)
- [Python](https://github.com/pocketbase/pypebble)
- [Go](https://github.com/pocketbase/pocketbase-sdk-go)

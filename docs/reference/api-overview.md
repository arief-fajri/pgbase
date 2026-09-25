# API Overview

<DocMeta audience="Developer" status="stable" verified="v0.5.4" />

PG-BASE exposes a REST and realtime API in front of your PostgreSQL database. This page is the map. Behavior that callers must rely on is the [API contract](./api-contract.md). A complete per-endpoint reference is not published yet — that is [Phase 1](../contributor/roadmap.md).

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

## Behavior contract

Read the [API contract](./api-contract.md) before you rely on edge behavior. It covers the PostgreSQL engine, identity indexes, record IDs, editor HTML, realtime scope, batch transactions, auth limits, and file-fetch guards.

The in-dashboard API preview is the live syntax reference for a collection. It is more accurate than this overview.

## SDKs

These clients work against this API today. They are a convenience. This overview and the contract are the source of truth.

- [JavaScript/TypeScript](https://github.com/pocketbase/js-sdk)
- [Dart/Flutter](https://github.com/pocketbase/dart-sdk)

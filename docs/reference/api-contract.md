# API Contract

<DocMeta audience="App Builder" status="stable" verified="v0.5.4" />

This page is the behavior contract for PG-BASE. Build against it and the [API overview](./api-overview.md). A complete per-endpoint reference is not published here yet — that is [Phase 1](../contributor/roadmap.md). The in-dashboard API preview is the live syntax reference.

PocketBase JS and Dart SDKs work against this API today. That is a convenience, not the source of truth. If this page and an SDK example disagree, this page wins.

## 1. PostgreSQL only

- Engine: `pgx` v5 plus the query builder in `third_party/dbx`. Connection: `core/db_connect.go:37 ResolveDBConfig`, `core/db_connect.go:74 DefaultDBConnect`.
- Dates are `timestamptz`. Schemaless fields are JSONB (`tools/dbutils/pgsql.go`, `tools/dbutils/json.go`). Filter and sort translation: `tools/search/`.
- Extension `pgcrypto` must exist **before** first boot, otherwise aux and data bootstrap deadlock. See the [development guide](../contributor/developing.md). Docker Compose and `docker/init/` pre-install it.
- There are 14 field types (`core/field_*.go`): text, number, bool, email, url, date, autodate, password, relation, select, file, json, editor, geoPoint. There is no 15th type.
- The database is yours. You can connect with `psql`. PG-BASE does not hide PostgreSQL behind a private query language.

## 2. Case-insensitive identity with an enforced functional index

Password-auth identity fields (`email`, `username`, custom text fields in `PasswordAuth.IdentityFields`) always match `LOWER(field) = LOWER(?)`.

- Every active identity field must carry a partial functional unique index:

  ```sql
  CREATE UNIQUE INDEX "idx_<field>_<id>" ON "<collection>" (LOWER("<field>")) WHERE "<field>" <> '';
  ```

- Runtime auto-appends it (`initIdentityFieldIndexes`). The collection validator rejects an auth collection whose identity field lacks the `LOWER()` index (`validation_non_functional_identity_unique_index`). A plain case-sensitive index is refused so logins stay index-served.
- Removing a field from `IdentityFields` does **not** drop its index. Destructive drops need an admin decision. The orphaned index keeps enforcing case-insensitive uniqueness; drop it from the dashboard or a migration if unwanted. See the [development guide](../contributor/developing.md).
- The OAuth2 username check uses `LOWER(username) = LOWER(?)` without the `<> ''` predicate. It is a secondary guard (`LIMIT 1`), not the hot login path.

## 3. Record IDs are random 15-character TEXT

Primary keys are random, non-monotonic TEXT (`gen_random_bytes` fallback). Inserts split B-tree pages. IDs are not a time order — sort by `-created`, not `-@rowid`. This ID shape is part of the API contract and will not change. See the [development guide](../contributor/developing.md).

## 4. `editor` HTML is sanitized on write, verbatim on read

`editor` values are stored as HTML and sanitized on write (allow-list via bluemonday — scripts, event handlers, iframes/embeds, and `javascript:` / `data:` / SVG URLs are stripped). The dashboard renders through a sandboxed editor. `GET /api/collections/.../records` returns stored HTML verbatim. Treat it as untrusted in any client that injects into the DOM when non-superusers can write. Native `pg_restore` restores stored HTML as-is.

## 5. Realtime defaults to one instance

`GET /api/realtime` fans out only to subscribers on the instance that performed the write. Cross-instance fanout needs the opt-in outbox (`PB_REALTIME_OUTBOX=1`, table `_realtime_outbox`, channel `pb_realtime_outbox`). Details: [Realtime](../flows/realtime.md). The LISTEN connection is a dedicated `pgx` connection, not pooled — connect directly or via session pooling, not PgBouncer transaction mode.

## 6. Batch is one database transaction

`POST /api/batch` (`apis/batch.go:26`) must be enabled in app settings. Requests share auth state. File uploads use `FormData` `@jsonPayload` plus `requests.N.<field>` (see in-dashboard `docsBatch.js`). Large batches hold one transaction — keep them small. See [Collections and API rules](../collections-and-api-rules.md).

## 7. Auth specifics

- OTP email codes: max 5 attempts per 180s per OTP id (`apis/record_auth_with_otp.go:57`), then `429`. OTP is single-use (deleted on success).
- MFA is enforced per `MFA.Enabled + Rule` (`apis/record_helpers.go:76-90`) and advertised in `auth-methods` (`apis/record_auth_methods.go:99-114`).
- Refresh (`apis/record_auth_refresh.go:25-31`): a new token is issued **only** when the claim `TokenClaimRefreshable` is true. Impersonate tokens reuse the presented token (conditional renewal, not rotation).
- Impersonate is superuser-only (`apis/record_auth_impersonate.go`). Duration is capped by `PB_IMPERSONATE_MAX_TOKEN_DURATION` (seconds, default 30 days).
- `TrustedProxy` (`core/settings_model.go:655-665`): set it only when **all** inbound traffic passes through the trusted proxy. `RealIP()` trusts `X-Forwarded-For` verbatim. A directly reachable app port lets anyone spoof it and bypass rate limits and `SuperuserIPs` (`core/event_request.go:40-44`).

## 8. Files: SSRF guard, caps, `nosniff`

`NewFileFromURL` uses `SafeHTTPClient` (`tools/security/httputil.go`). Private, loopback, and multicast dials are rejected. Oversized responses are capped. File responses carry `X-Content-Type-Options: nosniff` (v0.5.2).

## 9. Liveness vs readiness

`GET /api/health` is liveness only: it answers `200` while the process serves and never touches the database — a load balancer must not kill a healthy process just because PostgreSQL is down. `GET /api/ready` (`apis/ready.go`) is the readiness probe: `200` only when the data pool can execute a query and the core `_collections` schema is readable (bounded by a 5 s probe timeout); otherwise `503` with a static `"API is not ready."` message — the raw error is logged server-side only, never in the response body. Wire readiness probes (load balancers, Kubernetes, the Compose healthcheck in `docker-compose.prod.yml`) to `/api/ready`; use `/api/health` only to decide whether the process itself is alive.

## 10. Intentionally not in this contract yet

Do not assume these. They are on the [roadmap](../contributor/roadmap.md): multi-instance rate limiting (today the limit is per instance; Phase 4), `_logs` retention that does not bloat on cleanup (Phase 4), GIN indexes for multi-value JSONB filters (Phase 4), a dedicated PocketBase migration CLI (Phase 1), a published endpoint reference (Phase 1), and a PostgreSQL 16/17 compatibility matrix (Phase 0).

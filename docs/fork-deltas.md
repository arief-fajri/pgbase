# PGBase vs Upstream PocketBase: Fork Deltas

> Audience: App Builder · Status: stable · Last verified: v0.5.2 (`923e860`)

The public REST API is PocketBase-compatible. Build against the
[upstream docs](https://pocketbase.io/docs) **except** the deltas below.
Each delta links to the code that enforces it.

## 1. PostgreSQL-only, no SQLite

- Engine: `pgx` v5 stdlib + `pocketbase/dbx` fork (`third_party/dbx`).
  Connection: `core/db_connect.go:37 ResolveDBConfig`, `core/db_connect.go:74 DefaultDBConnect`.
- Dates are `timestamptz`, schemaless fields are JSONB (`tools/dbutils/pgsql.go`,
  `tools/dbutils/json.go`). Filter/sort translation: `tools/search/`.
- Extension `pgcrypto` must exist **before** first boot, otherwise the aux + data
  bootstrap deadlocks (`../DEV.md` §2). Docker Compose and `docker/init/` pre-install it.
- There are 14 field types (`core/field_*.go`): text, number, bool, email, url,
  date, autodate, password, relation, select, file, json, editor, geoPoint.
  There is no 15th type.

## 2. Case-insensitive identity with enforced functional index

Password-auth identity fields (`email`, `username`, custom text fields in
`PasswordAuth.IdentityFields`) always match `LOWER(field) = LOWER(?)`.

- Every active identity field must carry a partial functional unique index:

  ```sql
  CREATE UNIQUE INDEX "idx_<field>_<id>" ON "<collection>" (LOWER("<field>")) WHERE "<field>" <> '';
  ```

- Runtime auto-appends it (`initIdentityFieldIndexes`); the collection validator
  rejects an auth collection whose identity field lacks the `LOWER()` index
  (`validation_non_functional_identity_unique_index`). A plain case-sensitive
  index is refused so logins stay index-served.
- Removing a field from `IdentityFields` does **not** drop its index (intentional,
  destructive ops need an admin decision). The orphaned index keeps enforcing
  case-insensitive uniqueness; drop it via dashboard/migration if unwanted.
  See `../DEV.md` §3b.
- OAuth2 username check uses `LOWER(username) = LOWER(?)` without the `<> ''`
  predicate (secondary guard, `LIMIT 1`, not the hot login path).

## 3. Record IDs are random 15-char TEXT (kept for API compat)

Random, non-monotonic TEXT primary keys (`gen_random_bytes` fallback). Consequence:
B-tree page splits on insert, no time ordering (use `-created`, not `-@rowid`).
This is an upstream-compat tradeoff and will not change (`../DEV.md` §5b, `roadmap.md` PERF-I07).

## 4. `editor` HTML: sanitized at write, verbatim on read

`editor` values are stored as HTML, sanitized server-side at write time
(allow-list via bluemonday — scripts, event handlers, iframes/embeds,
`javascript:`/`data:`/SVG URLs stripped). The dashboard renders through a
sandboxed editor, but `GET /api/collections/.../records` returns stored HTML
verbatim. Treat it as untrusted in any client that injects into the DOM
(use a DOMPurify-style pass) when non-superusers can write. Native `pg_restore`
restores stored HTML as-is.

## 5. Realtime defaults to single-instance

`GET /api/realtime` SSE fans out only to subscribers on the instance that
performed the write. Cross-instance fanout needs the opt-in outbox
(`PB_REALTIME_OUTBOX=1`, table `_realtime_outbox`, channel `pb_realtime_outbox`).
Details: `flows/realtime.md`. The LISTEN connection is a dedicated `pgx` conn,
not pooled — connect directly or via session pooling, not PgBouncer transaction mode.

## 6. Batch is a single-DB-transaction (perf caution)

`POST /api/batch` (`apis/batch.go:26`) must be enabled in App settings. Requests
share auth state; file uploads use `FormData` `@jsonPayload` + `requests.N.<field>`
pattern (see in-dashboard `docsBatch.js`). Large batches hold one transaction —
keep them small; see `collections-and-api-rules.md`.

## 7. Auth specifics

- OTP email codes: max 5 attempts per 180s per OTP id
  (`apis/record_auth_with_otp.go:57`), then `429`. OTP is single-use (deleted on success).
- MFA: enforced per `MFA.Enabled + Rule` (`apis/record_helpers.go:76-90`);
  advertised in `auth-methods` (`apis/record_auth_methods.go:99-114`).
- Refresh (`apis/record_auth_refresh.go:25-31`): a new token is issued **only**
  when the claim `TokenClaimRefreshable` is true; impersonate tokens reuse the
  presented token (conditional renewal, not rotation).
- Impersonate (superuser-only, `apis/record_auth_impersonate.go`):
  duration capped by `PB_IMPERSONATE_MAX_TOKEN_DURATION` (seconds, default 30 days).
- `TrustedProxy` (`core/settings_model.go:633-658`): only set when **all** inbound
  traffic passes through the trusted proxy. `RealIP()` trusts `X-Forwarded-For`
  verbatim — a directly reachable app port lets anyone spoof it and bypass
  rate limits + `SuperuserIPs` (`core/event_request.go:40-44`).

## 8. Files: SSRF guard + caps + `nosniff`

`NewFileFromURL` uses shared `SafeHTTPClient` (`tools/security/httputil.go`) —
private/loopback/multicast dials rejected; oversized responses capped;
file responses carry `X-Content-Type-Options: nosniff` (v0.5.2).

## 9. What is intentionally out of scope

Per `roadmap.md` §5: no SQLite re-add, no TEXT-PK change, no REST API divergence.
Multi-instance rate limiting (N× per-instance today), `_logs` partitioning,
GIN JSONB indexes, OTel tracing, PITR/WAL, S3 auto-offload, K8s, and the SDK/PG
support matrix are `roadmap-open` — do not assume them.

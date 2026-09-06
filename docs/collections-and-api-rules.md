# Collections, Records, and API Rules

> Audience: App Builder · Status: stable · Last verified: v0.5.2 (`923e860`)

## 1. Model

| Kind | Meaning | Code |
|---|---|---|
| `base` | Default content collection | `core/collection_model.go:25-27` |
| `auth` | Users with password / OAuth2 / OTP / MFA + tokens | `core/collection_model_auth_options.go`, `core/record_model_auth.go` |
| `view` | Read-only SQL view collection | `core/collection_model.go:476-487` |

Collections map to one PostgreSQL table each; schema sync emits DDL per
collection (`core/collection_record_table_sync.go`). System collections include
`_superusers`, `_migrations`, `_logs` (aux DB), `_audits` + `_audit_reads`
(month-range partitioned), `_externalAuths`, `_mfas`, `_otps`, `_authOrigins`.

Rules (`list/view/create/update/delete`) are the authorization layer. Realtime
delivery re-checks them per message (view-rule for single-record subscriptions,
list-rule for `*`).

## 2. Use the in-dashboard API preview (don't copy this doc)

The dashboard ships a live, collection-aware reference at
`ui/src/apiPreview/` (19 files): `apiPreviewModal.js` tabs per operation with
JS / Dart / curl snippets using the collection's actual fields and rules.

| Operation | Preview file | Notes |
|---|---|---|
| List/Search | `docsList.js` | `page/perPage/skipTotal`, `sort` (`-`/`+`, `@random`), `filter`, `expand`, `fields`; `200/400/403` |
| View / Create / Update / Delete | `docsView\|Create\|Update\|Delete.js` | Same envelope as list |
| Batch | `docsBatch.js` | `POST /api/batch` (`apis/batch.go:26`); must be enabled in App settings; single-DB-tx — keep small; shared auth state |
| Realtime | `docsRealtime.js` | `GET/POST /api/realtime`; `*` needs list-rule, id needs view-rule; event `{action: create\|update\|delete, record}` |
| Auth | `docsAuthWithPassword\|OAuth2\|OTP\|Refresh\|ListAuthMethods\|Verification\|PasswordReset\|EmailChange.js` | Live `identityFields`; OTP/MFA tabs appear only when enabled |
| Syntax | `filterSyntax.js`, `expandInfo.js`, `fieldsInfo.js` | Operators `= != > >= < <= ~ !~ ?= ?!= ?> ?>= ?< ?<= ?~ ?!~`, `&& \|\| (...)`, auto-`%` wrap, URL-encoding note |

Workflow: open `#/collections` → collection → record → **API preview**.
This doc only covers fork deltas (`fork-deltas.md`); syntax truth lives in the preview + upstream docs.

## 3. CRUD routes (rule-checked)

| Method | Route | Handler |
|---|---|---|
| `GET` | `/api/collections` | `apis/collection.go` (superuser list) |
| `POST` | `/api/collections` | create (superuser) |
| `GET/PATCH/DELETE` | `/api/collections/{collection}` | manage incl. `DELETE .../truncate` |
| `PUT` | `/api/collections/import` | import + diff review (`apis/collection_import.go`) |
| `GET/POST` | `/api/collections/{collection}/records` | list / create (`apis/record_crud.go`, `forms/record_upsert.go`) |
| `GET/PATCH/DELETE` | `/api/collections/{collection}/records/{id}` | view / update / delete |
| `POST` | `/api/batch` | multi upsert/delete (single transaction) |

Expand is batched server-side (N+1 removed in v0.5.0); file fields serve via
`apis/file.go:47` (Gzip opt-out) with superuser file-token flow.

## 4. Indexes (what builders can declare)

Declare indexes in the collection's `indexes` JSON; schema sync rebuilds only
changed/added ones (index-diff sync, v0.5.0). Auth identity fields require the
`LOWER()` partial unique form (see `fork-deltas.md` §2). GIN/`jsonb_path_ops`
for multi-value JSONB filters is `roadmap-open` (PERF-I02) — expect full scans today.

Exceptional escape hatch for huge hot tables: build out-of-band
(`../DEV.md` §5b):

```sql
CREATE UNIQUE INDEX CONCURRENTLY "idx_foo" ON "my_table" (LOWER("username")) WHERE "username" <> '';
```

`CONCURRENTLY` cannot run in a transaction (use `psql` directly); drop `INVALID`
leftovers before retry; then declare the index in the collection JSON so sync
treats it as managed. A general out-of-tx `CONCURRENTLY` path is `roadmap-open` (PERF-I08).

## 5. Settings that gate features

`GET/PATCH /api/settings` (`apis/settings.go:13-22`): Batch on/off, RateLimits
(per-instance — N instances = N× limit, shared store is `roadmap-open` Theme A),
`SuperuserIPs` whitelist, `TrustedProxy`, HSTS/CSP. Mail test, S3 test, and
Apple client-secret endpoints live under the same group.

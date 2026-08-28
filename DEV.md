# PG-BASE Development Guide

Step-by-step guide to run, test, and contribute to **PG-BASE** in a local environment.

PG-BASE is a PostgreSQL-powered backend-as-a-service and a fork of
[PocketBase v0.39.11](https://pocketbase.io). It ships a REST API, real-time
subscriptions, auth, file storage, and a web dashboard.

> **Target audience:** every engineer, from junior to senior. If any step looks
> confusing or fails, open an issue — this document is meant to be the single
> source of truth for local development.

---

## Quick Start (TL;DR)

Known-good path in ~5 minutes (uses Docker for PostgreSQL):

```bash
# 1. Clone the repo and install dependencies
git clone <repository-url> && cd pgbase
go mod download
cd ui && npm install && cd ..

# 2. Start PostgreSQL (Docker). Run from the repo root: the docker/init mount
#    pre-installs the pgcrypto extension on first boot, without which the very
#    first startup deadlocks while bootstrapping (see Troubleshooting).
docker run -d --name pgbase-pg \
  -p 5432:5432 \
  -e POSTGRES_DB=pgbase \
  -e POSTGRES_USER=pgbase \
  -e POSTGRES_PASSWORD=secret \
  -v "$(pwd)/docker/init:/docker-entrypoint-initdb.d:ro" \
  postgres:16-alpine

# 3. Start the backend (dev mode)
PB_POSTGRES_PASSWORD=secret go run ./examples/base serve --http="127.0.0.1:8090" --dev
```

Open a **second terminal** and create a superuser (required to log into `/_/`):

```bash
PB_POSTGRES_PASSWORD=secret go run ./examples/base superuser upsert admin@example.com "changeme123"
```

Then open <http://127.0.0.1:8090/_/> and log in.

**You know your setup works when:**

- `go run ./examples/base serve ...` prints the start banner and stays running
- `curl http://127.0.0.1:8090/api/health` returns `200` with an `ok` JSON body
- The dashboard at `http://127.0.0.1:8090/_/` loads and accepts the superuser login
- `go test ./...` passes after the test database is started (see [Testing](#9-running-tests))

---

## Prerequisites

| Tool | Minimal | Why / Notes | Check |
|------|---------|-------------|-------|
| Go | 1.25+ | Matches `go 1.25.0` in `go.mod`; CI pins `>=1.26.5` | `go version` |
| Node.js | 22+ (see note) | `ui/` is a Vite app; CI pins `>=25.2.1` | `node --version` |
| npm | 10+ | Bundled with Node.js | `npm --version` |
| PostgreSQL | 16+ | Both dev and CI use `postgres:16-alpine` | `psql --version` |
| Docker | 24+ (optional) | Fastest way to run PostgreSQL and the full stack | `docker --version` |

> [!IMPORTANT]
> Node.js **18 is EOL** and is too old for this project. Use at least Node 22
> (LTS); the CI runs on Node 25+ (`>=25.2.1`).

> [!NOTE]
> The PostgreSQL binary itself is only needed if you use **Option B
> (Postgres.app)** below. With Docker you never need a local `psql`.

---

## 1. Clone & Install Dependencies

```bash
# Go dependencies
go mod download

# UI dependencies (Vite-bundled vanilla-JS dashboard)
cd ui
npm install
cd ..
```

---

## 2. Start PostgreSQL

Pick **one** option. Option A is recommended for day-to-day development.

### Option A: Docker (recommended)

```bash
docker run -d --name pgbase-pg \
  -p 5432:5432 \
  -e POSTGRES_DB=pgbase \
  -e POSTGRES_USER=pgbase \
  -e POSTGRES_PASSWORD=secret \
  -v "$(pwd)/docker/init:/docker-entrypoint-initdb.d:ro" \
  postgres:16-alpine
```

PostgreSQL is now reachable at `localhost:5432`.

> [!IMPORTANT]
> The `-v .../docker/init:/docker-entrypoint-initdb.d` mount runs
> `docker/init/01_pgcrypto.sql` on the **first** boot, pre-installing the
> `pgcrypto` extension. This is required: without it the first
> `serve`/`superuser` **deadlocks** while bootstrapping the aux + data
> migrations on an empty database (the startup hangs and `:8090` never opens).
> If you already started the container **without** the mount (or reuse an old
> volume — init scripts only run on an empty data dir), install it once
> instead:
> `docker exec pgbase-pg psql -U pgbase -d pgbase -c "CREATE EXTENSION IF NOT EXISTS pgcrypto;"`

> If the host port `5432` is already in use (e.g. by Postgres.app or another
> container), map a different port, e.g. `-p 5433:5432`, and set
> `PB_POSTGRES_PORT=5433` when running the app.

### Option B: Postgres.app (macOS)

1. Open **Postgres.app** → click **Start**.
2. The default superuser is your **macOS username** (e.g. `volantisfrontend`).
   All `psql`/`createdb` commands below run as that superuser.
3. Create the development database, role, and assign them:

```bash
createdb pgbase
createuser pgbase
psql -d pgbase -c "ALTER USER pgbase WITH PASSWORD 'secret';"
psql -d pgbase -c "ALTER DATABASE pgbase OWNER TO pgbase;"
psql -d pgbase -c "CREATE EXTENSION IF NOT EXISTS pgcrypto;"
```

> [!IMPORTANT]
> The `CREATE EXTENSION ... pgcrypto` line is required. Without it the first
> `serve`/`superuser` deadlocks while bootstrapping the aux + data migrations
> (see Troubleshooting) — the Docker option pre-installs it via `docker/init`,
> but with Postgres.app you must create it yourself.

> [!IMPORTANT]
> PostgreSQL 15+ removed the public create privilege for non-owner roles. If
> the `pgbase` role does not **own** the database, you will hit
> `permission denied for schema public` on the first startup. The
> `ALTER DATABASE pgbase OWNER TO pgbase;` above prevents that. (For local dev
> you could also use `createuser -s pgbase` to make it a superuser.)

4. Create the **test** database (used by section [9. Running Tests](#9-running-tests)):

```bash
createdb pgbase_test
createuser test
psql -d pgbase_test -c "ALTER USER test WITH PASSWORD 'test';"
psql -d pgbase_test -c "ALTER DATABASE pgbase_test OWNER TO test;"
psql -d pgbase_test -c "CREATE EXTENSION IF NOT EXISTS pgcrypto;"
```

### Option C: Docker Compose (full stack)

```bash
docker compose up
```

This builds the image (slow the first time) and runs **PostgreSQL + the PG-BASE
web app** (`http://localhost:8090/_/`). It is NOT the right choice when you want
hot-reload UI development — for that, use Option A/B and follow sections 3 & 4.

---

## 3. Run the Backend (Dev Mode)

The main entrypoint lives in `examples/base/main.go`. From the project root:

```bash
# Start the Go API server in dev mode
PB_POSTGRES_PASSWORD=secret go run ./examples/base serve --http="127.0.0.1:8090" --dev
```

> [!WARNING]
> Do **not** use `go run .` or `go build -o pgbase .` from the root — the root
> package is a **library** (`package pgbase`), not a `main` package. Those
> commands produce errors/archives, not an app. The runnable entrypoint is
> `./examples/base`.

### `serve` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--http` | `127.0.0.1:8090` | TCP address for the HTTP server |
| `--https` | – | TCP address for the HTTPS server |
| `--dev` | `false` | Dev mode: verbose logging, relaxed caching, etc. |
| `--origins` | `*` | CORS allowed origins list |

### PostgreSQL connection (environment variables)

The app reads its PostgreSQL connection settings from **environment variables**
only (all sharing the `PB_POSTGRES_` prefix). For the section 2 setup the
defaults already match the database, so in practice you only need to pass
`PB_POSTGRES_PASSWORD`.

| Env variable | Default (if unset) | Value for the section 2 setup |
|--------------|--------------------|-------------------------------|
| `PB_POSTGRES_HOST` | `localhost` | `localhost` |
| `PB_POSTGRES_PORT` | `5432` | `5432` |
| `PB_POSTGRES_USER` | `pgbase` | `pgbase` |
| `PB_POSTGRES_PASSWORD` | *(empty — must be set)* | `secret` |
| `PB_POSTGRES_DBNAME` | `pgbase` | `pgbase` |

> [!NOTE]
> SSL is disabled (`sslmode=disable`) by the built-in connector and is not
> configurable through an env variable in the default build.

> [!WARNING]
> The `serve` command also *lists* `--pg-host`, `--pg-port`, `--pg-user`,
> `--pg-password`, `--pg-dbname`, and `--pg-sslmode` flags, but they are **not
> wired to the database connection** — the app always reads the `PB_POSTGRES_*`
> variables above. Set those env vars instead of the flags. Also note that
> `go run` enables `--dev` automatically, so that flag is optional in dev.

Because the defaults already match the section 2 database, the minimal dev
command only needs the password:

```bash
PB_POSTGRES_PASSWORD=secret go run ./examples/base serve --http="127.0.0.1:8090"
```

> The API will be available at `http://127.0.0.1:8090`.
> Read more about the API at [PocketBase docs](https://pocketbase.io/docs).

### Connection pool sizing (high concurrency / multiple instances)

The app keeps two connection pools: a **data** pool (application queries) and a
smaller **aux** pool (logs). Their sizes are tunable via env vars (no rebuild):

| Env variable | Default | Pool |
|--------------|---------|------|
| `PB_POSTGRES_DATA_MAX_OPEN_CONNS` | `80` | data — max open connections |
| `PB_POSTGRES_DATA_MAX_IDLE_CONNS` | `15` | data — max idle connections |
| `PB_POSTGRES_AUX_MAX_OPEN_CONNS`  | `10` | aux — max open connections |
| `PB_POSTGRES_AUX_MAX_IDLE_CONNS`  | `3`  | aux — max idle connections |
| `PB_POSTGRES_CONN_MAX_LIFETIME`   | `30m` | max lifetime before a conn is recycled |
| `PB_POSTGRES_CONN_MAX_IDLE_TIME`  | `3m`  | max idle time before an idle conn is closed |
| `PB_POSTGRES_STATEMENT_TIMEOUT`   | `60s` | role-level `statement_timeout` (`ALTER ROLE CURRENT_USER` on boot; `off`/`0` resets) |
| `PB_POSTGRES_LOCK_TIMEOUT`        | `30s` | role-level `lock_timeout` (`ALTER ROLE CURRENT_USER` on boot; `off`/`0` resets) |
| `PB_POSTGRES_DEFAULT_QUERY_EXEC_MODE` | *unset* | pgx client-side query exec mode (`exec`/`simple_protocol` for PgBouncer transaction pooling; see warning above) |

> [!IMPORTANT]
> The effective per-instance ceiling is **`DATA_MAX_OPEN + AUX_MAX_OPEN`**
> (default **90**). This must stay below the PostgreSQL server
> `max_connections`, leaving headroom for superuser/maintenance sessions and
> dividing across every app instance:
>
> ```
> (DATA_MAX_OPEN + AUX_MAX_OPEN) × instances  ≤  max_connections − reserved
> ```
>
> The default 90 fits under the stock Postgres `max_connections=100`. If you run
> **multiple instances** or raise the pool env vars, raise `max_connections`
> accordingly (`postgres -c max_connections=...`; the bundled
> `docker-compose.yml` sets `200`) — and note that pushing many more than a few
> dozen *active* connections at Postgres usually hurts (server-side contention).
> For high fan-out, front Postgres with **PgBouncer** in *transaction* pooling
> mode and keep each app pool small; PgBouncer multiplexes them onto a handful
> of real server connections.
>
> ⚠️ **PgBouncer transaction mode requires setting
> `PB_POSTGRES_DEFAULT_QUERY_EXEC_MODE`.** pgx defaults to `cache_statement`,
> which uses per-connection named prepared statements that PgBouncer
> transaction pooling cannot serve (`prepared statement "stmtcache_..." does
> not exist`). Set it to `exec` (recommended) or `simple_protocol` when fronting
> with PgBouncer in transaction mode. With PgBouncer 1.21+ you can alternatively
> keep `cache_statement` and enable PgBouncer's protocol-level prepared-statement
> support (`max_prepared_statements`).

### Connection idle behavior (POOL-1)

Each PostgreSQL connection is a forked backend (expensive to create), so
`MaxIdleConns` sits below `MaxOpenConns` by default (data 80/15, aux 10/3) to
let the pool shrink during quiet periods — refreshed after `ConnMaxIdleTime`
(3m). On **direct connection** deployments with sustained concurrency this
causes connect/reconnect churn between 15 and 80 as `database/sql` closes
returned connections once idle exceeds `MaxIdleConns`. Two deployment profiles:

- **Direct connection (single/small):** raising `MaxIdleConns` toward
  `MaxOpenConns` (eg. `PB_POSTGRES_DATA_MAX_IDLE_CONNS=80`) eliminates the
  churn; `ConnMaxIdleTime` still drains idle backends during quiet periods.
- **Behind PgBouncer:** keep each app pool **small** (PgBouncer multiplexes)
  and leave idle low — the app connections are cheap sockets to PgBouncer, so
  raising idle gains nothing and only adds sockets to hold.

The defaults are tuned for the multi-instance / pooled profile; adjust per
deployment.

### Query & lock timeouts (server-side backstop)

On boot the app applies `statement_timeout` (default `60s`) and `lock_timeout`
(default `30s`) **at the PostgreSQL role level** via
`ALTER ROLE CURRENT_USER SET ...`. They are a server-side backstop so a runaway
statement or a write blocked on a row lock cannot hold a pooled connection
indefinitely, and they survive PgBouncer transaction pooling (which does not
reliably forward per-client DSN `options`). Applying is best-effort: a role
that cannot alter itself logs a warning and skips. Tune with
`PB_POSTGRES_STATEMENT_TIMEOUT` / `PB_POSTGRES_LOCK_TIMEOUT`; set either to
`off` or `0` to **reset** the role setting (a previous boot's value is not left
behind). Values are deliberately above the app's own 30s
`DefaultQueryTimeout` so legitimate queries are not cut short.

---

## 3b. Auth identity fields & case-insensitive indexes

Password-auth identity fields (eg. `email`, `username`, or any custom text field
listed in `PasswordAuth.IdentityFields`) are always matched case-insensitively
(`LOWER(field) = LOWER(?)`), so each active identity field must carry a
**functional partial unique index** of the form:

```sql
CREATE UNIQUE INDEX "idx_<field>_<id>" ON "<collection>" (LOWER("<field>")) WHERE "<field>" <> ''
```

- The runtime generator (`initIdentityFieldIndexes`) appends this index
  automatically when an auth collection defines a text identity field without
  one.
- The collection validator **rejects** an auth collection whose active identity
  field does not have a `LOWER(...)` unique index (a plain case-sensitive
  index is refused — see `validation_non_functional_identity_unique_index`).
  On a fresh install this is never an issue; it also prevents accidentally
  degrading identity lookups to sequential scans.
- Legacy databases are converted by the `1787237101` migration; SQLite backups
  are normalized during import.

### Removed identity fields ("keep" lifecycle)

Removing a field from `PasswordAuth.IdentityFields` does **not** drop its unique
index. This is intentional (D-2): dropping a unique index is a destructive data
operation better left to a conscious admin decision.

Consequences to be aware of:

- The index keeps enforcing **case-insensitive uniqueness** on that column even
  though it is no longer usable for login — you may still see duplicate errors
  for values that differ only by letter case.
- The lookup path is unaffected (the field is simply not consulted anymore).
- On the save that removes the field, a warning is logged with the index name so
  operators know a now-orphaned unique constraint remains.

To fully drop the constraint, remove the index from the collection's `indexes`
list (via the dashboard or a migration) and let the schema sync rebuild without
it. If you still want case-insensitive uniqueness on a column that is no longer
an identity field, keeping the index is the correct choice — no action needed.

### Note on OAuth2 username uniqueness

The OAuth2 sign-up uniqueness check runs `LOWER(username) = LOWER(?)` without
the `username <> ''` predicate, so it does not use the partial functional index.
This is intentional and mirrors upstream: the check is a secondary guard with
`LIMIT 1` over a bounded set, not a hot login path. The password identity
lookup — the actual hot path — is index-served.

---

## 3c. Cross-instance realtime (outbox)

By default realtime is **single-instance**: record broadcasts are fanned out to
the subscribers connected to the instance that performed the write. To fan a
write out to subscribers on *other* app instances (a **multi-instance**
deployment sharing one database), enable the DB-backed event outbox:

```bash
PB_REALTIME_OUTBOX=1
```

When enabled, each record write appends an event row to `_realtime_outbox`
(`action`, `collection`, `record_id`, `snapshot`) and sends a PostgreSQL
`NOTIFY pb_realtime_outbox` wake-up. Every other instance `LISTEN`s on that
channel (dedicated pgx-native connection), reads the pending events, and
re-broadcasts them to its own local subscribers.

### Design notes

- **Create/update** events store only identifiers; the receiver re-fetches the
  record by id, so the broadcast always carries the latest committed state
  (last-write-wins is automatic — no manual field merge).
- **Delete** events store the **full serialized record snapshot** taken before
  the delete, because a re-fetch is impossible after the delete commits; the
  receiver re-evaluates its local access rules against that snapshot.
- **Broadcast queue, not competing consumers**: every instance processes every
  row for its own local fanout, deduplicating per event id in memory. Rows are
  not ack-marked (a per-row ack would let one instance "consume" an event
  before another read it); stale rows are removed by the hourly TTL cleanup.
- The listener reconnects with a backoff and re-polls pending events after a
  reconnect (catch-up), so brief disconnects do not lose broadcasts to the
  local subscribers.

### Operational notes

- `PB_REALTIME_OUTBOX` defaults to **off**: single-instance deployments have
  zero outbox reads, zero NOTIFY listeners and zero extra rows (the write path
  is unchanged).
- The listener needs a dedicated PostgreSQL connection (LISTEN is not
  multi-plexed by a transaction-pooling proxy); connect it directly or via a
  session-pooling front.
- The outbox table is small (event rows, TTL-cleaned). With the listener
  disabled the rows are not produced at all.

---

## 4. Run the UI (Dev Mode, Hot Reload)

The dashboard is a single-page app written in vanilla JavaScript (a small
custom reactive framework — not React/Svelte/Vue) and bundled by Vite, located
in `ui/`. Run it on a separate Vite dev server for hot reload:

```bash
cd ui
npm run dev
```

| Setting | Value |
|---------|-------|
| UI dev server | `http://localhost:5173` |
| Backend API URL | `http://127.0.0.1:8090` (from `ui/.env.development`) |

The UI is a client-side app. It does **not** rely on a Vite proxy — the PocketBase
JS SDK is initialized with `PB_BACKEND_URL` (`ui/src/pb.js`) and talks to the
backend **directly** (cross-origin, allowed by the backend's default CORS
`--origins *`). So **both servers must run at the same time**, and the backend
must be reachable at the URL in `PB_BACKEND_URL`.

To point the UI at a different backend, create `ui/.env.development.local`
(this file wins over `.env.development`):

```env
PB_BACKEND_URL = "http://127.0.0.1:8090"
```

Edit files under `ui/src/` → the browser auto-reloads.

Open <http://localhost:5173>.

### Stop / kill the dev servers

Normally press **Ctrl+C** in each terminal (backend and UI). If a terminal is
gone and a server is left orphaned — with `go run` the compiled binary lives in
the Go build cache and can reparent to PID 1, so it keeps `:8090` bound and the
next start fails with `address already in use` — stop it by port instead:

```bash
# Backend on :8090 (SIGTERM, equivalent to Ctrl+C)
lsof -ti tcp:8090 -sTCP:LISTEN | xargs kill

# Vite UI dev server on :5173 (if it was running)
lsof -ti tcp:5173 -sTCP:LISTEN | xargs kill
```

> If a process refuses to exit, force it with `-9`, e.g.
> `lsof -ti tcp:8090 -sTCP:LISTEN | xargs kill -9`.

---

## 5. Create a Superuser (Dashboard Login)

The dashboard at `/_/` requires a superuser account. This only needs PostgreSQL
to be running (the migrations are applied automatically); the API server does
not need to be up.

```bash
# Create or update (upsert) a superuser
PB_POSTGRES_PASSWORD=secret go run ./examples/base superuser upsert admin@example.com "changeme123"
```

`superuser` subcommands:

| Command | Purpose |
|---------|---------|
| `upsert <email> <password>` | Create, or update if the email already exists |
| `create <email> <password>` | Create a new superuser (errors if it exists) |
| `update <email> <password>` | Change a superuser's password |
| `delete <email>` | Delete a superuser |
| `otp <email>` | Generate a one-time password for the superuser |
| `ips <ip/cidr ...>` | Limit superuser logins to specific IPs/subnets |

---

## 5b. Schema & data-model notes

### Manual `CREATE INDEX CONCURRENTLY` (IDX-4 escape hatch)

Collection schema changes run inside a transaction, so plain `CREATE INDEX` on a
large populated table takes a `SHARE` lock that blocks **writes** for the build
duration. Post-index-diff, only genuinely changed/added indexes trigger a
rebuild, so this is usually a short window. For exceptional cases (a huge hot
table), a DBA can build the index out-of-band to avoid the write stall:

```sql
CREATE UNIQUE INDEX CONCURRENTLY "idx_foo" ON "my_table" (LOWER("username")) WHERE "username" <> '';
```

Notes:
- `CONCURRENTLY` cannot run inside a transaction; run it directly via `psql`.
- It may leave an `INVALID` index on failure; drop it before retrying
  (`DROP INDEX ...`).
- After building, the index must also be declared in the collection's
  `indexes` JSON so the schema sync treats it as managed (or it will be
  re-created by the sync instead of reused).

### Legacy SQLite-format backup export (TX-1 tradeoff)

The default backup format (`pg`) runs `pg_dump` as an external process with its
own consistent snapshot and is fully safe. The legacy opt-in format
(`Backups.Format = "sqlite"`) wraps its reads in a single transaction to keep
them snapshot-consistent. On PostgreSQL this does **not** block writes (MVCC),
but the transaction pins the xmin horizon for the export duration, which can
slow autovacuum cluster-wide on very large databases, and holds one data-pool
connection. Use the default `pg` format; the legacy format is kept for
SQLite-tooling interoperability.

### Record primary keys (IDX-6, awareness only)

Record ids are random 15-char lowercase strings (fallback `gen_random_bytes`).
As TEXT primary keys they are **not monotonic**, which causes B-tree page
splits on insert, slightly larger relation columns/indexes, and no
time-ordering (the earlier `-@rowid`→`-created` logs bug). This is an
upstream/architectural tradeoff, kept for id-format compatibility; no change is
planned.

---

## 6. Production Build (Single Binary)

The UI is embedded into the Go binary at compile time via `ui/embed.go`
(`//go:embed all:dist`). Therefore the order **matters**:

```bash
# 1. Build the UI first (also runs dprint fmt + vite build → ui/dist)
cd ui
npm run build
cd ..

# 2. Build the binary (entrypoint: examples/base)
go build -o pgbase ./examples/base

# 3. Run it
PB_POSTGRES_PASSWORD=secret ./pgbase serve --http="127.0.0.1:8090"
```

> [!IMPORTANT]
> If you change `ui/src/` but don't run `npm run build` before `go build`, the
> binary will embed the **stale** `ui/dist` — the symptom is a website that
> doesn't reflect your UI changes.

Open <http://127.0.0.1:8090/_/> in the browser.

---

## 7. Docker (Full Stack)

```bash
# Build & start all services (first build is slow)
docker compose up

# Open the dashboard
http://localhost:8090/_/
```

| Service | Port | Function |
|---------|------|----------|
| `pgbase` | `8090` | API + embedded dashboard UI |
| `postgres` | `5432` | Database |

The `pgbase` service takes its PostgreSQL connection settings from the
`PB_POSTGRES_*` environment variables defined in `docker-compose.yml`.

**Create a superuser inside the running stack:**

```bash
docker compose exec pgbase pgbase superuser create admin@example.com "changeme123"
```

> [!NOTE]
> If a local PostgreSQL is already using host port `5432`, change the
> `ports` mapping in `docker-compose.yml` (e.g. `"5433:5432"`).

---

## 8. Migrations

### System migrations (auto-applied)

Schema/DDL migrations live in `migrations/` (e.g.
`1778828400_normalize_indexes.go`). Each file `Register`s a migration into
`core.SystemMigrations` via `init()`:

```go
func init() {
    core.SystemMigrations.Register(func(txApp core.App) error {
        // ...DDL / data changes...
        return nil
    })
}
```

System migrations run **automatically on every app start**
(`core.BaseApp.RunSystemMigrations`), so nothing extra is needed to apply them.

### Generating migration files

Along with system migrations, the app also supports user/app migrations
and automations. The app registers `migratecmd`
(`examples/base/main.go` → `migratecmd.MustRegister(...)`) which adds a
`migrate` CLI command:

```bash
# After building the binary once (section 6)
./pgbase migrate --help
```

Use it to scaffold JS/Go migration templates for your own (non-core) app logic.

---

## 9. Backup & Restore

A backup is a `.zip` of `pb_data` (storage files + a full database dump), stored under `pb_data/backups` (or S3 if configured). The database dump format is controlled by the `Backups.Format` setting:

- **`pg` (default)** — a native, full-fidelity PostgreSQL `pg_dump` archive (`pgdata.dump`) restored via `pg_restore`. It captures the **entire** database: schema, all record tables, settings (`_params`), `_migrations`, `_superusers`, request `_logs` and the partitioned `_audits` / `_audit_reads`. This is an exact-replica disaster-recovery snapshot.
- **`sqlite` (legacy, opt-in)** — a portable v0.23-format SQLite `data.db` for cross-engine portability and older tooling. Records + schema + settings only (no request logs).

Restore auto-detects which dump the archive bundles (`pgdata.dump` → native, `data.db` → legacy SQLite), so old backups keep working.

### Requirements (native `pg` format)

The native path shells out to the `pg_dump` / `pg_restore` client binaries:

- They must be on `PATH` at runtime, and the **client major version must be `>=` the PostgreSQL server major** (`pg_dump` refuses to dump a newer server). The release Docker image ships the v16 client to match the supported server.
- Override the binary locations with `PB_PG_DUMP_BIN` / `PB_PG_RESTORE_BIN` if they live elsewhere.
- If the client is unavailable, set `Backups.Format = "sqlite"` (or pass `--format sqlite`) to fall back to the portable dump.

### CLI

```bash
# create a backup (uses the configured Backups.Format; override per run with --format)
./pgbase backup                       # autogenerated name
./pgbase backup my_snapshot.zip
./pgbase backup my_snapshot.zip --format sqlite

# restore a backup by name — OFFLINE: does NOT restart the process
# (safe to run while the server is stopped; (re)start afterwards to load the data)
./pgbase restore my_snapshot.zip
```

Backups are also created/restored from the dashboard, the `/api/backups` endpoints, and the autobackup cron (`Backups.Cron`).

---

## 10. Running Tests

### Start the test database

```bash
# Option A: Docker (recommended)
docker compose -f tests/docker-compose.test.yml up -d --wait
#   - listens on host port 5433 (so it never clashes with the dev DB on 5432)
#   - runs tests/init-test-db.sql on first boot (enables pgcrypto)

# Option B: Postgres.app
# Make sure pgbase_test + role "test" exist (see Section 2, Option B step 4).
```

### How test isolation works

Each test gets its **own PostgreSQL database**, cloned from a seeded template
via `CREATE DATABASE ... TEMPLATE` (see `tests/app.go`). Packages therefore run
in **parallel deterministically** — `-p 1` is **not** required. `-p 4` bounds
package parallelism so a single heavy package (`core`/`apis`) stays under the
per-package test timeout.

### Environment variables

- The test harness (`tests/app.go`, `tests/db.go`) reads **`PGTEST_`** vars with
  defaults matching the compose service: `localhost:5433`,
  user `test`, password `test`, db `pgbase_test` → usually **no need to set them**.
- A few raw core tests (`base_test`, `log_printer_test`, `system_alert_test`,
  `notify_watcher_test`) connect through the production path and read
  **`PB_POSTGRES_`** vars. Set them to the same test DB as CI does:

```bash
PB_POSTGRES_HOST=localhost \
PB_POSTGRES_PORT=5433 \
PB_POSTGRES_USER=test \
PB_POSTGRES_PASSWORD=test \
PB_POSTGRES_DBNAME=pgbase_test \
go test ./... -count=1 -p 4 -timeout=1200s
```

### Run a single package / specific test

```bash
# Single package
go test ./core/... -count=1

# Single test by name (regex)
go test ./core/... -run TestXxx -count=1 -v
```

### Run non-DB tests only

```bash
go test ./tools/... ./plugins/ghupdate -count=1
```

### Cleanup leftover test databases (optional)

Test databases are dropped automatically, but a per-process template
(`pb_template_<pid>`) is left behind, and crashed/killed runs may leave
`pb_test_*`. They are harmless (each test DB is fully isolated) and are reused
on the next run, but you can reclaim them manually:

```bash
PGPASSWORD=test psql -h localhost -p 5433 -U test -d pgbase_test -tAc \
  "SELECT 'DROP DATABASE IF EXISTS '||datname||' WITH (FORCE);' \
   FROM pg_database WHERE datname LIKE 'pb\_%'" \
  | PGPASSWORD=test psql -h localhost -p 5433 -U test -d pgbase_test
```

### Cleanup test container

```bash
docker compose -f tests/docker-compose.test.yml down -v
```

---

## 11. Lint & Formatting

```bash
# Go linter (requires golangci-lint: https://golangci-lint.run/usage/install/)
make lint
# → golangci-lint run -c ./golangci.yml ./...

# UI formatting (dprint) — also runs automatically on `npm run build`
cd ui && npx dprint fmt && cd ..

# Regenerate JS SDK bindings (rarely needed; keep non-deterministic output in
# mind before committing)
make jstypes
```

Run **all** of the above before opening a PR. See `CONTRIBUTING.md` for the PR
flow.

---

## 12. Makefile Commands

| Command | Description |
|---------|-------------|
| `make build` | Build the `pgbase` binary from `examples/base` |
| `make test` | Start the test PostgreSQL + run the full test suite |
| `make lint` | Run `golangci-lint` with `./golangci.yml` |
| `make migrate` | Run `./pgbase migrate` (requires a built binary) |
| `make superuser` | Run `./pgbase superuser` (requires a built binary) |
| `make docker-build` | Build the Docker image |
| `make docker-run` | `docker compose up` |
| `make docker-stop` | `docker compose down` |
| `make clean` | Remove the `pgbase` binary + Docker volumes |
| `make jstypes` | Regenerate JSVM types |
| `make test-report` | Run tests with coverage + open HTML report |

---

## 13. Project Structure

```
pgbase/
├── apis/               # REST API handlers, routers, middleware
├── cmd/                # CLI commands (serve, superuser)
├── core/               # Core business logic, app bootstrap, DB layer
│   ├── base.go         # App bootstrap, DB init, migrations runner
│   ├── db_connect.go   # PostgreSQL connection
│   ├── db.go           # Model query helpers
│   └── ...
├── forms/              # Form/batch validations & actions
├── migrations/         # System migrations (PostgreSQL DDL)
├── mails/              # Email templates & mailers
├── plugins/            # Optional plugins (jsvm, migratecmd, ghupdate, ...)
├── tools/              # Utilities
│   ├── dbutils/        # SQL dialect, index builder
│   ├── search/         # Search & filter engine
│   └── ...
├── ui/                 # Dashboard frontend (vanilla JS + Vite)
│   ├── src/            # Source code
│   ├── dist/           # Production build (embedded into the binary)
│   ├── embed.go        # Embed ui/dist into the Go binary
│   └── vite.config.js
├── tests/              # Test helpers & fixtures
│   ├── app.go          # TestApp wrapper (per-test DB isolation)
│   ├── docker-compose.test.yml
│   ├── init-test-db.sql
│   └── data/           # Test fixtures (storage)
├── examples/
│   └── base/           # Runnable main entrypoint (go run ./examples/base)
├── third_party/        # Vendored dependencies
├── .github/workflows/  # CI (lint, tests, releases)
├── pocketbase.go       # Main app struct (library)
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

---

## 14. Development Workflow (Checklist)

1. Create a branch from `main` (`git checkout -b my-feature`).
2. Make the change where it belongs:
   - Business logic → `core/`
   - HTTP endpoints/routes → `apis/`
   - Validation/actions → `forms/`
   - Email → `mails/`
   - Dashboard UI → `ui/src/`
3. Add or update tests (standard `testing` package; use the `tests.TestApp`
   harness for anything touching the DB).
4. Run the relevant tests and `make lint` (sections 9 & 10).
5. If you changed the UI, run `npm run build` before building the binary.
6. Open a PR against `main` and follow the contribution notes in
   `CONTRIBUTING.md`.
7. Once merged, cut a release by updating `CHANGELOG.md` and pushing a version
   tag (see section 15).
8. Reference upstream behavior via the [PocketBase docs](https://pocketbase.io/docs)
   — the public API and DB schema are intentionally PocketBase-compatible.

---

## 15. Releasing (Tags & Draft Release)

Releases are **git-tag driven** and produced by
[GoReleaser](https://goreleaser.com) through the `basebuild` workflow
(`.github/workflows/release.yaml`). Pushing a `vX.Y.Z` tag builds the
cross-platform binaries and opens a **draft** GitHub release (a draft so a human
reviews it before publishing).

> [!IMPORTANT]
> The version printed by `pgbase --version` comes from the git tag (injected at
> build time via `-ldflags -X ...Version={{ .Version }}`). Only tags that start
> with `v` (e.g. `v0.2.0`) trigger a release.

### What runs when

| Trigger | What the `basebuild` workflow does |
|---------|-------------------------------------|
| Open / update a PR | Build the UI, start the test Postgres, run the full test suite + 32-bit cross-compile check. **No release.** |
| Push a `vX.Y.Z` tag | The tests above **+** GoReleaser publishes a **draft** GitHub release whose body is the latest `CHANGELOG.md` section. |

> Plain pushes to branches (including `main`) do **not** trigger CI — only PRs
> and `v*` tags do. The PR that introduced a change already ran the full suite
> before it was merged, so re-running on the merge push would be redundant.

### Release flow (PR → merge → draft release)

1. **Open a PR** from your feature branch into `main` and get it merged
   (sections 1–13). CI must be green.

2. **Update `CHANGELOG.md`** on `main`: add a new top section `## vX.Y.Z`
   describing the release. This is the single source of the release notes —
   GoReleaser copies the **top-most** section into the draft release body, so it
   must be updated *before* you tag.

3. **Create an annotated tag** on the up-to-date `main` and push it:

   ```bash
   git checkout main && git pull
   git tag -a v0.2.0 -m "v0.2.0"
   git push origin v0.2.0
   ```

4. **Wait for the workflow.** `basebuild` runs the tests, then GoReleaser builds
   every target and creates a **draft** release with the `## v0.2.0` changelog
   section as its body.

5. **Review & publish.** Open the draft under *GitHub → Releases*, verify the
   notes and assets, then click **Publish**.

> [!WARNING]
> Tags are shared references others may pull, so double-check the tag name and
> that `CHANGELOG.md` is updated **before** pushing. To remove a mistaken
> *local* tag: `git tag -d v0.2.0`. Deleting an already-pushed tag
> (`git push origin :refs/tags/v0.2.0`) also removes the draft release — avoid
> unless truly necessary.

### How the draft release notes are generated

`.goreleaser.yaml` keeps GoReleaser's auto-changelog **disabled**
(`changelog.disable: true`) so the pre-fork PocketBase history is never pulled
in, and `release.draft: true` makes every release a draft. Instead of an
auto-changelog, the workflow extracts the newest `CHANGELOG.md` section into a
file and hands it to GoReleaser via `--release-notes`:

```bash
# runs in CI on tag pushes — prints the body of the first "## " section
awk 'f&&/^## /{exit} /^## /{f=1;next} f' CHANGELOG.md > .release-notes.md
goreleaser release --clean --release-notes=.release-notes.md
```

---

## 16. Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `role "pgbase" does not exist` | Dev role missing | `createuser pgbase` (or recreate the Docker container) |
| `database "pgbase" does not exist` | Dev DB missing | `createdb pgbase` / recreate container |
| `permission denied for schema public` | Owner of DB is not the app role (PG 15+) | `ALTER DATABASE pgbase OWNER TO pgbase;` |
| `Password authentication failed for user "test"` | Wrong test credentials | Set `PGTEST_*` / `PB_POSTGRES_*` to `test` / `test` |
| `connection refused` | PostgreSQL not running | Start Postgres.app / `docker start pgbase-pg`; check the port matches |
| `port 5432: bind: address already in use` | Local PG already on 5432 | Use `-p 5433:5432` for Docker and set `PB_POSTGRES_PORT=5433` |
| `package ... is not a main package` | Ran `go run .` from the root | Use `go run ./examples/base` |
| Binary `pgbase` is not executable | Built from the root package (`go build -o pgbase .`) | `go build -o pgbase ./examples/base` |
| Dashboard shows a stale UI after UI changes | `ui/dist` not rebuilt before `go build` | `cd ui && npm run build && cd ..` then rebuild |
| `getaddrinfo EAI_AGAIN host.docker.internal` | Docker DNS issue | Use `--add-host` or connect to `localhost` directly |
| UI renders but API calls fail in dev | `PB_BACKEND_URL` wrong / backend down | Check `ui/.env.development` (`http://127.0.0.1:8090`) and that the backend is running |
| `gen_random_bytes` not found | `pgcrypto` extension missing | Docker (`init-test-db.sql`) or Postgres.app: `psql -d pgbase_test -c "CREATE EXTENSION IF NOT EXISTS pgcrypto;"` |
| First `serve`/`superuser` hangs on a fresh DB (stalls right after the first `_migrations` log line, `:8090` never opens) | `pgcrypto` not pre-installed → the aux + data bootstrap transactions deadlock on `CREATE EXTENSION` | Provision `pgcrypto` **before** first boot: mount `docker/init` (Section 2 A) or `docker compose` (already mounts it); Postgres.app / existing container: `docker exec pgbase-pg psql -U pgbase -d pgbase -c "CREATE EXTENSION IF NOT EXISTS pgcrypto;"` |
| Tests time out / hang | Parallelism + heavy packages | Use `-count=1 -p 4 -timeout=1200s` (see section 10) |

### Port Reference

| Port | Service | Environment |
|------|---------|-------------|
| `5432` | PostgreSQL | Development / Production |
| `5433` | PostgreSQL (tests) | Testing (via Docker) |
| `8090` | PG-BASE API + UI | Production / Development |
| `5173` | Vite dev server (UI) | Development only |
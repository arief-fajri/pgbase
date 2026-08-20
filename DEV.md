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

# 2. Start PostgreSQL (Docker)
docker run -d --name pgbase-pg \
  -p 5432:5432 \
  -e POSTGRES_DB=pgbase \
  -e POSTGRES_USER=pgbase \
  -e POSTGRES_PASSWORD=secret \
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
  postgres:16-alpine
```

PostgreSQL is now reachable at `localhost:5432`.

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
```

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

## 9. Running Tests

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

## 10. Lint & Formatting

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

## 11. Makefile Commands

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

## 12. Project Structure

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

## 13. Development Workflow (Checklist)

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
   tag (see section 14).
8. Reference upstream behavior via the [PocketBase docs](https://pocketbase.io/docs)
   — the public API and DB schema are intentionally PocketBase-compatible.

---

## 14. Releasing (Tags & Draft Release)

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
| Open / update a PR | Build the UI, start the test Postgres, run the full test suite + 32-bit cross-compile check. **No release.** (A bare push to a feature branch does *not* trigger CI on its own — only once a PR exists.) |
| Push to `main` (no tag) | The tests above **+** a GoReleaser `--snapshot` build (local artifacts only, nothing published). |
| Push a `vX.Y.Z` tag | The tests above **+** GoReleaser publishes a **draft** GitHub release whose body is the latest `CHANGELOG.md` section. |

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

## 15. Troubleshooting

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
| Tests time out / hang | Parallelism + heavy packages | Use `-count=1 -p 4 -timeout=1200s` (see section 9) |

### Port Reference

| Port | Service | Environment |
|------|---------|-------------|
| `5432` | PostgreSQL | Development / Production |
| `5433` | PostgreSQL (tests) | Testing (via Docker) |
| `8090` | PG-BASE API + UI | Production / Development |
| `5173` | Vite dev server (UI) | Development only |
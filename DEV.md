# PG-BASE Development Guide

Step-by-step guide to run PG-BASE in local environment.

---

## Prerequisites

| Tool | Minimal | Check |
|------|---------|-------|
| Go | 1.25+ | `go version` |
| PostgreSQL | 16+ | `psql --version` |
| Node.js | 18+ | `node --version` |
| npm | 9+ | `npm --version` |
| Docker (optional) | 24+ | `docker --version` |

---

## 1. Clone & Install Dependencies

```bash
# Go dependencies
go mod download

# UI dependencies
cd ui
npm install
cd ..
```

---

## 2. Setup PostgreSQL

### Option A: Postgres.app (macOS, Recommended)

1. Open **Postgres.app** → klik **Start**
2. Buka terminal:

```bash
# Create database & user for development
createdb pgbase
createuser pgbase
psql -c "ALTER USER pgbase WITH PASSWORD 'secret';"

# Create database & user for tests
createdb pgbase_test
createuser test
psql -c "ALTER USER test WITH PASSWORD 'test';"
```

PostgreSQL running di `localhost:5432`.

### Option B: Docker

```bash
docker run -d --name pgbase-pg \
  -e POSTGRES_DB=pgbase \
  -e POSTGRES_USER=pgbase \
  -e POSTGRES_PASSWORD=secret \
  -p 5432:5432 \
  postgres:16-alpine
```

### Option C: Docker Compose (full stack)

```bash
docker compose up
```

PostgreSQL + pgbase otomatis running. Langsung ke step 5.

---

## 3. Mode Development (Frontend + Backend Terpisah)

Cara ini memberikan **hot reload** pada UI dashboard — cocok untuk development.

### Terminal 1: Backend (Go API Server)

```bash
# Dari root project
PB_POSTGRES_PASSWORD=secret go run . serve --http="127.0.0.1:8090" --dev
```

Penjelasan:

| Flag | Fungsi |
|------|--------|
| `--http="127.0.0.1:8090"` | Port API server |
| `--dev` | Mode development (log query, dll.) |
| `PB_POSTGRES_PASSWORD=secret` | Password PostgreSQL (env var) |

> API akan running di `http://127.0.0.1:8090`.

### Terminal 2: Frontend (Vite Dev Server)

```bash
cd ui
npm run dev
```

Penjelasan:

| Perintah | Fungsi |
|----------|--------|
| `npm run dev` | Start Vite dev server dengan **hot reload** |

> UI akan running di `http://localhost:5173`.
> Semua request API (`/api/...`) otomatis di-proxy ke `http://127.0.0.1:8090`
> (konfigurasi dari `ui/.env.development`).

### Buka Browser

```
http://localhost:5173
```

Edit file di `ui/src/` → browser auto reload.

---

## 4. Mode Production (Single Binary)

UI sudah ter-embed langsung di binary (`ui/embed.go`). Tidak perlu Vite server.

```bash
# Build binary
go build -o pgbase .

# Run
PB_POSTGRES_PASSWORD=secret ./pgbase serve --http="127.0.0.1:8090"
```

Buka browser:

```
http://127.0.0.1:8090/_/
```

---

## 5. Docker (Full Stack)

```bash
# Start semua service
docker compose up

# Buka browser
http://localhost:8090/_/
```

Docker Compose akan menjalankan:

| Service | Port | Fungsi |
|---------|------|--------|
| `pgbase` | `8090` | API + UI dashboard |
| `postgres` | `5432` | Database |

---

## 6. Run Tests

### Start Test Database

```bash
# Option A: Docker (recommended)
docker compose -f tests/docker-compose.test.yml up -d

# Option B: Postgres.app
# Pastikan database pgbase_test sudah dibuat
```

### Run All Tests

```bash
PB_POSTGRES_HOST=localhost \
PB_POSTGRES_PORT=5433 \
PB_POSTGRES_USER=test \
PB_POSTGRES_PASSWORD=test \
PB_POSTGRES_DBNAME=pgbase_test \
go test ./... -count=1 -timeout=300s
```

### Run Non-DB Tests Saja

```bash
go test ./tools/... ./plugins/ghupdate -count=1
```

### Cleanup Test Container

```bash
docker compose -f tests/docker-compose.test.yml down -v
```

---

## 7. Makefile Commands

| Command | Fungsi |
|---------|--------|
| `make build` | Build binary `pgbase` |
| `make test` | Start test PG + run tests |
| `make docker-build` | Build Docker image |
| `make docker-run` | `docker compose up` |
| `make docker-stop` | `docker compose down` |
| `make clean` | Hapus binary + Docker volume |

---

## 8. Project Structure

```
pgbase/
├── apis/              # REST API handlers
├── cmd/               # CLI commands (serve, superuser)
├── core/              # Core business logic
│   ├── base.go        # App bootstrap, DB init
│   ├── db_connect.go  # PostgreSQL connection
│   ├── db.go          # Model query helpers
│   └── ...
├── migrations/        # System migrations (PostgreSQL DDL)
├── tools/             # Utilities
│   ├── dbutils/       # SQL dialect, index builder
│   ├── search/        # Search & filter engine
│   └── ...
├── ui/                # Dashboard frontend (Shablon + Vite)
│   ├── src/           # Source code
│   ├── dist/          # Production build (embedded)
│   ├── embed.go       # Embed dist ke Go binary
│   └── vite.config.js
├── tests/             # Test helpers
│   ├── app.go         # TestApp wrapper
│   ├── docker-compose.test.yml
│   └── data/          # Test fixtures (storage)
├── pocketbase.go      # Main app struct
├── Dockerfile
├── docker-compose.yml
└── Makefile
```

---

## 9. Troubleshooting

| Error | Penyebab | Solusi |
|-------|----------|--------|
| `role "pgbase" does not exist` | User PostgreSQL belum dibuat | `createuser pgbase` |
| `database "pgbase" does not exist` | Database belum dibuat | `createdb pgbase` |
| `connection refused` | PostgreSQL belum running | Start Postgres.app / `docker start pgbase-pg` |
| `getaddrinfo EAI_AGAIN host.docker.internal` | DNS Docker | Gunakan `--add-host` atau `localhost` langsung |
| UI tidak muncul / error | Backend tidak running atau port salah | Cek `PB_BACKEND_URL` di `ui/.env` / `.env.development` |
| `gen_random_bytes` not found | Extension pgcrypto belum ada | Auto-dibuat oleh migration pertama |

### Port Reference

| Port | Service | Environment |
|------|---------|-------------|
| `5432` | PostgreSQL | Development / Production |
| `5433` | PostgreSQL (test) | Testing (via Docker) |
| `8090` | pgbase API + UI | Production / Development |
| `5173` | Vite dev server (UI) | Development only |

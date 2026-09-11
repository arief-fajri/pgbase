#!/bin/sh
# PG-BASE one-command trial provision (Sprint 0a quickstart).
#
#   curl -fsSL https://raw.githubusercontent.com/arief-fajri/pgbase/main/deploy/quickstart.sh | sh
#
# Boots PG-BASE + PostgreSQL (pgvector-ready) via docker compose in a local
# directory, generates credentials, creates the first superuser and prints
# the working URLs. Idempotent: re-running in the same directory reuses the
# existing .env and stack.
#
# Optional env overrides:
#   PGBASE_DIR             target directory (default: ./pgbase-quickstart)
#   PGBASE_PORT            host port for the app   (default: 8090)
#   PGBASE_VERSION         image tag               (default: latest)
#   PGBASE_SUPERUSER_EMAIL first superuser email   (default: admin@pgbase.local)
#   PGBASE_SKIP_SUPERUSER  set to 1 to skip superuser creation
#
# Requires: docker (Engine 24+), docker compose v2.23.1+, curl.
# Trial stack only — for production see https://arief-fajri.github.io/pgbase/production
set -eu

REPO="arief-fajri/pgbase"
COMPOSE_URL="https://raw.githubusercontent.com/${REPO}/main/deploy/docker-compose.quickstart.yml"
DIR="${PGBASE_DIR:-pgbase-quickstart}"
PORT="${PGBASE_PORT:-8090}"
SUPERUSER_EMAIL="${PGBASE_SUPERUSER_EMAIL:-admin@pgbase.local}"

log() { printf '==> %s\n' "$1"; }
die() { printf 'error: %s\n' "$1" >&2; exit 1; }

# --- random string (hex — safe for compose .env interpolation and shells) ---
rand_string() {
    if command -v openssl >/dev/null 2>&1; then
        openssl rand -hex 16 2>/dev/null && return 0
    fi
    tr -dc 'a-f0-9' </dev/urandom 2>/dev/null | head -c 32
}

# --- prerequisites -----------------------------------------------------------
command -v docker >/dev/null 2>&1 || die "docker is required (https://docs.docker.com/engine/install/)"
docker compose version >/dev/null 2>&1 || die "docker compose v2 is required"
command -v curl >/dev/null 2>&1 || die "curl is required"

# --- target directory --------------------------------------------------------
if [ -f "$DIR/docker-compose.yml" ] || [ -f "$DIR/docker-compose.yaml" ]; then
    log "reusing existing stack in $DIR"
    cd "$DIR"
else
    log "setting up a new trial stack in $DIR"
    mkdir -p "$DIR"
    cd "$DIR"
    log "fetching docker-compose.quickstart.yml"
    curl -fsSL "$COMPOSE_URL" -o docker-compose.yml
fi

# --- credentials -------------------------------------------------------------
# hex charset on purpose: no '$', quotes or escapes to fight in .env
if [ ! -f .env ]; then
    db_password="$(rand_string)"
    [ -n "$db_password" ] || die "could not generate a random password"
    encryption_key="$(rand_string)"
    {
        printf 'PB_POSTGRES_PASSWORD=%s\n' "$db_password"
        printf 'PB_ENCRYPTION_KEY=%s\n' "$encryption_key"
        printf 'PGBASE_VERSION=%s\n' "${PGBASE_VERSION:-latest}"
        printf 'PGBASE_PORT=%s\n' "$PORT"
    } >.env
    chmod 600 .env
    log "generated .env with random database password + encryption key"
else
    log "reusing existing .env"
fi

# --- boot --------------------------------------------------------------------
log "starting PG-BASE + Postgres (first pull may take a minute)"
docker compose up -d --wait

# --- wait for the app --------------------------------------------------------
log "waiting for PG-BASE to answer on 127.0.0.1:$PORT"
ready=0
i=0
while [ "$i" -lt 60 ]; do
    if curl -fsS "http://127.0.0.1:$PORT/api/health" >/dev/null 2>&1; then
        ready=1
        break
    fi
    i=$((i + 1))
    sleep 2
done
[ "$ready" = "1" ] || die "PG-BASE did not become healthy within 120s — check: docker compose logs pgbase"

# --- first superuser ---------------------------------------------------------
if [ "${PGBASE_SKIP_SUPERUSER:-0}" = "1" ]; then
    log "skipping superuser creation (PGBASE_SKIP_SUPERUSER=1)"
    superuser_password=""
else
    superuser_password="$(rand_string)"
    # --encryptionEnv is required on every CLI invocation once the app booted
    # with an encryption key (settings secrets are encrypted with it) — it is
    # a persistent root flag, so it works on subcommands too.
    if docker compose exec -T pgbase pgbase --encryptionEnv=PB_ENCRYPTION_KEY superuser upsert "$SUPERUSER_EMAIL" "$superuser_password" >/dev/null 2>&1; then
        log "superuser ready: $SUPERUSER_EMAIL"
    else
        # non-fatal: the stack is up, the user can create one manually
        log "could not create the superuser automatically — create one with:"
        printf '    docker compose exec pgbase pgbase --encryptionEnv=PB_ENCRYPTION_KEY superuser upsert %s <password>\n' "$SUPERUSER_EMAIL"
        superuser_password=""
    fi
fi

# --- summary -----------------------------------------------------------------
printf '\n'
printf 'PG-BASE is running.\n\n'
printf '  Dashboard : http://127.0.0.1:%s/_/\n' "$PORT"
printf '  API base  : http://127.0.0.1:%s/api/\n' "$PORT"
if [ -n "$superuser_password" ]; then
    printf '  Login     : %s / %s\n' "$SUPERUSER_EMAIL" "$superuser_password"
fi
printf '\n'
printf 'The REST API is PocketBase-compatible — use the PocketBase SDKs and\n'
printf 'docs, plus the fork deltas: https://arief-fajri.github.io/pgbase/fork-deltas\n\n'
printf 'Manage the stack from %s:\n' "$DIR"
printf '  stop          docker compose down\n'
printf '  stop + wipe   docker compose down -v   (deletes ALL data)\n'
printf '  logs          docker compose logs -f pgbase\n'
printf '  psql          docker compose exec postgres psql -U pgbase -d pgbase\n'

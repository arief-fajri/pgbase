#!/usr/bin/env bash
# Shared helpers for the controlled-failure experiment harnesses.
# Sourced by EXPERIMENT-A..E.sh. No side effects beyond env + functions.

set -u

EXP_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$EXP_ROOT/../.." && pwd)"

PGBASE="${PB_EXP_PGBASE:-$REPO_ROOT/pgbase}"
PGHOST="${PB_EXP_PGHOST:-localhost}"
PGPORT="${PB_EXP_PGPORT:-5433}"
PGUSER="${PB_EXP_PGUSER:-test}"
PGPASS="${PB_EXP_PGPASS:-test}"
PGSSLMODE="${PB_EXP_PGSSLMODE:-disable}"
# The test compose project is the directory holding docker-compose.test.yml
# ("tests"); override with PB_EXP_COMPOSE_PROJECT if your stack differs.
COMPOSE_PROJECT="${PB_EXP_COMPOSE_PROJECT:-tests}"
COMPOSE_FILE="$REPO_ROOT/tests/docker-compose.test.yml"
LOG_DIR="$EXP_ROOT/logs"
SESSION="$1"
LOG_FILE="$LOG_DIR/$SESSION-$(date +%Y%m%d-%H%M%S).log"
mkdir -p "$LOG_DIR"

exp_log() {
    printf '%s\n' "$*" | tee -a "$LOG_FILE"
}

require_db() {
    PGPASSWORD="$PGPASS" pg_isready -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" >/dev/null
}

wait_db() {
    for _ in $(seq 1 60); do
        require_db && return 0
        sleep 1
    done
    return 1
}

require_pgbase() {
    [ -x "$PGBASE" ] || {
        exp_log "FAIL: binary not found or not executable: $PGBASE (build: go build -o pgbase ./examples/base)"
        exit 1
    }
}

require_psql() {
    command -v psql >/dev/null || { exp_log "FAIL: psql not on PATH"; exit 1; }
}

PG_ENV_STR() {
    printf 'PB_POSTGRES_HOST=%s PB_POSTGRES_PORT=%s PB_POSTGRES_USER=%s PB_POSTGRES_PASSWORD=%s PB_POSTGRES_DBNAME=%s PB_POSTGRES_SSLMODE=%s' \
        "$PGHOST" "$PGPORT" "$PGUSER" "$PGPASS" "$1" "$PGSSLMODE"
}

run_sql() {
    local db="$1"
    shift
    PGPASSWORD="$PGPASS" psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$db" -X -q -v ON_ERROR_STOP=1 "$@"
}

create_exp_db() {
    local db="$1"
    PGPASSWORD="$PGPASS" psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d postgres -X -q -c "DROP DATABASE IF EXISTS $db;" >/dev/null
    PGPASSWORD="$PGPASS" psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d postgres -X -q -c "CREATE DATABASE $db;" >/dev/null
}

drop_exp_db() {
    local db="$1"
    # terminate any leftover sessions (eg. pooled connections from a restore)
    # so the DROP never fails with "database is being accessed by other users"
    PGPASSWORD="$PGPASS" psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d postgres -X -q -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='$db' AND pid <> pg_backend_pid();" >/dev/null 2>&1
    PGPASSWORD="$PGPASS" psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d postgres -X -q -c "DROP DATABASE IF EXISTS $db;" >/dev/null
}

# provision_db applies all migrations with a SINGLE migration runner before
# the app boots. This is the documented operator path ("pgbase migrate up")
# and keeps each experiment focused on its own phenomenon (pool saturation,
# lock contention, backups, ...) instead of schema bootstrap.
#
# NB! Cold-boot migration itself is now single-connection safe (FAILURE-MODES
# W-10 was fixed: migrations run in one transaction/connection; see
# core/migrations_runner.go + TestColdBootMigrationsSingleConnection).
provision_db() {
    local db="$1"
    local dir="$2"
    PB_POSTGRES_SSLMODE="$PGSSLMODE" PB_POSTGRES_DBNAME="$db" PB_POSTGRES_HOST="$PGHOST" PB_POSTGRES_PORT="$PGPORT" \
        PB_POSTGRES_USER="$PGUSER" PB_POSTGRES_PASSWORD="$PGPASS" \
        "$PGBASE" migrate up --dir "$dir" >>"$LOG_FILE" 2>&1 || return 1
}

stop_pg_container() {
    docker compose -f "$COMPOSE_FILE" -p "$COMPOSE_PROJECT" stop postgres-test >/dev/null 2>&1
}

start_pg_container() {
    docker compose -f "$COMPOSE_FILE" -p "$COMPOSE_PROJECT" start postgres-test >/dev/null
    wait_db
}

wait_http() {
    local url="$1"
    local want="$2"
    for _ in $(seq 1 60); do
        code="$(curl -sS --max-time 2 -o /dev/null -w "%{http_code}" "$url" 2>/dev/null)"
        [ "$code" = "$want" ] && return 0
        sleep 1
    done
    return 1
}

finish() {
    local verdict="$1"
    exp_log "RESULT: $verdict"
    exp_log "LOG: $LOG_FILE"
    [ "$verdict" = "PASS" ]
}
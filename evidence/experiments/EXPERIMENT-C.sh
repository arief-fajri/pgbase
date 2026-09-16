#!/usr/bin/env bash
# EXPERIMENT C — external lock held → lock wait/timeout on app queries.
# Claim: when PostgreSQL blocks the app behind an ACCESS EXCLUSIVE lock,
# queries respect the lock timeout and fail with a bounded error instead of
# blocking forever; when the lock is released the query recovers.
set -u
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh" "EXPERIMENT-C"

exp_log "EXPERIMENT C — lock contention bounded timeout (gate E5, first run L3)"
require_db || { exp_log "FAIL: test DB not up"; exit 1; }
require_pgbase
require_psql
command -v curl >/dev/null || { exp_log "FAIL: curl required"; exit 1; }

DB="pgbase_exp_c"
WORK="$(mktemp -d /var/folders/kf/h0f6mmcj6pqcycr_6t6pz7jh0000gn/T/opencode/exp-c.XXXXXX 2>/dev/null || mktemp -d)"
create_exp_db "$DB"
provision_db "$DB" "$WORK/data" || { exp_log "FAIL: schema provisioning failed"; exit 1; }
exp_log "OK: schema provisioned (migrate up, single runner)"

PB_POSTGRES_SSLMODE="$PGSSLMODE" PB_POSTGRES_DBNAME="$DB" PB_POSTGRES_HOST="$PGHOST" PB_POSTGRES_PORT="$PGPORT" \
    PB_POSTGRES_USER="$PGUSER" PB_POSTGRES_PASSWORD="$PGPASS" \
    "$PGBASE" serve --http=127.0.0.1:8097 --dir "$WORK/data" >"$LOG_DIR/EXPERIMENT-C-app.log" 2>&1 &
APP_PID=$!
trap 'kill $APP_PID 2>/dev/null; drop_exp_db "$DB"; rm -rf "$WORK"' EXIT

wait_http "http://127.0.0.1:8097/api/health" 200 || { exp_log "FAIL: app never ready"; exit 1; }
exp_log "OK: app ready"

START=$(date +%s)
BASE_CODE="$(curl -sS --max-time 10 -o /dev/null -w "%{http_code}" "http://127.0.0.1:8097/api/collections/users/records")"
exp_log "baseline (no lock): http=$BASE_CODE (expect 200)"

# Hold an ACCESS EXCLUSIVE lock on the "users" table so any app SELECT on it
# must wait; "pg_sleep" keeps the lock session alive, simulating a stuck DDL /
# external client context-switching process.
run_sql "$DB" -c "BEGIN; LOCK TABLE users IN ACCESS EXCLUSIVE MODE; SELECT pg_sleep(120); COMMIT;" >/dev/null 2>&1 &
LOCK_PID=$!
sleep 2

START=$(date +%s)
HOLD_CODE="$(curl -sS --max-time 45 -o /dev/null -w "%{http_code}" "http://127.0.0.1:8097/api/collections/users/records")"
HOLD_RC=$?
HOLD_ELAPSED=$(( $(date +%s) - START ))
exp_log "with lock held: curl_rc=$HOLD_RC http=$HOLD_CODE elapsed=${HOLD_ELAPSED}s (expect bounded ~30s failure, not a hang)"

# release the lock by terminating the backend directly (NOT kill $LOCK_PID,
# which only kills the run_sql wrapper subshell and orphans the psql)
run_sql "$DB" -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='$DB' AND query ILIKE '%pg_sleep%' AND pid <> pg_backend_pid();" >/dev/null 2>&1
kill "$LOCK_PID" 2>/dev/null
wait "$LOCK_PID" 2>/dev/null
sleep 2

START=$(date +%s)
RELEASE_CODE="$(curl -sS --max-time 10 -o /dev/null -w "%{http_code}" "http://127.0.0.1:8097/api/collections/users/records")"
RELEASE_ELAPSED=$(( $(date +%s) - START ))
exp_log "after lock release: http=$RELEASE_CODE elapsed=${RELEASE_ELAPSED}s (expect 200)"
kill -0 "$APP_PID" 2>/dev/null && APP_ALIVE=yes || APP_ALIVE=no
exp_log "app alive: $APP_ALIVE"

VERDICT="FAIL"
if [ "$APP_ALIVE" = yes ] && [ "$BASE_CODE" = 200 ] && [ "$HOLD_ELAPSED" -ge 25 ] && [ "$HOLD_ELAPSED" -lt 40 ] && [ "$RELEASE_CODE" = 200 ]; then
    VERDICT="PASS"
fi
finish "$VERDICT"
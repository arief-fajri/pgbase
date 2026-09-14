#!/usr/bin/env bash
# EXPERIMENT A — PG outage → requests → recovery.
# Claim: with the database down, HTTP requests fail with bounded errors (no
# infinite hangs); after the database returns, requests recover deterministically.
set -u
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh" "EXPERIMENT-A"

exp_log "EXPERIMENT A — PG outage -> recovery (gate E5, first run L3)"
require_db || { exp_log "FAIL: test DB not up (docker compose -f tests/docker-compose.test.yml up -d --wait)"; exit 1; }
require_pgbase
command -v curl docker >/dev/null || { exp_log "FAIL: curl/docker required"; exit 1; }

DB="pgbase_exp_a"
WORK="$(mktemp -d /var/folders/kf/h0f6mmcj6pqcycr_6t6pz7jh0000gn/T/opencode/exp-a.XXXXXX 2>/dev/null || mktemp -d)"
create_exp_db "$DB"
provision_db "$DB" "$WORK/data" || { exp_log "FAIL: schema provisioning failed"; exit 1; }
exp_log "OK: schema provisioned (migrate up, single runner)"

BIN="${PGBASE}"
APP_ENV="$(PG_ENV_STR "$DB")"
PB_POSTGRES_SSLMODE="$PGSSLMODE" PB_POSTGRES_DBNAME="$DB" PB_POSTGRES_HOST="$PGHOST" PB_POSTGRES_PORT="$PGPORT" \
    PB_POSTGRES_USER="$PGUSER" PB_POSTGRES_PASSWORD="$PGPASS" \
    "$BIN" serve --http=127.0.0.1:8099 --dir "$WORK/data" >"$LOG_DIR/EXPERIMENT-A-app-before.log" 2>&1 &
APP_PID=$!
trap 'kill $APP_PID 2>/dev/null; drop_exp_db "$DB"; rm -rf "$WORK"' EXIT

# DB-backed guest route (listRule unset -> 200 for guest): proves the app
# reaches PostgreSQL. /api/health is NOT DB-backed (always 200).
PROBE_URL="http://127.0.0.1:8099/api/collections/users/records"
wait_http "$PROBE_URL" 200 || { exp_log "FAIL: app never ready"; exit 1; }
exp_log "OK: app healthy with DB up"

stop_pg_container
sleep 3
START="$(date +%s)"
for i in 1 2 3; do
    START_R="$SECONDS"
    code="$(curl -sS --max-time 8 -o "$LOG_DIR/exp-a-probe-$i.out" -w "%{http_code}" "$PROBE_URL" 2>"$LOG_DIR/exp-a-probe-$i.err")"
    rc=$?
    SECONDS_ELAPSED=$(( SECONDS - START_R ))
    exp_log "probe $i: curl_rc=$rc http=$code elapsed=${SECONDS_ELAPSED}s"
done
TOTAL_ELAPSED=$(( $(date +%s) - START ))
kill -0 "$APP_PID" 2>/dev/null && APP_ALIVE=yes || APP_ALIVE=no
exp_log "app alive after outage: $APP_ALIVE (probe wall ~${TOTAL_ELAPSED}s)"

start_pg_container
sleep 3
wait_http "$PROBE_URL" 200 && RECOVERED=yes || RECOVERED=no
exp_log "recovery: DB-backed route -> $RECOVERED"

VERDICT="FAIL"
if [ "$APP_ALIVE" = yes ] && [ "$RECOVERED" = yes ] && [ "$TOTAL_ELAPSED" -lt 30 ]; then
    VERDICT="PASS"
fi
finish "$VERDICT"
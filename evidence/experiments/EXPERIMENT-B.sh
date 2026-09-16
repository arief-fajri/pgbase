#!/usr/bin/env bash
# EXPERIMENT B — pool saturation under concurrent load.
# Claim: concurrency climbing past the pool ceiling degrades predictably
# (bounded waits followed by completion or clean errors); it never hangs and
# never crashes the process; the wait is observable via metrics.
set -u
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh" "EXPERIMENT-B"

exp_log "EXPERIMENT B — pool saturation (gate E5, first run L3)"
require_db || { exp_log "FAIL: test DB not up"; exit 1; }
require_pgbase
command -v curl >/dev/null || { exp_log "FAIL: curl required"; exit 1; }

DB="pgbase_exp_b"
WORK="$(mktemp -d /var/folders/kf/h0f6mmcj6pqcycr_6t6pz7jh0000gn/T/opencode/exp-b.XXXXXX 2>/dev/null || mktemp -d)"
create_exp_db "$DB"
provision_db "$DB" "$WORK/data" || { exp_log "FAIL: schema provisioning failed"; exit 1; }
exp_log "OK: schema provisioned (migrate up, single runner)"

PB_POSTGRES_SSLMODE="$PGSSLMODE" PB_POSTGRES_DBNAME="$DB" PB_POSTGRES_HOST="$PGHOST" PB_POSTGRES_PORT="$PGPORT" \
    PB_POSTGRES_USER="$PGUSER" PB_POSTGRES_PASSWORD="$PGPASS" \
    PB_POSTGRES_DATA_MAX_OPEN_CONNS=2 PB_POSTGRES_AUX_MAX_OPEN_CONNS=2 \
    PB_METRICS_ADDR="127.0.0.1:9091" \
    "$PGBASE" serve --http=127.0.0.1:8098 --dir "$WORK/data" >"$LOG_DIR/EXPERIMENT-B-app.log" 2>&1 &
APP_PID=$!
trap 'kill $APP_PID 2>/dev/null; drop_exp_db "$DB"; rm -rf "$WORK"' EXIT

wait_http "http://127.0.0.1:8098/api/health" 200 || { exp_log "FAIL: app never ready"; exit 1; }
wait_http "http://127.0.0.1:9091/metrics" 200 || { exp_log "FAIL: metrics listener not ready"; exit 1; }
exp_log "OK: app ready with data pool=2"

CONCURRENCY=32
OUT_DIR="$LOG_DIR/exp-b-out"
mkdir -p "$OUT_DIR"
rm -f "$OUT_DIR"/*
seq 1 "$CONCURRENCY" | xargs -P "$CONCURRENCY" -I{} curl -sS --max-time 30 \
    -o "$OUT_DIR/res-{}.out" -w "{} %{http_code} %{time_total}\n" \
    "http://127.0.0.1:8098/api/collections" >"$OUT_DIR/times.txt" 2>"$OUT_DIR/errors.txt"
DONE_COUNT="$(wc -l <"$OUT_DIR/times.txt")"
ERR_COUNT="$(wc -l <"$OUT_DIR/errors.txt")"

TOTALS="$(awk '{print $2}' "$OUT_DIR/times.txt" | sort | uniq -c | sort -rn)"
MAX_TIME="$(awk '{print $3}' "$OUT_DIR/times.txt" | sort -n | tail -1)"
exp_log "requests completed with bounded response: $DONE_COUNT/$CONCURRENCY"
exp_log "curl-level failures: $ERR_COUNT"
exp_log "http codes: $TOTALS"
exp_log "max response time (s): ${MAX_TIME:-none}"
exp_log "metrics snapshot (pool wait):"
curl -sS --max-time 5 "http://127.0.0.1:8098/metrics" | rg "pgbase_db_wait_count_total|pgbase_db_in_use_connections" | head -6 | tee -a "$LOG_FILE"
kill -0 "$APP_PID" 2>/dev/null && APP_ALIVE=yes || APP_ALIVE=no
exp_log "app alive after saturation: $APP_ALIVE"

VERDICT="FAIL"
if [ "$APP_ALIVE" = yes ] && [ "$DONE_COUNT" -ge "$((CONCURRENCY / 2))" ] && [ "$ERR_COUNT" -eq 0 ] && [ "${MAX_TIME:-999}" != "none" ] && awk -v m="${MAX_TIME:-999}" 'BEGIN{exit !(m < 30)}'; then
    VERDICT="PASS"
fi
finish "$VERDICT"
#!/usr/bin/env bash
# EXPERIMENT E — backup → destroy → restore → verify.
# Claim: a native pg backup round-trips: after the database is destroyed and
# the archive is restored, schema, data, and auth-related state are valid
# (_collections present, params rows present, superuser entry with a password
# hash present). Full auth E2E is covered by the Go backup round-trip test.
set -u
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh" "EXPERIMENT-E"

exp_log "EXPERIMENT E — backup/destroy/restore/verify (gate E5, first run L3)"
require_db || { exp_log "FAIL: test DB not up"; exit 1; }
require_pgbase
require_psql
command -v pg_restore >/dev/null || { exp_log "FAIL: pg_restore required"; exit 1; }

DB="pgbase_exp_e"
WORK="$(mktemp -d /var/folders/kf/h0f6mmcj6pqcycr_6t6pz7jh0000gn/T/opencode/exp-e.XXXXXX 2>/dev/null || mktemp -d)"
create_exp_db "$DB"
provision_db "$DB" "$WORK/data" || { exp_log "FAIL: schema provisioning failed"; exit 1; }
exp_log "OK: schema provisioned (migrate up, single runner)"

APP_PID=""
trap 'kill $APP_PID 2>/dev/null; drop_exp_db "$DB"; rm -rf "$WORK"' EXIT

# NOTE: "$APP_ENV" must be used UNQUOTED so env receives separate VAR=value args
# (quoted, env treats the whole string as one malformed assignment).
APP_ENV="PB_POSTGRES_SSLMODE=$PGSSLMODE PB_POSTGRES_DBNAME=$DB PB_POSTGRES_HOST=$PGHOST PB_POSTGRES_PORT=$PGPORT PB_POSTGRES_USER=$PGUSER PB_POSTGRES_PASSWORD=$PGPASS"

env $APP_ENV "$PGBASE" serve --http=127.0.0.1:8096 --dir "$WORK/data" >"$LOG_DIR/EXPERIMENT-E-app-pre.log" 2>&1 &
APP_PID=$!
wait_http "http://127.0.0.1:8096/api/health" 200 || { exp_log "FAIL: app never ready"; exit 1; }
exp_log "OK: app ready"

env $APP_ENV "$PGBASE" superuser upsert admin@example.com testpass123 --dir "$WORK/data" >>"$LOG_FILE" 2>&1

# deterministic round-trip probe data: a custom _params row (_params has
# "id" (the key) and "value" columns)
run_sql "$DB" -c "INSERT INTO _params (id, value) VALUES ('exp_e_probe', '42');" >>"$LOG_FILE" 2>&1

PRE_COLLECTIONS="$(run_sql "$DB" -t -A -c "SELECT count(*) FROM _collections;")"
PRE_PARAMS="$(run_sql "$DB" -t -A -c "SELECT count(*) FROM _params;")"
PRE_SUPERUSERS="$(run_sql "$DB" -t -A -c "SELECT count(*) FROM _superusers;")"
PRE_HASH_LEN="$(run_sql "$DB" -t -A -c "SELECT length(coalesce(password,'')) FROM _superusers LIMIT 1;")"
exp_log "pre-backup: _collections=$PRE_COLLECTIONS _params=$PRE_PARAMS superusers=$PRE_SUPERUSERS hash_len=$PRE_HASH_LEN"

kill "$APP_PID" 2>/dev/null
wait "$APP_PID" 2>/dev/null
APP_PID=""

env $APP_ENV "$PGBASE" backup exp-e.zip --format pg --dir "$WORK/data" >>"$LOG_FILE" 2>&1
BACKUP_RC=$?
[ -f "$WORK/data/backups/exp-e.zip" ] && BACKUP_OK=yes || BACKUP_OK=no
exp_log "backup: rc=$BACKUP_RC created=$BACKUP_OK"

DROP_SCHEMA="DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
PGPASSWORD="$PGPASS" psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$DB" -X -v ON_ERROR_STOP=1 -c "$DROP_SCHEMA" >>"$LOG_FILE" 2>&1
exp_log "DB destroyed (schema dropped)"

env $APP_ENV "$PGBASE" restore exp-e.zip --dir "$WORK/data" >>"$LOG_FILE" 2>&1
RESTORE_RC=$?
exp_log "restore rc=$RESTORE_RC"

POST_COLLECTIONS="$(run_sql "$DB" -t -A -c "SELECT count(*) FROM _collections;" 2>/dev/null)"
POST_PARAMS="$(run_sql "$DB" -t -A -c "SELECT count(*) FROM _params;" 2>/dev/null)"
POST_PROBE="$(run_sql "$DB" -t -A -c "SELECT count(*) FROM _params WHERE id='exp_e_probe' AND value='42';" 2>/dev/null)"
POST_SUPERUSERS="$(run_sql "$DB" -t -A -c "SELECT count(*) FROM _superusers;" 2>/dev/null)"
POST_HASH_LEN="$(run_sql "$DB" -t -A -c "SELECT length(coalesce(password,'')) FROM _superusers LIMIT 1;" 2>/dev/null)"
exp_log "post-restore: _collections=$POST_COLLECTIONS _params=$POST_PARAMS probe=$POST_PROBE superusers=$POST_SUPERUSERS hash_len=$POST_HASH_LEN"

VERDICT="FAIL"
if [ "$BACKUP_OK" = yes ] && [ "$RESTORE_RC" -eq 0 ] \
   && [ "$POST_COLLECTIONS" = "$PRE_COLLECTIONS" ] && [ "$POST_COLLECTIONS" -gt 0 ] \
   && [ "$POST_PARAMS" = "$PRE_PARAMS" ] && [ "$POST_PROBE" = 1 ] \
   && [ "$POST_SUPERUSERS" -ge 1 ] && [ "$POST_HASH_LEN" -gt 20 ]
then
    VERDICT="PASS"
fi
finish "$VERDICT"
#!/usr/bin/env bash
# EXPERIMENT D — migration fails → restart → verify state.
# Claim: (1) a migrator blocked/aborted by an external lock fails cleanly and
# boundedly; (2) after the failure is removed, re-running migrations on the
# same DB (advisory-lock serialized) reproduces an identical, valid schema;
# (3) a fresh DB migrated by the same binary produces the identical schema
# fingerprint (determinism).
set -u
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/common.sh" "EXPERIMENT-D"

exp_log "EXPERIMENT D — migration failure + state verification (gate E5, first run L3)"
require_db || { exp_log "FAIL: test DB not up"; exit 1; }
require_pgbase
require_psql
command -v pg_dump >/dev/null || { exp_log "FAIL: pg_dump required"; exit 1; }

DB="pgbase_exp_d"
DB2="pgbase_exp_d2"
WORK="$(mktemp -d /var/folders/kf/h0f6mmcj6pqcycr_6t6pz7jh0000gn/T/opencode/exp-d.XXXXXX 2>/dev/null || mktemp -d)"
create_exp_db "$DB"
create_exp_db "$DB2"
trap 'drop_exp_db "$DB"; drop_exp_db "$DB2"; rm -rf "$WORK"' EXIT

MIGRATE_ENV="PB_POSTGRES_SSLMODE=$PGSSLMODE PB_POSTGRES_DBNAME=$DB PB_POSTGRES_HOST=$PGHOST PB_POSTGRES_PORT=$PGPORT PB_POSTGRES_USER=$PGUSER PB_POSTGRES_PASSWORD=$PGPASS"

schema_fingerprint() {
    # pg_dump 18 emits a random "\restrict"/"\unrestrict" pseudo-command; it is
    # a dump artifact (dump scope marker), not schema content, so filter it out
    # to get a deterministic schema fingerprint.
    PGPASSWORD="$PGPASS" pg_dump -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$1" --schema-only 2>/dev/null \
        | rg -v '^\\restrict |^\\unrestrict ' | sha256sum | awk '{print $1}'
}

exp_log "STEP 1: clean migrate + record schema fingerprint"
env $MIGRATE_ENV "$PGBASE" migrate up --dir "$WORK/data" >>"$LOG_FILE" 2>&1
FINGERPRINT_CLEAN="$(schema_fingerprint "$DB")"
exp_log "fingerprint_clean=$FINGERPRINT_CLEAN"

exp_log "STEP 2: block migrations with an external lock -> migrate must fail boundedly"
run_sql "$DB" -c "BEGIN; LOCK TABLE _migrations IN ACCESS EXCLUSIVE MODE; SELECT pg_sleep(120); COMMIT;" >/dev/null 2>&1 &
LOCK_PID=$!
sleep 2
START=$(date +%s)
env $MIGRATE_ENV "$PGBASE" migrate up --dir "$WORK/data" >/dev/null 2>&1
FAIL_RC=$?
FAIL_ELAPSED=$(( $(date +%s) - START ))

# release the lock by terminating the backend directly (NOT kill $LOCK_PID,
# which only kills the run_sql wrapper subshell and orphans the psql)
run_sql "$DB" -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='$DB' AND query ILIKE '%pg_sleep%' AND pid <> pg_backend_pid();" >/dev/null 2>&1
kill "$LOCK_PID" 2>/dev/null
wait "$LOCK_PID" 2>/dev/null

[ "$FAIL_RC" -ne 0 ] && FAIL_CLEANLY=yes || FAIL_CLEANLY=no
exp_log "blocked migrate: rc=$FAIL_RC elapsed=${FAIL_ELAPSED}s (expect ~30s + rc!=0, bounded not hang)"
exp_log "migrate failed cleanly (non-zero rc): $FAIL_CLEANLY"

exp_log "STEP 3: remove the failure, re-run migrate, compare fingerprint"
env $MIGRATE_ENV "$PGBASE" migrate up --dir "$WORK/data" >>"$LOG_FILE" 2>&1
MIGRATE2_RC=$?
FINGERPRINT_RECOVERED="$(schema_fingerprint "$DB")"
exp_log "fingerprint_recovered=$FINGERPRINT_RECOVERED"
[ "$FINGERPRINT_CLEAN" = "$FINGERPRINT_RECOVERED" ] && FINGERPRINT_MATCH=yes || FINGERPRINT_MATCH=no
exp_log "recovered schema fingerprint matches clean: $FINGERPRINT_MATCH"

exp_log "STEP 4: fresh DB, same binary -> identical schema fingerprint (determinism)"
MIGRATE_ENV2="PB_POSTGRES_SSLMODE=$PGSSLMODE PB_POSTGRES_DBNAME=$DB2 PB_POSTGRES_HOST=$PGHOST PB_POSTGRES_PORT=$PGPORT PB_POSTGRES_USER=$PGUSER PB_POSTGRES_PASSWORD=$PGPASS"
env $MIGRATE_ENV2 "$PGBASE" migrate up --dir "$WORK/data2" >>"$LOG_FILE" 2>&1
MIGRATE3_RC=$?
FINGERPRINT_FRESH="$(schema_fingerprint "$DB2")"
exp_log "fingerprint_fresh=$FINGERPRINT_FRESH"
[ "$FINGERPRINT_CLEAN" = "$FINGERPRINT_FRESH" ] && FINGERPRINT_FRESH_MATCH=yes || FINGERPRINT_FRESH_MATCH=no
exp_log "fresh schema fingerprint matches clean: $FINGERPRINT_FRESH_MATCH"

EXPECTED_COLS="$(run_sql "$DB" -t -A -c "SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name ~ '^_' AND table_type='BASE TABLE';")"
exp_log "internal tables (data db): $EXPECTED_COLS"

VERDICT="FAIL"
if [ "$FAIL_CLEANLY" = yes ] && [ "$MIGRATE2_RC" -eq 0 ] && [ "$MIGRATE3_RC" -eq 0 ] \
    && [ "$FINGERPRINT_MATCH" = yes ] && [ "$FINGERPRINT_FRESH_MATCH" = yes ] \
    && [ "$EXPECTED_COLS" = 15 ]; then
    VERDICT="PASS"
fi
finish "$VERDICT"
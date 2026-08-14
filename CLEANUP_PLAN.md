# PG-BASE: SQLite Cleanup Execution Plan (Validated)

## Objective
Remove all SQLite remnants from the PG-BASE codebase.

## Execution
Each task will be executed sequentially. After each task completes, execution pauses for confirmation.

---

## TASK 1: Fix JSON → JSONB Column Type Tests (5 files)
**Fixes:** Update test expectations to match production ColumnType() output.
- `core/field_json_test.go:24`: `"JSON DEFAULT NULL"` → `"JSONB DEFAULT NULL"`
- `core/field_file_test.go:46`: `"JSON DEFAULT '[]' NOT NULL"` → `"JSONB DEFAULT '[]'::jsonb NOT NULL"`
- `core/field_geo_point_test.go:24`: `` `JSON DEFAULT '{"lon":0,"lat":0}' NOT NULL` `` → `` `JSONB DEFAULT '{"lon":0,"lat":0}'::jsonb NOT NULL` ``
- `core/field_relation_test.go:40`: `"JSON DEFAULT '[]' NOT NULL"` → `"JSONB DEFAULT '[]'::jsonb NOT NULL"`
- `core/field_select_test.go:40`: `"JSON DEFAULT '[]' NOT NULL"` → `"JSONB DEFAULT '[]'::jsonb NOT NULL"`
**Risk:** Low

## TASK 2: Fix smallint[] Cast in TableInfoQuery
**File:** `tools/dbutils/pgsql.go:70`
**Change:** `pk.conkey @> ARRAY[c.ordinal_position::int]` → `pk.conkey @> ARRAY[c.ordinal_position::int]::smallint[]`
**Risk:** Medium

## TASK 3: Remove _rowid_ from System Reserved Field Name Tests
**File:** `core/field_test.go:222`
**Change:** `expectError: true` → `expectError: false` for `_rowid_` scenario
**Risk:** Low

## TASK 4: Fix Wrong Collection/Record IDs in Tests
**Files:** `core/record_query_test.go`, `forms/record_upsert_test.go`, `core/field_autodate_test.go`, `core/field_file_test.go`, `apis/record_crud_test.go`
**Changes:**
- `llvuca81nly1qls` (used as record ID) → `0yxhwia2amd8gec`
- `sz5l5z67tg7gku0` (collection ID) → `llvuca81nly1qls`
- `achvryl401bhse3` → `0yxhwia2amd8gec` (or new record ID)
- Fix record count expectations (seed has 1 demo2 record, not 3)
**Risk:** High

## TASK 5: Fix Non-existent Column `active` in RecordQuery Test
**File:** `core/record_query_test.go` (10 occurrences)
**Change:** Replace `dbx.HashExp{"active": true/false}` with valid columns like `title`
**Risk:** Low

## TASK 6: Fix FileField ValidateValue Test
**File:** `core/field_file_test.go`
**Change:** Properly set up file data before MaxSelect validation test
**Risk:** Low-Medium

## TASK 7: Fix Settings Encryption Round-trip
**File:** `core/settings_query_test.go`, `core/settings_query.go`
**Change:** Fix `loadParam` to handle JSONB-wrapped encrypted values (strip JSON quotes before decrypt)
**Risk:** Medium

## TASK 8: Fix Index Name Conflict in migratecmd Test
**File:** `plugins/migratecmd/migratecmd_test.go:850`
**Change:** Rename index `test` → `idx_test123_id`
**Risk:** Low

## TASK 9: Fix Backtick-Quoted Identifiers in Test Assertions
**Files:** `core/view_test.go`, `core/collection_validate_test.go`, `core/collection_import_test.go`
**Change:** Replace backtick quotes with double-quote PG identifiers
**Note:** `apis/record_auth_with_password_test.go` has no backtick issues (removed from scope)
**Risk:** Low

## TASK 10: Clean Up COLLATE Dead Code
**Files:** `tools/dbutils/index.go`, `tools/dbutils/index_test.go`, `core/collection_validate_test.go`, `apis/record_auth_with_password_test.go`
**Change:** Remove COLLATE parsing from regex, remove `Collate` struct field, update tests
**Note:** COLLATE is dead code - parsed but never output by Build()
**Risk:** Low

## TASK 11: Remove Dead Retry Logic
**Files:** `core/db_retry.go`, `core/db_retry_test.go`, `core/db.go`, `core/record_query.go`
**Change:** Inline `baseLockRetry` calls, remove `defaultMaxLockRetries`, remove test file
**Risk:** Low

## TASK 12: Fix VACUUM/ANALYZE Misalignment
**Files:** `core/db_table.go`, `core/app.go`, `core/base.go`, `core/collection_record_table_sync.go`
**Change:** Fix misleading names/comments; rename `Vacuum()` → `Analyze()`, fix log messages
**Risk:** Medium

## TASK 13: Update SQLITE_BUSY Comments
**File:** `plugins/jsvm/internal/types/generated/types.d.ts`
**Change:** Replace "SQLITE_BUSY" with "lock contention"
**Risk:** Low (generated file, accept temporary edit)

## TASK 14: Update UI SQLite Remnants
**Files:** `ui/src/mimeTypes.js`, `ui/src/settings/sql/pageSQLConsole.js`
**Change:** Remove `.sqlite` MIME type, remove `"PRAGMA "` from keywords
**Risk:** Low

## Final Verification
```bash
PB_POSTGRES_HOST=localhost PB_POSTGRES_PORT=5433 PB_POSTGRES_USER=test PB_POSTGRES_PASSWORD=test PB_POSTGRES_DBNAME=pgbase_test go test ./... -count=1 -timeout=300s -p 1
```

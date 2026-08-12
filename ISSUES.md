# Issue Catalog & Fix Plan — PG-BASE

> Generated after running comprehensive tests on PG-BASE fork.

---

## ✅ PASSING (26 packages)

All `tools/*`, `plugins/ghupdate`, `tools/dbutils`, `tools/search` — compiled and all tests pass.

---

## ❌ ISSUES FOUND

### Category A: Test Assertions — Already Fixed

| Issue | File | Fix Applied |
|-------|------|-------------|
| `concurrentDB`/`nonconcurrentDB` field access | `core/log_printer_test.go:58-61` | Removed field access (PG uses single pool) |
| Unused `dbx` import | `core/log_printer_test.go:11` | Removed import |
| Backtick quoting in index tests | `tools/dbutils/index_test.go` | Updated expected strings to double-quote |
| SQLite `json_*` in JSON tests | `tools/dbutils/json_test.go` | Updated to `jsonb_*` PG functions |
| `_rowid_` in sort test | `tools/search/sort_test.go` | Changed to `id` |
| `strftime` → `to_char` in token functions | `tools/search/token_functions_test.go` | Updated expected SQL patterns |

### Category B: Need PostgreSQL to Run (19 packages)

| Package | # Tests | Root Cause |
|---------|---------|------------|
| `./core/validators` | 1 | `TestUniqueId` calls `NewTestApp()` → nil pointer |
| `./core` | ~30+ | Most tests use `tests.NewTestApp()` |
| `./apis` | ~50+ | All integration tests need DB |
| `./mails` | ~5 | Need DB for record creation |
| `./forms` | ~10 | Need DB for record upsert |
| `./plugins/jsvm` | ~15 | Need DB for hook execution |
| `./plugins/migratecmd` | ~5 | Need DB for migrations |

**All fail with same pattern:** `tests.NewTestApp()` → `core.NewBaseApp()` → `initDataDB()` → tries to connect to PostgreSQL → fails → nil pointer panic in `Cleanup()`.

### Category C: Known Code Issues (Not Yet Fixed)

| # | Issue | Severity | File |
|---|-------|----------|------|
| C1 | CI workflow `release.yaml` runs `go test ./...` which will **always fail** without PG | **HIGH** | `.github/workflows/release.yaml` |
| C2 | `tests.NewTestApp()` has no PG connection fallback → `Cleanup()` panics on nil app | **HIGH** | `tests/app.go` |
| C3 | `core/validators/db_test.go` doesn't skip test when PG is unavailable | **MEDIUM** | `core/validators/db_test.go` |
| C4 | `tests/data/data.db`, `auxiliary.db` removed — test data remains in SQLite format | **LOW** | `tests/data/` |
| C5 | No `tests/data/schema.sql` or `seed.sql` for PG test setup | **LOW** | `tests/data/` |

---

## 🛠 FIX PLAN

### Phase 1: Make Tests Graceful Without PostgreSQL (HIGH Priority)

**Problem:** All DB-dependent tests crash instead of skipping gracefully.

```go
// Instead of:
func NewTestApp() (*TestApp, error) { ... }

// We need:
func NewTestApp() (*TestApp, error) {
    if !isPostgresAvailable() {
        return nil, ErrPostgresUnavailable
    }
    ...
}
```

**Files to modify:**
- `tests/app.go` — Add `isPostgresAvailable()` check, return skip-able error
- `core/validators/db_test.go` — Add `t.Skip()` when PG unavailable
- Each test file that calls `tests.NewTestApp()` — Add skip guard

### Phase 2: CI/CD Pipeline (HIGH Priority)

**Problem:** GitHub Actions `release.yaml` runs `go test ./...` which will always fail.

**Fix:** Update workflow to:
1. Start a PostgreSQL service container (`postgres:16-alpine`) in CI
2. Pass PG connection env vars to the test runner
3. Or, split tests into DB-dependent and non-DB-dependent groups

**Files:**
- `.github/workflows/release.yaml` — Add `services.postgres` section

### Phase 3: Test Data for PostgreSQL (MEDIUM Priority)

**Problem:** Test data (`tests/data/`) still uses SQLite format. Need PG-compatible test data.

**Fix:**
- Create `tests/data/schema.sql` — DDL for test tables
- Create `tests/data/seed.sql` — INSERT statements for test records
- Keep `tests/data/storage/` for file storage tests (DB-independent)

**New files:**
- `tests/data/schema.sql`
- `tests/data/seed.sql`

### Phase 4: Remaining SQLite References in Tests (LOW Priority)

Search for remaining `sqlite_master`, `json_each`, `randomblob` patterns in test assertion strings:

```bash
grep -rn "sqlite_master\|randomblob\|json_each\|json_valid\|json_type\|json_array(" \
  --include="*_test.go" .
```

These are in test assertion strings that reference the SQL output format. They need to be updated to match the new PostgreSQL output.

---

## Execution Plan Summary

| Phase | Effort | Dependencies | Parallelizable |
|-------|--------|-------------|----------------|
| P1: Graceful skip for no-PG | 1 day | None | Yes |
| P2: CI/CD pipeline | 0.5 day | P1 | No |
| P3: PG test data | 1 day | None | Yes |
| P4: Remaining test assertions | 2 days | None | Yes |

**Total estimated effort: ~4.5 days**

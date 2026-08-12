# PG-BASE: Comprehensive Rewrite Plan

> Fork PocketBase → Rewrite ke PostgreSQL-only → Rename ke PG-BASE

## Table of Contents

- [Overview](#overview)
- [Phase 0: Setup & Preparation](#phase-0-setup--preparation)
- [Phase 1: Database Connection Layer](#phase-1-database-connection-layer)
- [Phase 2: SQL Dialect Abstraction](#phase-2-sql-dialect-abstraction)
- [Phase 2.5: Index, Quoting & Table Sync](#phase-25-index-quoting--table-sync)
- [Phase 3: Migration System](#phase-3-migration-system)
- [Phase 4: Field System Update](#phase-4-field-system-update)
- [Phase 5: SQL-specific Code Updates](#phase-5-sql-specific-code-updates)
- [Phase 6: Test Infrastructure](#phase-6-test-infrastructure)
- [Phase 7: Configuration & Deployment](#phase-7-configuration--deployment)
- [Phase 8: UI Updates](#phase-8-ui-updates)
- [Phase 9: Documentation & Polish](#phase-9-documentation--polish)
- [File Change Summary](#file-change-summary)
- [Timeline](#timeline)
- [Risk Mitigation](#risk-mitigation)
- [Success Criteria](#success-criteria)
- [Appendix A: File Storage Architecture](#appendix-a-file-storage-architecture)
- [Appendix B: SQLite vs PostgreSQL Reference](#appendix-b-sqlite-vs-postgresql-reference)

---

## Overview

| Item | Detail |
|------|--------|
| **Nama** | PG-BASE |
| **Base** | PocketBase (fork) |
| **Database** | PostgreSQL only (drop SQLite) |
| **UI** | Keep PocketBase dashboard UI |
| **Deployment** | Docker + Binary |
| **Module** | `github.com/arief-fajri/pgbase` |

### Key Principles

1. **PostgreSQL only** - Drop SQLite support completely
2. **Keep UI** - Reuse PocketBase dashboard UI
3. **Dual deployment** - Docker container + standalone binary
4. **API compatible** - REST API endpoints remain the same
5. **Minimal breaking changes** - Keep the developer experience similar

---

## Phase 0: Setup & Preparation

> Estimasi: Day 1-2

### 0.1 Fork & Rename

```bash
# Re-init git untuk fresh start
rm -rf .git
git init
git add .
    git commit -m "Initial commit: PG-BASE fork from PocketBase"
```

- [ ] Rename Go module: `github.com/pocketbase/pocketbase` → `github.com/arief-fajri/pgbase`
- [ ] Rename package references di seluruh codebase
- [ ] Update `go.mod` module path
- [ ] Rename binary/CLI dari "pocketbase" ke "pgbase"
- [ ] Update semua import paths

### 0.2 Dependency Changes

**Hapus:**

| Package | Version | Reason |
|---------|---------|--------|
| `modernc.org/sqlite` | v1.55.0 | SQLite driver |
| `modernc.org/libc` | v1.74.1 | SQLite dependency |
| `modernc.org/mathutil` | v1.7.1 | SQLite dependency |
| `modernc.org/memory` | v1.11.0 | SQLite dependency |
| `github.com/ncruces/go-strftime` | v1.0.0 | SQLite dependency |

**Tambah:**

| Package | Purpose |
|---------|---------|
| `github.com/jackc/pgx/v5` | PostgreSQL driver & connection pool |
| `github.com/jackc/pgx/v5/stdlib` | database/sql compatible driver |
| `github.com/jackc/pgx/v5/pgconn` | Low-level PostgreSQL connection |

**Update:**

- [ ] `github.com/pocketbase/dbx` - pastikan support PostgreSQL (sudah ada `builder_pgsql.go`)

### 0.3 Remove SQLite-specific Files

| File | Reason |
|------|--------|
| `core/db_connect.go` | SQLite DefaultDBConnect |
| `core/db_connect_nodefaultdriver.go` | SQLite build tag |
| `modernc_versions_check.go` | SQLite version check |
| `tests/data/data.db` | SQLite test fixture |
| `tests/data/auxiliary.db` | SQLite test fixture |

---

## Phase 1: Database Connection Layer

> Estimasi: Day 3-5

### 1.1 New Connection Module

**File baru: `core/db_connect.go`**

> ⚠️ **FIX:** Jangan bikin dual pool (pgxpool + dbx.Open). Cukup satu pool via `dbx.Open("pgx", dsn)` + konfigurasi pool limits via `*sql.DB` methods.

```go
package core

import (
    "database/sql"
    "fmt"
    "github.com/pocketbase/dbx"
    _ "github.com/jackc/pgx/v5/stdlib"  // register pgx as database/sql driver
)

// DBConfig defines PostgreSQL connection configuration
type DBConfig struct {
    Host         string
    Port         int
    User         string
    Password     string
    DBName       string
    SSLMode      string
    MaxOpenConns int  // default 100
    MaxIdleConns int  // default 10
}

// DefaultDBConnect creates a new PostgreSQL database connection
func DefaultDBConnect(config DBConfig) (*dbx.DB, error) {
    if config.MaxOpenConns == 0 {
        config.MaxOpenConns = 100
    }
    if config.MaxIdleConns == 0 {
        config.MaxIdleConns = 10
    }

    db, err := dbx.Open("pgx", buildDSN(config))
    if err != nil {
        return nil, fmt.Errorf("failed to open dbx: %w", err)
    }

    // Configure pool via stdlib *sql.DB
    db.DB().SetMaxOpenConns(config.MaxOpenConns)
    db.DB().SetMaxIdleConns(config.MaxIdleConns)
    db.DB().SetConnMaxIdleTime(3 * time.Minute)

    return db, nil
}

func buildDSN(config DBConfig) string {
    sslmode := config.SSLMode
    if sslmode == "" {
        sslmode = "disable"
    }

    return fmt.Sprintf(
        "host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
        config.Host, config.Port, config.User, config.Password,
        config.DBName, sslmode,
    )
}
```

### 1.2 Replace dualDBBuilder — Remove Connection Pool Routing

**File: `core/db_builder.go`** — **HAPUS file ini (186 lines)**

PostgreSQL tidak butuh dual `concurrentDB`/`nonconcurrentDB` pattern. `*dbx.DB` sudah handle connection pooling internally.

**BEFORE (SQLite — `db_builder.go`):**
```go
type dualDBBuilder struct {
    concurrentDB    dbx.Builder  // READ pool (120 conns)
    nonconcurrentDB dbx.Builder  // WRITE pool (1 conn)
}
// NewQuery() routes SELECT→concurrentDB, others→nonconcurrentDB
```

**AFTER (PostgreSQL):**
```go
// Tidak ada dualDBBuilder. Semua method di BaseApp return *dbx.DB langsung.
```

**File: `core/base.go`** — Update struct fields:

```go
// BEFORE:
app.concurrentDB     *dbx.DB   // READ pool
app.nonconcurrentDB  *dbx.DB   // WRITE pool (1 conn)

// AFTER:
app.dataDB           *dbx.DB   // Single connection pool
app.auxDB            *dbx.DB   // Auxiliary DB pool
```

Update methods:
- [ ] `DB()` → return `app.dataDB`
- [ ] `ConcurrentDB()` → return `app.dataDB`
- [ ] `NonconcurrentDB()` → return `app.dataDB`
- [ ] `AuxDB()` → return `app.auxDB`
- [ ] `AuxConcurrentDB()` → return `app.auxDB`
- [ ] `AuxNonconcurrentDB()` → return `app.auxDB`

### 1.3 Remove SQLite Retry Logic from `core/db.go`

**File: `core/db.go`** — PostgreSQL tidak punya `SQLITE_BUSY` error.

```go
// BEFORE (modelQuery — baris ~84):
func (app *BaseApp) modelQuery(db dbx.Builder, m Model) *dbx.SelectQuery {
    return db.
        Select("{{" + tableName + "}}.*").
        From(tableName).
        WithBuildHook(func(query *dbx.Query) {
            query.WithExecHook(execLockRetry(app.config.QueryTimeout, defaultMaxLockRetries))
        })
}

// AFTER:
func (app *BaseApp) modelQuery(db dbx.Builder, m Model) *dbx.SelectQuery {
    return db.
        Select("{{" + tableName + "}}.*").
        From(tableName)
}
```

- [ ] Hapus `execLockRetry` hook dari `modelQuery()`
- [ ] Hapus `baseLockRetry` wrapper dari `create()` / `update()` / `delete()`
- [ ] Simplify/hapus `core/db_retry.go` (PostgreSQL tidak perlu retry untuk lock contention)

### 1.4 Update `initDataDB` / `initAuxDB`

**File: `core/base.go`**

Method signature berubah — tidak pakai file path, tapi DBConfig.

```go
// BEFORE (SQLite):
func (app *BaseApp) initDataDB() error {
    dbPath := filepath.Join(app.DataDir(), "data.db")
    concurrentDB, err := app.config.DBConnect(dbPath)
    nonconcurrentDB, err := app.config.DBConnect(dbPath)
    // ...
}

// AFTER (PostgreSQL):
func (app *BaseApp) initDataDB(config DBConfig) error {
    db, err := DefaultDBConnect(config)
    if err != nil {
        return err
    }
    app.dataDB = db
    return nil
}
```

- [ ] `DataDir` tetap dipertahankan untuk file storage (bukan untuk DB files)
- [ ] Hapus PRAGMA initialization (busy_timeout, journal_mode, dll)
- [ ] Hapus dual pool creation di `initDataDB` dan `initAuxDB`

### 1.5 Connection Config via CLI/Env

**File: `pocketbase.go` + `cmd/serve.go`**

> ⚠️ **FIX:** Credentials hanya dari env vars / CLI flags, jangan simpan di DB settings (bootstrap problem).

```go
type Config struct {
    // Existing fields...

    // PostgreSQL config (only from env/CLI — NOT from DB settings)
    PostgresHost     string
    PostgresPort     int
    PostgresUser     string
    PostgresPassword string
    PostgresDBName   string
    PostgresSSLMode  string

    // Connection pool
    DataMaxOpenConns  int  // default 100
    DataMaxIdleConns  int  // default 10
    AuxMaxOpenConns   int  // default 20
    AuxMaxIdleConns   int  // default 5
}
```

**Environment variables:**

```bash
PB_POSTGRES_HOST=localhost
PB_POSTGRES_PORT=5432
PB_POSTGRES_USER=pgbase
PB_POSTGRES_PASSWORD=secret
PB_POSTGRES_DBNAME=pgbase
PB_POSTGRES_SSLMODE=disable
```

> ⚠️ **Wajib:** Hapus field PostgreSQL dari `Settings` model. Credentials tidak boleh disimpan di database.

---

## Phase 2: SQL Dialect Abstraction

> Estimasi: Day 6-12

### 2.1 Create Dialect Interface

**File baru: `tools/dbutils/dialect.go`**

```go
package dbutils

// Dialect defines database-specific SQL generation
type Dialect interface {
    // JSON functions
    JSONEach(column string) string
    JSONArrayLength(column string) string
    JSONExtract(column, path string) string

    // Table introspection
    TableColumnsQuery() string
    TableInfoQuery() string
    TableIndexesQuery() string
    HasTableQuery() string

    // ID generation
    GenerateIDExpression() string

    // Date/Time functions
    Strftime(column, format string) string

    // Case insensitive comparison
    CollateNocase(column string) string

    // Index quoting
    QuoteIdentifier(name string) string

    // Random for sorting
    RandomExpression() string

    // VACUUM/OPTIMIZE
    OptimizeQuery() string
}
```

### 2.2 PostgreSQL Dialect Implementation

**File baru: `tools/dbutils/pgsql.go`**

```go
package dbutils

import (
    "fmt"
    "strings"
)

// PgSQLDialect implements Dialect for PostgreSQL
type PgSQLDialect struct{}

func (d *PgSQLDialect) JSONEach(column string) string {
    return fmt.Sprintf(
        `jsonb_array_elements(
            CASE WHEN jsonb_typeof([[%s]]) = 'array' 
            THEN [[%s]]::jsonb 
            ELSE jsonb_build_array([[%s]]::text) 
            END
        )`,
        column, column, column,
    )
}

func (d *PgSQLDialect) JSONArrayLength(column string) string {
    return fmt.Sprintf(
        `jsonb_array_length(
            CASE WHEN jsonb_typeof([[%s]]) = 'array' 
            THEN [[%s]]::jsonb 
            ELSE CASE WHEN [[%s]] = '' OR [[%s]] IS NULL 
                 THEN '[]'::jsonb 
                 ELSE jsonb_build_array([[%s]]) 
            END 
            END
        )`,
        column, column, column, column, column,
    )
}

func (d *PgSQLDialect) JSONExtract(column, path string) string {
    if path != "" && !strings.HasPrefix(path, "[") {
        path = "." + path
    }

    return fmt.Sprintf(
        `(CASE WHEN [[%s]] IS NOT NULL AND jsonb_typeof([[%s]]::jsonb) IS NOT NULL 
         THEN [[%s]]::jsonb #>> '%s' 
         ELSE (jsonb_build_object('pb', [[%s]]::text)) #>> '%s' 
         END)`,
        column, column, column, path, column, ".pb"+path,
    )
}

func (d *PgSQLDialect) TableColumnsQuery() string {
    return `SELECT column_name 
            FROM information_schema.columns 
            WHERE table_name = {:tableName} AND table_schema = 'public'
            ORDER BY ordinal_position`
}

func (d *PgSQLDialect) TableInfoQuery() string {
    return `SELECT
               c.ordinal_position - 1 AS cid,
               c.column_name AS name,
               c.data_type AS type,
               (c.is_nullable = 'NO')::boolean AS notnull,
               c.column_default AS dflt_value,
               CASE WHEN pk.contype IS NOT NULL THEN 1 ELSE 0 END AS pk
            FROM information_schema.columns c
            LEFT JOIN pg_constraint pk
                ON pk.conrelid = (quote_ident(c.table_schema) || '.' || quote_ident(c.table_name))::regclass
                AND pk.contype = 'p'
                AND pk.conkey @> ARRAY[c.ordinal_position::int]
            WHERE c.table_name = {:tableName} AND c.table_schema = 'public'
            ORDER BY c.ordinal_position`
}

func (d *PgSQLDialect) TableIndexesQuery() string {
    return `SELECT indexname AS name, indexdef AS sql 
            FROM pg_indexes 
            WHERE tablename = {:tableName} AND schemaname = 'public'`
}

func (d *PgSQLDialect) HasTableQuery() string {
    return `SELECT 1 
            FROM information_schema.tables 
            WHERE LOWER(table_name) = LOWER({:tableName}) 
            AND table_schema = 'public' 
            LIMIT 1`
}

func (d *PgSQLDialect) GenerateIDExpression() string {
    return "DEFAULT ('r' || lower(hex(gen_random_bytes(7))))"
}

func (d *PgSQLDialect) Strftime(column, format string) string {
    pgFormat := convertStrftimeFormat(format)
    return fmt.Sprintf("to_char(%s, '%s')", column, pgFormat)
}

func (d *PgSQLDialect) CollateNocase(column string) string {
    return fmt.Sprintf("LOWER(%s)", column)
}

func (d *PgSQLDialect) QuoteIdentifier(name string) string {
    return fmt.Sprintf(`"%s"`, name)
}

func (d *PgSQLDialect) RandomExpression() string {
    return "RANDOM()"
}

func (d *PgSQLDialect) OptimizeQuery() string {
    return "ANALYZE"
}

// convertStrftimeFormat converts SQLite strftime format to PostgreSQL to_char format
func convertStrftimeFormat(format string) string {
    replacer := strings.NewReplacer(
        "%Y", "YYYY",
        "%m", "MM",
        "%d", "DD",
        "%H", "HH24",
        "%M", "MI",
        "%S", "SS",
        "%f", "US",
        "%w", "D",
        "%j", "DDD",
    )
    return replacer.Replace(format)
}
```

### 2.3 Update Existing JSON Functions

**File: `tools/dbutils/json.go`**

```go
package dbutils

import (
    "fmt"
    "strings"
)

// DefaultDialect is the global SQL dialect (PostgreSQL)
var DefaultDialect Dialect = &PgSQLDialect{}

// SetDialect allows changing the active dialect
func SetDialect(d Dialect) {
    DefaultDialect = d
}

// JSONEach returns the JSON each expression for unnesting arrays
func JSONEach(column string) string {
    return DefaultDialect.JSONEach(column)
}

// JSONArrayLength returns the JSON array length expression
func JSONArrayLength(column string) string {
    return DefaultDialect.JSONArrayLength(column)
}

// JSONExtract returns the JSON extract expression
func JSONExtract(column string, path string) string {
    return DefaultDialect.JSONExtract(column, path)
}
```

### 2.4 Update Table Introspection

**File: `core/db_table.go`**

```go
package core

import (
    "database/sql"
    "fmt"

    "github.com/pocketbase/dbx"
    "github.com/arief-fajri/pgbase/tools/dbutils"
)

// TableColumns returns all column names of a single table by its name
func (app *BaseApp) TableColumns(tableName string) ([]string, error) {
    columns := []string{}

    err := app.DB().NewQuery(dbutils.DefaultDialect.TableColumnsQuery()).
        Bind(dbx.Params{"tableName": tableName}).
        Column(&columns)

    return columns, err
}

type TableInfoRow struct {
    PK           int            `db:"pk"`
    Index        int            `db:"cid"`
    Name         string         `db:"name"`
    Type         string         `db:"type"`
    NotNull      bool           `db:"notnull"`
    DefaultValue sql.NullString `db:"dflt_value"`
}

// ⚠️ Note: cid = ordinal_position - 1 (0-indexed, match SQLite PRAGMA)
// pk = constraint ordinal from pg_constraint (0 = not PK, 1+ = PK position)

// TableInfo returns the column info for the specified table
func (app *BaseApp) TableInfo(tableName string) ([]*TableInfoRow, error) {
    info := []*TableInfoRow{}

    err := app.DB().NewQuery(dbutils.DefaultDialect.TableInfoQuery()).
        Bind(dbx.Params{"tableName": tableName}).
        All(&info)
    if err != nil {
        return nil, err
    }

    if len(info) == 0 {
        return nil, fmt.Errorf("empty table info probably due to invalid or missing table %s", tableName)
    }

    return info, nil
}

// TableIndexes returns a name grouped map with all non empty index of the specified table
func (app *BaseApp) TableIndexes(tableName string) (map[string]string, error) {
    indexes := []struct {
        Name string
        Sql  string
    }{}

    err := app.DB().NewQuery(dbutils.DefaultDialect.TableIndexesQuery()).
        Bind(dbx.Params{"tableName": tableName}).
        All(&indexes)
    if err != nil {
        return nil, err
    }

    result := make(map[string]string, len(indexes))
    for _, idx := range indexes {
        result[idx.Name] = idx.Sql
    }

    return result, nil
}

// hasTable checks if a table or view exists
func (app *BaseApp) hasTable(db dbx.Builder, tableName string) bool {
    var exists int

    err := db.NewQuery(dbutils.DefaultDialect.HasTableQuery()).
        Bind(dbx.Params{"tableName": tableName}).
        Row(&exists)

    return err == nil && exists > 0
}

// Vacuum executes ANALYZE on the data.db to update statistics
func (app *BaseApp) Vacuum() error {
    _, err := app.DB().NewQuery(dbutils.DefaultDialect.OptimizeQuery()).Execute()
    return err
}

// AuxVacuum executes ANALYZE on the auxiliary.db
func (app *BaseApp) AuxVacuum() error {
    _, err := app.AuxDB().NewQuery(dbutils.DefaultDialect.OptimizeQuery()).Execute()
    return err
}
```

### 2.5 Update Record Field Resolver

**File: `core/record_field_resolver_runner.go`**

Replace all `json_each()` patterns:

```go
// BEFORE:
jeTable := fmt.Sprintf("json_each({:%s})", placeholder)

// AFTER:
jeTable := fmt.Sprintf("%s({:%s})", dbutils.DefaultDialect.JSONEach(placeholder), "")
```

**Also update `core/record_query_expand.go`** — `json_each` in EXISTS subquery:

```go
// BEFORE:
q.AndWhere(dbx.Exists(dbx.NewExp(fmt.Sprintf(
    "SELECT 1 FROM %s je WHERE je.value = {:id}",
    dbutils.JSONEach(indirectRelField.Name),
))))

// AFTER:
q.AndWhere(dbx.Exists(dbx.NewExp(fmt.Sprintf(
    `SELECT 1 FROM %s AS je(value) WHERE je.value = {:id}`,
    dbutils.JSONEach(indirectRelField.Name),
))))
```

**Also update `core/record_model.go`** — cascade delete for multi-value relations:

```go
// Same pattern: json_each(...) AS je → jsonb_array_elements(...) AS je(value)
```

**Also update `core/view.go`** — `FindRecordByViewFile()` uses `dbutils.JSONEach`

---

## Phase 2.6: Index & Quoting System

> Estimasi: Day 13-14

### 2.6.1 Update Index Builder

**File: `tools/dbutils/index.go`**

Index.Build() menggunakan backtick quoting (`` `column` ``). PostgreSQL memakai double-quote.

```go
// BEFORE:
fmt.Sprintf("`%s`", name)

// AFTER:
fmt.Sprintf(`"%s"`, strings.ReplaceAll(name, `"`, `""`))
```

- [ ] Replace all backtick quoting → double-quote quoting
- [ ] Handle escaping: `"` → `""` di dalam identifier
- [ ] Verifikasi 63-byte PostgreSQL identifier limit vs 64-char PocketBase truncation

### 2.6.2 Update JSONEach Usage in Subqueries

**Files: `core/record_query_expand.go`, `core/record_model.go`, `core/view.go`**

PostgreSQL `jsonb_array_elements()` adalah set-returning function (SRF), bukan table-valued function seperti `json_each()`. Pattern berbeda:

```sql
-- SQLite (json_each adalah table-valued function):
SELECT 1 FROM json_each(col) AS je WHERE je.value = :id

-- PostgreSQL (jsonb_array_elements adalah SRF, butuh AS alias):
SELECT 1 FROM jsonb_array_elements(col::jsonb) AS je(value) WHERE je.value = :id
```

- [ ] Update ke 3 file di atas

---

## Phase 2.7: Collection Record Table Sync (NEW — Critical)

> Estimasi: Day 15-18

**File: `core/collection_record_table_sync.go`** — ★ **Paling SQLite-dependent dari semua file**

File ini (366 lines) menggunakan SQLite-specific patterns secara ekstensif:

### SQLite Patterns yang Perlu Diganti:

| Pattern | SQLite | PostgreSQL |
|---------|--------|------------|
| View discovery | `SELECT sql FROM sqlite_master WHERE type='view'` | `SELECT pg_get_viewdef(viewname) FROM pg_views WHERE schemaname='public'` |
| JSON validation | `json_valid(col)` | `jsonb_typeof(col::jsonb) IS NOT NULL` |
| JSON type check | `json_type(col)` | `jsonb_typeof(col::jsonb)` |
| JSON array build | `json_array(a)` | `jsonb_build_array(a)` |
| JSON array length | `json_array_length(col)` | `jsonb_array_length(col::jsonb)` |
| JSON extract | `json_extract(col, '$[#-1]')` | `col::jsonb #>> '{-1}'` |
| Conditional | `iif(cond, a, b)` | `CASE WHEN cond THEN a ELSE b END` |
| Optimize | `PRAGMA optimize` | Hapus (PG auto optimize) |
| Identifier quoting | `` `name` `` | `"name"` |

### Fungsi yang Perlu Direwrite:

- `normalizeSingleVsMultipleFieldChanges()` — baris 155-298, paling kompleks
- `SyncRecordTableSchema()` — baris 29-139, schema creation + alteration
- `createCollectionIndexes()` + `dropCollectionIndexes()` — baris 300-366

### Strategi Rewrite:

1. **View save/restore**: Ganti `sqlite_master` dengan `pg_views` + `pg_get_viewdef()`
2. **JSON functions**: Panggil `dbutils.DefaultDialect.JSON*()` methods
3. **Data migration**: Gunakan `UPDATE ... SET col = col::jsonb` untuk konversi tipe
4. **PRAGMA optimize**: Hapus panggilan `PRAGMA optimize` setelah sync (tidak diperlukan di PG)

---

> Estimasi: Day 13-17

### 3.1 Rewrite Initial Migration

**File: `migrations/1640988000_init.go`**

```go
package migrations

import (
    "github.com/arief-fajri/pgbase/core"
)

func init() {
    core.SystemMigrations.Register(func(txApp core.App) error {
        // PostgreSQL-compatible DDL

        // _migrations table (created by migrations_runner.go, but included here for safety)
        _, err := txApp.DB().NewQuery(`
            CREATE TABLE IF NOT EXISTS _migrations (
                file VARCHAR(255) PRIMARY KEY NOT NULL,
                applied BIGINT NOT NULL
            )
        `).Execute()
        if err != nil {
            return err
        }

        // _params table
        _, err := txApp.DB().NewQuery(`
            CREATE TABLE IF NOT EXISTS _params (
                id VARCHAR(255) PRIMARY KEY DEFAULT ('r' || lower(hex(gen_random_bytes(7)))),
                value JSONB,
                created TIMESTAMPTZ DEFAULT NOW(),
                updated TIMESTAMPTZ DEFAULT NOW()
            )
        `).Execute()
        if err != nil {
            return err
        }

        // _collections table
        _, err = txApp.DB().NewQuery(`
            CREATE TABLE IF NOT EXISTS _collections (
                id VARCHAR(255) PRIMARY KEY DEFAULT ('r' || lower(hex(gen_random_bytes(7)))),
                system BOOLEAN DEFAULT FALSE NOT NULL,
                type VARCHAR(255) DEFAULT 'base' NOT NULL,
                name VARCHAR(255) UNIQUE NOT NULL,
                fields JSONB DEFAULT '[]' NOT NULL,
                indexes JSONB DEFAULT '[]' NOT NULL,
                listRule TEXT,
                viewRule TEXT,
                createRule TEXT,
                updateRule TEXT,
                deleteRule TEXT,
                options JSONB DEFAULT '{}' NOT NULL,
                created TIMESTAMPTZ DEFAULT NOW(),
                updated TIMESTAMPTZ DEFAULT NOW()
            );
            CREATE INDEX idx__collections_type ON _collections(type);
        `).Execute()
        if err != nil {
            return err
        }

        // _mfas table
        _, err = txApp.DB().NewQuery(`
            CREATE TABLE IF NOT EXISTS _mfas (
                id VARCHAR(255) PRIMARY KEY DEFAULT ('r' || lower(hex(gen_random_bytes(7)))),
                collectionRef VARCHAR(255) NOT NULL,
                recordRef VARCHAR(255) NOT NULL,
                method VARCHAR(255) NOT NULL,
                created TIMESTAMPTZ DEFAULT NOW(),
                updated TIMESTAMPTZ DEFAULT NOW()
            );
            CREATE INDEX idx_mfas_collectionRef_recordRef ON _mfas(collectionRef, recordRef);
        `).Execute()
        if err != nil {
            return err
        }

        // _otps table
        _, err = txApp.DB().NewQuery(`
            CREATE TABLE IF NOT EXISTS _otps (
                id VARCHAR(255) PRIMARY KEY DEFAULT ('r' || lower(hex(gen_random_bytes(7)))),
                collectionRef VARCHAR(255) NOT NULL,
                recordRef VARCHAR(255) NOT NULL,
                password VARCHAR(255) NOT NULL,
                sentTo VARCHAR(255),
                created TIMESTAMPTZ DEFAULT NOW(),
                updated TIMESTAMPTZ DEFAULT NOW()
            );
            CREATE INDEX idx_otps_collectionRef_recordRef ON _otps(collectionRef, recordRef);
        `).Execute()
        if err != nil {
            return err
        }

        // _externalAuths table
        _, err = txApp.DB().NewQuery(`
            CREATE TABLE IF NOT EXISTS _externalAuths (
                id VARCHAR(255) PRIMARY KEY DEFAULT ('r' || lower(hex(gen_random_bytes(7)))),
                collectionRef VARCHAR(255) NOT NULL,
                recordRef VARCHAR(255) NOT NULL,
                provider VARCHAR(255) NOT NULL,
                providerId VARCHAR(255) NOT NULL,
                created TIMESTAMPTZ DEFAULT NOW(),
                updated TIMESTAMPTZ DEFAULT NOW()
            );
            CREATE UNIQUE INDEX idx_externalAuths_record_provider ON _externalAuths(collectionRef, recordRef, provider);
            CREATE UNIQUE INDEX idx_externalAuths_collection_provider ON _externalAuths(collectionRef, provider, providerId);
        `).Execute()
        if err != nil {
            return err
        }

        // _authOrigins table
        _, err = txApp.DB().NewQuery(`
            CREATE TABLE IF NOT EXISTS _authOrigins (
                id VARCHAR(255) PRIMARY KEY DEFAULT ('r' || lower(hex(gen_random_bytes(7)))),
                collectionRef VARCHAR(255) NOT NULL,
                recordRef VARCHAR(255) NOT NULL,
                fingerprint VARCHAR(255) NOT NULL,
                created TIMESTAMPTZ DEFAULT NOW(),
                updated TIMESTAMPTZ DEFAULT NOW()
            );
            CREATE UNIQUE INDEX idx_authOrigins_unique_pairs ON _authOrigins(collectionRef, recordRef, fingerprint);
        `).Execute()
        if err != nil {
            return err
        }

        // _superusers table
        _, err = txApp.DB().NewQuery(`
            CREATE TABLE IF NOT EXISTS _superusers (
                id VARCHAR(255) PRIMARY KEY DEFAULT ('r' || lower(hex(gen_random_bytes(7)))),
                email VARCHAR(255) UNIQUE NOT NULL,
                password VARCHAR(255) NOT NULL,
                tokenKey VARCHAR(255) DEFAULT '' NOT NULL,
                verified BOOLEAN DEFAULT FALSE NOT NULL,
                created TIMESTAMPTZ DEFAULT NOW(),
                updated TIMESTAMPTZ DEFAULT NOW()
            );
        `).Execute()
        if err != nil {
            return err
        }

        // users table
        _, err = txApp.DB().NewQuery(`
            CREATE TABLE IF NOT EXISTS users (
                id VARCHAR(255) PRIMARY KEY DEFAULT ('r' || lower(hex(gen_random_bytes(7)))),
                email VARCHAR(255) UNIQUE NOT NULL,
                emailVisibility BOOLEAN DEFAULT FALSE NOT NULL,
                username VARCHAR(255) UNIQUE NOT NULL,
                verified BOOLEAN DEFAULT FALSE NOT NULL,
                password VARCHAR(255) NOT NULL,
                name VARCHAR(255) DEFAULT '' NOT NULL,
                avatar VARCHAR(255) DEFAULT '' NOT NULL,
                tokenKey VARCHAR(255) DEFAULT '' NOT NULL,
                created TIMESTAMPTZ DEFAULT NOW(),
                updated TIMESTAMPTZ DEFAULT NOW()
            );
        `).Execute()
        if err != nil {
            return err
        }

        return nil
    }, func(txApp core.App) error {
        // Down migration
        tables := []string{
            "users",
            core.CollectionNameSuperusers,
            core.CollectionNameMFAs,
            core.CollectionNameOTPs,
            core.CollectionNameAuthOrigins,
            "_params",
            "_collections",
        }

        for _, name := range tables {
            if _, err := txApp.DB().NewQuery(
                "DROP TABLE IF EXISTS " + name + " CASCADE",
            ).Execute(); err != nil {
                return err
            }
        }

        return nil
    })
}
```

### 3.2 Rewrite Auxiliary DB Migration

**File: `migrations/1640988000_aux_init.go`**

> ⚠️ **FIX:** `level` tetap `INTEGER` (jangan diubah ke VARCHAR).  
> ⚠️ **FIX:** Tambah `ReapplyCondition` seperti aslinya supaya tabel bisa recreate jika hilang.

```go
func init() {
    core.SystemMigrations.Register(func(txApp core.App) error {
        // _logs table for auxiliary DB
        _, err := txApp.AuxDB().NewQuery(`
            CREATE TABLE IF NOT EXISTS _logs (
                id VARCHAR(255) PRIMARY KEY DEFAULT ('r' || lower(hex(gen_random_bytes(7)))),
                level INTEGER DEFAULT 0 NOT NULL,
                message TEXT DEFAULT '' NOT NULL,
                data JSONB DEFAULT '{}' NOT NULL,
                created TIMESTAMPTZ DEFAULT NOW()
            );
            CREATE INDEX idx__logs_level ON _logs(level);
            CREATE INDEX idx__logs_created ON _logs(created);
            CREATE INDEX idx__logs_created_hour ON _logs(to_char(created, 'YYYY-MM-DD HH24:00:00'));
        `).Execute()

        return err
    }, func(txApp core.App) error {
        _, err := txApp.AuxDB().NewQuery("DROP TABLE IF EXISTS _logs").Execute()
        return err
    }, core.SystemMigrations.ReapplyCondition(func(txApp core.App, runner *core.MigrationsRunner, fileName string) (bool, error) {
        // Reapply if _logs table is missing
        var exists int
        err := txApp.AuxDB().NewQuery(
            "SELECT 1 FROM information_schema.tables WHERE table_name = '_logs' AND table_schema = 'public' LIMIT 1",
        ).Row(&exists)
        return err != nil || exists == 0, nil
    }))
}
```

### 3.3 Update All Other Migrations

**Files: `migrations/1717233556_v0.23_migrate.go` etc.**

Updates needed per file:
- [ ] `randomblob` → `gen_random_bytes`
- [ ] `strftime` → `to_char`
- [ ] `COLLATE NOCASE` → `LOWER()` or `::citext`
- [ ] `sqlite_master` → `pg_indexes` / `information_schema`
- [ ] Backtick quoting → double-quote quoting
- [ ] `iif()` → `CASE WHEN ... THEN ... ELSE ... END`

### 3.4 Update Migrations Runner

**File: `core/migrations_runner.go`**

```go
func (app *BaseApp) lastAppliedMigrations() ([]MigrationRecord, error) {
    var result []MigrationRecord
    // Remove the substr||'0000000000000000' hack
    // PostgreSQL handles integer ordering natively
    err := app.DB().NewQuery(
        "SELECT file, applied FROM {{_migrations}} ORDER BY applied DESC",
    ).All(&result)
    return result, err
}
```

---

## Phase 4: Field System Update

> Estimasi: Day 18-22

### 4.1 Update Field ColumnType Methods

**File: `core/field_text.go`**  
**File: `core/field_json.go`**  
**File: `core/field_file.go`**  
**File: `core/field_select.go`**  
**File: `core/field_relation.go`**  
**File: `core/field_geo_point.go`**  
**(Total: ~12 field type files)**

> ⚠️ **FIX:** Signature tetap `ColumnType(app App)` — jangan diubah ke `dbx.Builder`. Parameter `app` tidak dipakai saat ini tapi ada untuk future extensibility.

**All field type ColumnType changes:**

| Field | SQLite | PostgreSQL |
|-------|--------|------------|
| TextField (PK) | `TEXT PRIMARY KEY DEFAULT ('r'...randomblob(7)...)` | `TEXT PRIMARY KEY DEFAULT ('r'...gen_random_bytes(7)...)` |
| TextField (non-PK) | `TEXT DEFAULT '' NOT NULL` | `TEXT DEFAULT '' NOT NULL` |
| JSONField | `JSON DEFAULT NULL` | `JSONB DEFAULT NULL` |
| GeoPointField | `JSON DEFAULT '{"lon":0,"lat":0}' NOT NULL` | `JSONB DEFAULT '{"lon":0,"lat":0}'::jsonb NOT NULL` |
| SelectField (multi) | `JSON DEFAULT '[]' NOT NULL` | `JSONB DEFAULT '[]'::jsonb NOT NULL` |
| FileField (multi) | `JSON DEFAULT '[]' NOT NULL` | `JSONB DEFAULT '[]'::jsonb NOT NULL` |
| RelationField (multi) | `JSON DEFAULT '[]' NOT NULL` | `JSONB DEFAULT '[]'::jsonb NOT NULL` |
| BoolField | `BOOLEAN DEFAULT FALSE NOT NULL` | `BOOLEAN DEFAULT FALSE NOT NULL` ✅ |
| NumberField | `NUMERIC DEFAULT 0 NOT NULL` | `NUMERIC DEFAULT 0 NOT NULL` ✅ |
| AutodateField | `TEXT DEFAULT '' NOT NULL` | `TEXT DEFAULT '' NOT NULL` *(opsional: bisa tetap TEXT dulu)* |

### 4.2 Update `_rowid_` References

**File: `tools/search/provider.go`**

```go
// BEFORE:
const rowidSortKey = "_rowid_"

// AFTER:
const rowidSortKey = "id"  // Use primary key instead

// Also update:
// apis/record_crud.go:84
searchProvider.CountCol("id")  // instead of "_rowid_"
```

### 4.3 Update Sort System

**File: `tools/search/sort.go`**

```go
func (f *SortField) BuildExpr(table string) string {
    if f.Name == rowidSortKey {
        return fmt.Sprintf("[[%s.%s]] %s", table, f.Name, f.Direction)
    }
    // ... rest unchanged
}
```

### 4.4 Update Reserved Field Names

**File: `core/field.go`**

```go
var excludeNames = append([]any{
    "null", "true", "false",
    // "_rowid_" — dihapus, PostgreSQL tidak punya rowid
}, list.ToInterfaceSlice(SystemDynamicFieldNames)...)
```

---

## Phase 5: SQL-specific Code Updates

> Estimasi: Day 23-30

### 5.1 Update PRAGMA References

**File: `core/base.go`**

```go
// BEFORE (SQLite PRAGMAs in cron):
app.Cron().Add("__pbDBOptimize__", "0 0 * * *", func() {
    app.NonconcurrentDB().NewQuery("PRAGMA wal_checkpoint(TRUNCATE)").Execute()
    app.NonconcurrentDB().NewQuery("PRAGMA optimize").Execute()
})

// AFTER (PostgreSQL):
app.Cron().Add("__pbDBOptimize__", "0 0 * * *", func() {
    app.DB().NewQuery("VACUUM").Execute()
})
```

**File: `core/base_backup.go`**

- [ ] Remove `PRAGMA wal_checkpoint(TRUNCATE)` calls
- [ ] PostgreSQL handles WAL management automatically

### 5.2 Update COLLATE NOCASE

**Files affected: 5 locations + auth logic restructure**

> ⚠️ **FIX:** Bukan cuma replace string. Ada **conditional logic** perlu diubah.  
> Di `core/record_query.go:565`, `apis/record_auth_with_password.go:151`, `apis/record_auth_with_oauth2.go:243` — kode mengecek `index.Columns[0].Collate == "nocase"` untuk decide pakai case-insensitive atau exact match. PostgreSQL tidak punya metadata COLLATE di index.  
> **Solusi:** Hapus conditional check. Selalu pakai case-insensitive comparison untuk email/username.

```go
// BEFORE (record_query.go ~565):
if ok && strings.EqualFold(index.Columns[0].Collate, "nocase") {
    expr = dbx.NewExp("[[email]] = {:email} COLLATE NOCASE", ...)
} else {
    expr = dbx.HashExp{Email: email}
}

// AFTER (PostgreSQL — always case-insensitive):
expr = dbx.NewExp("LOWER([[email]]) = LOWER({:email})", dbx.Params{"email": email})
```

**Same pattern applies to:**
- [ ] `core/record_query.go:565` — `FindAuthRecordByEmail`
- [ ] `apis/record_auth_with_password.go:151` — `findRecordByIdentityField`
- [ ] `apis/record_auth_with_oauth2.go:243` — `oldCanAssignUsername`
- [ ] `core/field_text.go:209` — `ValidatePlainValue` (PK uniqueness)
- [ ] `migrations/1717233556_v0.23_migrate.go:498` — index creation

### 5.3 Cleanup sqlite_master References

**5 production files + 4 test files** masih menggunakan `sqlite_master` / `sqlite_schema`.

| File | Line | SQLite | PostgreSQL |
|------|------|--------|------------|
| `core/db_table.go` | 63 | `FROM sqlite_master` | `FROM information_schema.tables` |
| `core/db_table.go` | 114 | `FROM sqlite_schema` | `FROM pg_indexes` |
| `core/collection_record_table_sync.go` | 191 | `FROM sqlite_master` | `FROM pg_views` |
| `core/collection_validate.go` | 566 | `FROM sqlite_master` | `FROM information_schema.views` |
| `migrations/1778828400_normalize_indexes.go` | 32 | `FROM sqlite_master` | `FROM pg_indexes` |

- [ ] Update semua 5 lokasi produksi
- [ ] Update test assertions yang mengandung `sqlite_master`

### 5.4 Cleanup PRAGMA References

**11 PRAGMA references di 3 files:**

| File | Line | PRAGMA | Action |
|------|------|--------|--------|
| `core/db_table.go` | 14 | `PRAGMA_TABLE_INFO` | → `information_schema.columns` (Phase 2.4) |
| `core/db_table.go` | 37 | `PRAGMA_TABLE_INFO` | → `information_schema.columns` (Phase 2.4) |
| `core/base_backup.go` | 87-88 | `wal_checkpoint(TRUNCATE)` | Hapus (PG auto WAL) |
| `core/base.go` | 1361,1366 | `wal_checkpoint(TRUNCATE)` | Hapus (PG auto WAL) |
| `core/base.go` | 1371 | `PRAGMA optimize` | → `ANALYZE` |
| `core/collection_record_table_sync.go` | 147 | `PRAGMA optimize` | Hapus |

### 5.5 Update strftime → to_char

**File: `core/log_query.go`**

```go
// BEFORE:
"strftime('%Y-%m-%d %H:00:00', created) as date"

// AFTER:
"to_char(created, 'YYYY-MM-DD HH24:00:00') as date"
```

**File: `tools/search/token_functions.go`**

Update strftime function to use to_char with format conversion:
- `%Y` → `YYYY`
- `%m` → `MM`
- `%d` → `DD`
- `%H` → `HH24`
- `%M` → `MI`
- `%S` → `SS`

### 5.6 Update Index Quoting

**File: `tools/dbutils/index.go`**

```go
// BEFORE (SQLite backticks):
"`column_name`"

// AFTER (PostgreSQL double quotes):
`"column_name"`
```

### 5.7 Update View Creation

**File: `core/view.go`**

```go
func (app *BaseApp) SaveView(name, selectQuery string) error {
    // DROP VIEW IF EXISTS first
    app.DB().NewQuery(fmt.Sprintf("DROP VIEW IF EXISTS %s", name)).Execute()

    // CREATE VIEW
    _, err := app.DB().NewQuery(fmt.Sprintf(
        "CREATE VIEW %s AS %s",
        name, selectQuery,
    )).Execute()
    return err
}
```

### 5.8 Remove SQLite-specific Retry Logic

**File: `core/db_retry.go`**

```go
// PostgreSQL doesn't have SQLITE_BUSY errors
// Simplify or remove the retry logic
func baseLockRetry(op func(attempt int) error, maxRetries int) error {
    return op(1)  // No retry needed for PostgreSQL
}
```

### 5.9 Update SQL API Endpoint

**File: `apis/sql.go`**

- [ ] Remove SQLite-specific comments
- [ ] Update any SQLite-specific error handling
- [ ] Query execution is already dbx-based (database-agnostic)

### 5.3 Update strftime → to_char

**File: `core/log_query.go`**

```go
// BEFORE:
"strftime('%Y-%m-%d %H:00:00', created) as date"

// AFTER:
"to_char(created, 'YYYY-MM-DD HH24:00:00') as date"
```

**File: `tools/search/token_functions.go`**

Update strftime function to use to_char with format conversion:
- `%Y` → `YYYY`
- `%m` → `MM`
- `%d` → `DD`
- `%H` → `HH24`
- `%M` → `MI`
- `%S` → `SS`

### 5.4 Update Index Quoting

**File: `tools/dbutils/index.go`**

```go
// BEFORE (SQLite backticks):
"`column_name`"

// AFTER (PostgreSQL double quotes):
`"column_name"`
```

### 5.5 Update View Creation

**File: `core/view.go`**

```go
func (app *BaseApp) SaveView(name, selectQuery string) error {
    // DROP VIEW IF EXISTS first
    app.DB().NewQuery(fmt.Sprintf("DROP VIEW IF EXISTS %s", name)).Execute()

    // CREATE VIEW
    _, err := app.DB().NewQuery(fmt.Sprintf(
        "CREATE VIEW %s AS %s",
        name, selectQuery,
    )).Execute()
    return err
}
```

### 5.6 Remove SQLite-specific Retry Logic

**File: `core/db_retry.go`**

```go
// PostgreSQL doesn't have SQLITE_BUSY errors
// Simplify or remove the retry logic
func baseLockRetry(op func(attempt int) error, maxRetries int) error {
    return op(1)  // No retry needed for PostgreSQL
}
```

### 5.7 Update SQL API Endpoint

**File: `apis/sql.go`**

- [ ] Remove SQLite-specific comments
- [ ] Update any SQLite-specific error handling
- [ ] Query execution is already dbx-based (database-agnostic)

---

## Phase 6: Test Infrastructure

> Estimasi: Day 31-37

### 6.1 Create PostgreSQL Test Setup

> ⚠️ **ISOLASI:** Tests sekarang sharing satu PostgreSQL. Gunakan schema-per-test isolation:
> - `CREATE SCHEMA test_<random_id>`
> - `SET search_path TO test_<random_id>`
> - `DROP SCHEMA test_<random_id> CASCADE` di cleanup
>  
> Alternatif: `go test -parallel 1` (serial execution)

**File baru: `tests/db.go`**

```go
package tests

import (
    "testing"
    "github.com/arief-fajri/pgbase/core"
)

func SetupTestDB(t *testing.T) *core.BaseApp {
    t.Helper()

    config := core.BaseAppConfig{
        DBConnect: func(dbPath string) (*dbx.DB, error) {
            return core.DefaultDBConnect(core.DBConfig{
                Host:     getEnvOrDefault("PGTEST_HOST", "localhost"),
                Port:     5433,
                User:     getEnvOrDefault("PGTEST_USER", "test"),
                Password: getEnvOrDefault("PGTEST_PASSWORD", "test"),
                DBName:   getEnvOrDefault("PGTEST_DBNAME", "pgbase_test"),
                SSLMode:  "disable",
            })
        },
        DataDir: t.TempDir(),
    }

    app := core.NewBaseApp(config)
    if err := app.Bootstrap(); err != nil {
        t.Fatalf("Failed to bootstrap: %v", err)
    }

    // Run migrations
    if err := app.RunAllMigrations(); err != nil {
        t.Fatalf("Failed to run migrations: %v", err)
    }

    return app
}

func CleanupTestDB(t *testing.T, app core.App) {
    t.Helper()

    // Truncate all tables (termasuk _migrations untuk reset state)
    tables := []string{
        "users", "_superusers", "_authOrigins",
        "_otps", "_mfas", "_externalAuths",
        "_params", "_collections", "_migrations",
    }

    for _, table := range tables {
        app.DB().NewQuery("TRUNCATE " + table + " CASCADE").Execute()
    }
}
```

### 6.2 Update Test Helpers

**File: `tests/app.go`**

```go
func NewTestApp() (*TestApp, error) {
    // Connect to PostgreSQL test database
    config := core.BaseAppConfig{
        DBConnect: func(dbPath string) (*dbx.DB, error) {
            return core.DefaultDBConnect(core.DBConfig{
                Host:     "localhost",
                Port:     5433,
                User:     "test",
                Password: "test",
                DBName:   "pgbase_test",
                SSLMode:  "disable",
            })
        },
        DataDir: os.TempDir(),
    }

    app := core.NewBaseApp(config)
    if err := app.Bootstrap(); err != nil {
        return nil, err
    }

    // Run migrations
    if err := app.RunAllMigrations(); err != nil {
        return nil, err
    }

    // Seed test data
    if err := seedTestData(app); err != nil {
        return nil, err
    }

    return &TestApp{App: app}, nil
}
```

### 6.3 Create Test Database Docker Setup

**File baru: `tests/docker-compose.test.yml`**

```yaml
version: '3.8'
services:
  postgres-test:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: pgbase_test
      POSTGRES_USER: test
      POSTGRES_PASSWORD: test
    ports:
      - "5433:5432"
    volumes:
      - postgres_test_data:/var/lib/postgresql/data

volumes:
  postgres_test_data:
```

### 6.4 Update Test Data

- [ ] Buat `tests/data/schema.sql` - schema creation script
- [ ] Buat `tests/data/seed.sql` - test data insert script
- [ ] Update `tests/data/storage/` - file storage tests (independent of DB)

---

## Phase 7: Configuration & Deployment

> Estimasi: Day 38-42

### 7.1 Configuration System

**File: `core/settings_model.go`**

> ⚠️ **JANGAN simpan PostgreSQL credentials di Settings model.**  
> Ada bootstrap problem: butuh password untuk connect ke DB, tapi password ada di DB.  
> Credentials hanya dari environment variables / CLI flags.

```go
// JANGAN tambah PostgreSQLSettings ke Settings struct.
// Settings model tetap sama seperti PocketBase original.
// Credentials hanya dari env vars:
//   PB_POSTGRES_HOST, PB_POSTGRES_PORT, PB_POSTGRES_USER,
//   PB_POSTGRES_PASSWORD, PB_POSTGRES_DBNAME, PB_POSTGRES_SSLMODE
```

### 7.2 CLI Commands Update

**File: `cmd/serve.go`**

```go
command.PersistentFlags().StringVar(&pgHost, "pg-host", "", "PostgreSQL host")
command.PersistentFlags().IntVar(&pgPort, "pg-port", 5432, "PostgreSQL port")
command.PersistentFlags().StringVar(&pgUser, "pg-user", "", "PostgreSQL user")
command.PersistentFlags().StringVar(&pgPassword, "pg-password", "", "PostgreSQL password")
command.PersistentFlags().StringVar(&pgDBName, "pg-dbname", "", "PostgreSQL database name")
command.PersistentFlags().StringVar(&pgSSLMode, "pg-sslmode", "disable", "PostgreSQL SSL mode")
```

### 7.3 Docker Setup

**File baru: `Dockerfile`**

```dockerfile
# Build stage
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o pgbase .

# Runtime stage
FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata postgresql17-client
COPY --from=builder /app/pgbase /usr/local/bin/
EXPOSE 8090
ENTRYPOINT ["pgbase"]
CMD ["serve", "--http", "0.0.0.0:8090"]
```

**File baru: `docker-compose.yml`**

```yaml
version: '3.8'
services:
  pgbase:
    build: .
    ports:
      - "8090:8090"
    environment:
      - PB_POSTGRES_HOST=postgres
      - PB_POSTGRES_PORT=5432
      - PB_POSTGRES_USER=pgbase
      - PB_POSTGRES_PASSWORD=secret
      - PB_POSTGRES_DBNAME=pgbase
      - PB_POSTGRES_SSLMODE=disable
    volumes:
      - pb_data:/pb_data
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: pgbase
      POSTGRES_USER: pgbase
      POSTGRES_PASSWORD: secret
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U pgbase"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  pb_data:
  postgres_data:
```

### 7.4 Makefile Update

**File: `Makefile`**

```makefile
.PHONY: build test docker-build docker-run migrate superuser clean

# Build
build:
	go build -o pgbase .

# Test
test:
	docker-compose -f tests/docker-compose.test.yml up -d
	sleep 3
	go test ./...
	docker-compose -f tests/docker-compose.test.yml down

# Docker
docker-build:
	docker build -t pgbase .

docker-run:
	docker-compose up

docker-stop:
	docker-compose down

# Database
migrate:
	./pgbase migrate

superuser:
	./pgbase superuser

# Clean
clean:
	rm -f pgbase
	docker-compose down -v
```

---

## Phase 8: UI Updates

> Estimasi: Day 43-45

### 8.1 Update UI Build

**Directory: `ui/`**

- [ ] Update API endpoint references (jika ada)
- [ ] Update configuration display untuk PostgreSQL settings
- [ ] Test dashboard functionality dengan PostgreSQL backend

### 8.2 Update Installer

**File: `apis/installer.go`**

```go
func DefaultInstallerFunc(app core.App, baseURL string) error {
    // Update untuk PostgreSQL connection setup
    // Tampilkan PostgreSQL connection status
}
```

---

## Phase 9: Documentation & Polish

> Estimasi: Day 46-50

### 9.1 README Update

**File: `README.md`**

```markdown
# PG-BASE

PostgreSQL-powered backend as a service — Fork of PocketBase.

## Features

- REST API
- Real-time subscriptions
- Authentication & authorization
- File storage (local & S3)
- Dashboard UI
- **PostgreSQL database** (replaces SQLite)

## Quick Start

### Docker (Recommended)

```bash
docker-compose up
```

### Binary

```bash
./pgbase serve \
  --pg-host localhost \
  --pg-port 5432 \
  --pg-user pgbase \
  --pg-password secret \
  --pg-dbname pgbase
```

### Environment Variables

```bash
PB_POSTGRES_HOST=localhost
PB_POSTGRES_PORT=5432
PB_POSTGRES_USER=pgbase
PB_POSTGRES_PASSWORD=secret
PB_POSTGRES_DBNAME=pgbase
PB_POSTGRES_SSLMODE=disable
```

## Configuration

See [CONFIG.md](CONFIG.md) for full configuration options.

## Migration from PocketBase

See [MIGRATION.md](MIGRATION.md) for migrating from SQLite.
```

### 9.2 Migration Guide

**File baru: `MIGRATION.md`**

Guide untuk user yang ingin migrate dari PocketBase SQLite ke PG-BASE:
- Export data dari PocketBase (via backup ZIP)
- Transform SQLite data ke PostgreSQL format (`pg_dump` + conversion)
- Import ke PG-BASE (`pg_restore`)
- Verify data integrity

### 9.3 Configuration Guide

**File baru: `CONFIG.md`**

Detailed configuration documentation:
- PostgreSQL connection settings
- Connection pool settings
- SSL configuration
- S3 storage configuration
- SMTP settings
- Backup settings

### 9.4 API Documentation

- [ ] Update semua contoh SQL di dokumentasi
- [ ] Update filter syntax documentation
- [ ] Update search functions documentation

---

## File Change Summary

### New Files (~15 files)

| File | Purpose |
|------|---------|
| `core/db_connect.go` | PostgreSQL connection |
| `tools/dbutils/dialect.go` | SQL dialect interface |
| `tools/dbutils/pgsql.go` | PostgreSQL dialect implementation |
| `tests/db.go` | Test DB helpers |
| `tests/docker-compose.test.yml` | Test DB container |
| `tests/data/schema.sql` | Test schema |
| `tests/data/seed.sql` | Test data |
| `Dockerfile` | Container build |
| `docker-compose.yml` | Full stack deployment |
| `MIGRATION.md` | Migration guide |
| `CONFIG.md` | Configuration guide |
| `PLAN.md` | This file |

### Modified Files (~60+ files)

| Category | Files | Effort |
|----------|-------|--------|
| Connection | `core/base.go`, `core/db_connect.go`, **`core/db_builder.go`** | High |
| Retry/Hooks | **`core/db.go`**, **`core/db_retry.go`** | Medium |
| Schema sync | **`core/collection_record_table_sync.go`** | **Very High** (NEW) |
| JSON/SQL | `tools/dbutils/*.go`, `core/record_field_resolver_runner.go`, **`core/record_query_expand.go`**, **`core/record_model.go`** | Very High |
| Table introspection | `core/db_table.go` | Medium |
| Migrations | `migrations/*.go` (8 files) | High |
| Field types | `core/field_*.go` (15+ files) | Medium |
| Search/filter | `tools/search/*.go` | Medium |
| APIs | `apis/*.go` (5+ files) | Low |
| Auth queries | `apis/record_auth_with_password.go`, `apis/record_auth_with_oauth2.go`, `core/record_query.go` | Medium |
| Collection | `core/collection_validate.go`, `core/collection_model.go` | Low |
| View | `core/view.go` | Low |
| Backup | **`core/base_backup.go`** | **High** (NEW — pg_dump) |
| Tests | `tests/*.go` (10+ files) | High |
| Config | `pocketbase.go`, `cmd/*.go` | Medium |
| CI/CD | **`.github/workflows/release.yaml`** | Medium (NEW) |
| Documentation | `README.md`, docs | Low |

### Deleted Files (~5 files)

| File | Reason |
|------|--------|
| `core/db_builder.go` | Replaced — no dualDBBuilder needed |
| `core/db_connect_nodefaultdriver.go` | Not needed |
| `modernc_versions_check.go` | SQLite-specific |
| `tests/data/data.db` | SQLite fixture |
| `tests/data/auxiliary.db` | SQLite fixture |

---

## Timeline

| Phase | Days | Description | Status |
|-------|------|-------------|--------|
| 0 | 1-2 | Setup, fork, rename, deps | Pending |
| 1 | **5-8** | Connection layer + db_builder + retry removal | Pending |
| 2 | 6-12 | SQL dialect abstraction | Pending |
| 2.5 | 13-14 | Index & quoting system | Pending |
| 2.6 | 15-18 | **NEW** Collection record table sync | Pending |
| 3 | 19-23 | Migration system | Pending |
| 4 | 24-28 | Field system | Pending |
| 5 | 29-38 | SQL-specific code + sqlite_master cleanup + COLLATE restructure + backup rewrite | Pending |
| 6 | 39-47 | Test infrastructure + CI/CD service | Pending |
| 7 | 48-53 | Config & deployment + pg_dump backup | Pending |
| 8 | 54-56 | UI updates | Pending |
| 9 | 57-60 | Documentation | Pending |
| **Total** | **~60 days** | **~12 weeks** | |

---

## Risk Mitigation

### High Risk Areas

1. **JSON functions** - Test exhaustively dengan berbagai data types: `[]`, `["a","b"]`, `"scalar"`, `null`, `""`, `123`, `{"key":"val"}`
2. **Relation joins** - Pastikan `jsonb_array_elements` (tanpa `_text`) bekerja untuk ID joins
3. **`collection_record_table_sync.go`** — Paling kompleks, 366 lines SQLite-specific
4. **Search/filter** - Test semua token functions dengan PostgreSQL
5. **Edge cases** - Empty arrays, NULL values, nested JSON, type coercion
6. **dbx builder compatibility** — Verifikasi `CreateTable`, `AddColumn`, `DropColumn`, `RenameColumn` dengan PgsqlBuilder
7. **PK detection** — `information_schema` tidak punya PK info langsung, perlu JOIN `pg_constraint`
8. **Test isolation** — Schema-per-test atau serial execution untuk menghindari cross-test contamination

### Testing Strategy

1. **Unit tests** untuk setiap dialect function (JSON, introspection, date)
2. **Integration tests** untuk complete request flow (API scenarios)
3. **Performance tests** untuk comparison SQLite vs PostgreSQL
4. **Migration tests** untuk ensure schema correctness
5. **Parallel-safe tests** — Implement schema-per-test isolation

### Rollback Plan

- [ ] Maintain SQLite branch untuk comparison
- [ ] Feature flags untuk dialect switching (jika perlu)
- [ ] Comprehensive logging untuk debugging

### Additional Mitigations

- [ ] **Phase order**: Swap Phase 6 (Test Infrastructure) ke Phase 1.5 — setup PG test DB first
- [ ] **Keep dates as TEXT initially**: Minimalisir breakage, switch ke TIMESTAMPTZ nanti
- [ ] **Credentials di env vars**: Jangan simpan PG password di DB settings (bootstrap problem)
- [ ] **pg_dump backup**: Install postgresql-client di Dockerfile runtime stage

---

## Success Criteria

1. ✅ All existing PocketBase tests pass dengan PostgreSQL
2. ✅ REST API endpoints berfungsi dengan PostgreSQL
3. ✅ Real-time subscriptions berfungsi
4. ✅ Authentication flows berfungsi
5. ✅ File upload/download berfungsi
6. ✅ Dashboard UI berfungsi
7. ✅ Docker deployment berfungsi
8. ✅ Binary deployment berfungsi
9. ✅ Performance comparable atau better than SQLite
10. ✅ Documentation lengkap

---

## Appendix A: File Storage Architecture

### Core Principle: File Names in DB, Blobs in Filesystem

Files are **NOT** stored as BLOBs in the database. The database stores only the **file name(s)** as strings. The actual file content lives in an external storage system (local filesystem or S3-compatible object storage).

### Database Column Types

| Config | Column Type | Value |
|--------|-------------|-------|
| `MaxSelect = 1` (single) | `TEXT` | `"photo_abc123.jpg"` |
| `MaxSelect > 1` (multiple) | `JSONB` | `["file1.pdf", "file2.pdf"]` |

### Filename Format

```
<snake_case_name>_<10_char_random_alphanumeric>.<extension>

Example:
my_photo_abc123def4.jpg
report_xyz789qwe.pdf
```

### Storage Path Structure

```
<collectionId>/<recordId>/<filename>

Example:
abc123collection/xyz789record/photo_abc123def4.jpg
```

### Thumbs Storage

```
<collectionId>/<recordId>/thumbs_<filename>/<thumbSize>_<filename>

Example:
abc123collection/xyz789record/thumbs_photo_abc123def4.jpg/100x100_photo_abc123def4.jpg
```

### Upload Flow

```
HTTP Request (multipart/form-data)
  │
  ▼
apis/record_crud.go → extractUploadedFiles()
  │  Parse multipart, create []*filesystem.File objects
  ▼
forms.RecordUpsert.Load(data)
  │  Set record values (mix of string filenames + *File objects)
  ▼
FileField.Intercept() (core/field_file.go)
  │
  ├─► processFilesToUpload()
  │     │  Upload each *File to filesystem
  │     │  Path: <collectionId>/<recordId>/<filename>
  │     ▼
  │     fsys.UploadFile(file, path)
  │
  ├─► Replace *File objects with string filenames
  │     record.Set("avatar", "photo_abc123def4.jpg")
  │
  ▼
DB Write (INSERT/UPDATE)
  │  Stores ONLY the filename string
  ▼
afterRecordExecuteSuccess()
     Delete old files if replaced
```

### Download Flow

```
GET /api/files/<collection>/<recordId>/<filename>
  │
  ▼
apis/file.go → find record → check access
  │
  ├─ Protected file? → Validate JWT token
  │
  ▼
Record.FindFileFieldByFile(filename)
  │  Find which FileField contains this filename
  ▼
fsys.Serve(response, request, path, filename)
  │  Read from filesystem/S3 → stream to response
  ▼
Optional: Generate thumb if ?thumb=WxH
```

### Storage Configuration

**Local storage** (default):
```
<pb_data>/storage/<collectionId>/<recordId>/<filename>
```

**S3 storage** (optional):
```json
{
  "s3": {
    "enabled": true,
    "bucket": "my-bucket",
    "region": "us-east-1",
    "endpoint": "",
    "accessKey": "",
    "secret": "",
    "forcePathStyle": false
  }
}
```

---

## Appendix B: SQLite vs PostgreSQL Reference

### JSON Functions

| Operation | SQLite | PostgreSQL |
|-----------|--------|------------|
| Unnest array | `json_each(col)` | `jsonb_array_elements(col::jsonb)` |
| Array length | `json_array_length(col)` | `jsonb_array_length(col::jsonb)` |
| Extract value | `json_extract(col, '$.key')` | `col::jsonb #>> '{key}'` |
| Check valid JSON | `json_valid(col)` | `jsonb_typeof(col) IS NOT NULL` |
| Get JSON type | `json_type(col)` | `jsonb_typeof(col)` |
| Build array | `json_array(a, b)` | `jsonb_build_array(a, b)` |
| Build object | `json_object('k', v)` | `jsonb_build_object('k', v)` |

### Date/Time Functions

| Operation | SQLite | PostgreSQL |
|-----------|--------|------------|
| Format datetime | `strftime('%Y-%m-%d', col)` | `to_char(col, 'YYYY-MM-DD')` |
| Current timestamp | `datetime('now')` | `NOW()` |
| Date truncation | `strftime('%Y-%m-01', col)` | `date_trunc('month', col)` |

### Table Introspection

| Operation | SQLite | PostgreSQL |
|-----------|--------|------------|
| List columns | `PRAGMA_TABLE_INFO(table)` | `information_schema.columns` |
| List indexes | `sqlite_master` | `pg_indexes` |
| Check table exists | `sqlite_schema` | `information_schema.tables` |

### ID Generation

| Operation | SQLite | PostgreSQL |
|-----------|--------|------------|
| Random ID | `'r' \|\| lower(hex(randomblob(7)))` | `'r' \|\| lower(hex(gen_random_bytes(7)))` |

### Case Insensitive

| Operation | SQLite | PostgreSQL |
|-----------|--------|------------|
| Case-insensitive compare | `COLLATE NOCASE` | `LOWER(a) = LOWER(b)` or `::citext` |

### Optimization

| Operation | SQLite | PostgreSQL |
|-----------|--------|------------|
| Cleanup | `VACUUM` | `VACUUM` (outside transaction) |
| Update stats | `PRAGMA optimize` | `ANALYZE` |
| WAL checkpoint | `PRAGMA wal_checkpoint(TRUNCATE)` | Automatic |

---

## Appendix C: Git Re-init Checklist

When starting implementation:

```bash
# 1. Re-init git
rm -rf .git
git init

# 2. Create initial commit
git add .
git commit -m "Initial commit: PG-BASE fork from PocketBase"

# 3. Create remote repository
# Create arief-fajri/pgbase on GitHub/GitLab

# 4. Add remote
git remote add origin git@github.com-personal:arief-fajri/pgbase.git

# 5. Push
git push -u origin main
```

---

*Document created: 2026-08-12*
*Last updated: 2026-08-12*

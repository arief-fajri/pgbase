package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/arief-fajri/pgbase/tools/dbutils"
	"github.com/arief-fajri/pgbase/tools/types"
	"github.com/spf13/cast"
)

// sqliteExportCollectionsDDL creates the v0.23-shaped "_collections" table in
// the exported SQLite file. SQLite is dynamically typed so the declared column
// types are cosmetic; the importer re-derives the real Postgres types from the
// rebuilt collection schema (see ImportFromSQLiteDir).
const sqliteExportCollectionsDDL = `CREATE TABLE _collections (
	id TEXT PRIMARY KEY NOT NULL,
	system INTEGER NOT NULL DEFAULT 0,
	type TEXT NOT NULL DEFAULT 'base',
	name TEXT NOT NULL,
	fields JSON NOT NULL DEFAULT '[]',
	listRule TEXT,
	viewRule TEXT,
	createRule TEXT,
	updateRule TEXT,
	deleteRule TEXT,
	options JSON NOT NULL DEFAULT '{}',
	created TEXT NOT NULL DEFAULT '',
	updated TEXT NOT NULL DEFAULT '',
	indexes JSON NOT NULL DEFAULT '[]'
)`

// sqliteExportParamsDDL creates the "_params" table used to carry the app
// settings across the backup (mirrors the layout ImportFromSQLiteDir reads).
const sqliteExportParamsDDL = `CREATE TABLE _params (
	id TEXT PRIMARY KEY NOT NULL,
	value JSON,
	created TEXT,
	updated TEXT
)`

// ExportToSQLiteFile dumps the current PostgreSQL-backed app (schema, records
// and settings) into a portable, v0.23-format SQLite "data.db" written at
// destPath.
//
// It is the exact inverse of [BaseApp.ImportFromSQLiteDir]: the produced file
// can be bundled inside a backup archive and later restored on any PGBase
// instance (SQLite or this Postgres fork). It intentionally does NOT dump the
// request/audit logs (they live in a separate auxiliary database) nor the
// "_migrations" table (the restore target owns its own migration history).
//
// The method is exported so it can also back a future CLI command and be
// exercised directly in tests.
func (app *BaseApp) ExportToSQLiteFile(ctx context.Context, destPath string) error {
	// start from a clean file so a re-run never appends to a stale dump
	_ = os.Remove(destPath)

	db, err := sql.Open("sqlite", destPath)
	if err != nil {
		return fmt.Errorf("failed to create the SQLite dump %q: %w", destPath, err)
	}
	defer db.Close()

	if _, err := db.Exec(sqliteExportCollectionsDDL); err != nil {
		return fmt.Errorf("failed to create the \"_collections\" table: %w", err)
	}

	collections, err := app.FindAllCollections()
	if err != nil {
		return fmt.Errorf("failed to load the collections: %w", err)
	}

	// 1) write every collection definition row
	for _, coll := range collections {
		exported, err := coll.DBExport(app)
		if err != nil {
			return fmt.Errorf("failed to export the %q collection definition: %w", coll.Name, err)
		}
		if err := writeSQLiteCollectionRow(db, exported); err != nil {
			return fmt.Errorf("failed to write the %q collection row: %w", coll.Name, err)
		}
	}

	// 2) recreate each non-view data table and stream its rows
	for _, coll := range collections {
		if coll.Type == CollectionTypeView {
			continue // views hold no data (their query is reapplied on import)
		}

		info, err := app.TableInfo(coll.Name)
		if err != nil {
			return fmt.Errorf("failed to inspect the %q table: %w", coll.Name, err)
		}

		if err := app.exportSQLiteRecords(ctx, db, coll.Name, info); err != nil {
			return fmt.Errorf("failed to export the %q records: %w", coll.Name, err)
		}
	}

	// 3) carry the settings across (best-effort within the file)
	if err := app.exportSQLiteSettings(ctx, db); err != nil {
		return fmt.Errorf("failed to export the settings: %w", err)
	}

	return nil
}

// writeSQLiteCollectionRow inserts a single collection definition (as produced
// by [Collection.DBExport]) into the exported "_collections" table, serializing
// the composite values (fields/indexes/options) back to JSON text.
func writeSQLiteCollectionRow(db *sql.DB, exported map[string]any) error {
	fieldsJSON, err := toJSONText(exported["fields"])
	if err != nil {
		return err
	}
	if fieldsJSON == "" {
		fieldsJSON = "[]"
	}

	indexesJSON, err := toJSONText(exported["indexes"])
	if err != nil {
		return err
	}
	if indexesJSON == "" {
		indexesJSON = "[]"
	}

	optionsJSON, err := toJSONText(exported["options"])
	if err != nil {
		return err
	}
	if optionsJSON == "" {
		optionsJSON = "{}"
	}

	_, err = db.Exec(
		`INSERT INTO _collections
			(id, system, type, name, fields, listRule, viewRule, createRule, updateRule, deleteRule, options, created, updated, indexes)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cast.ToString(exported["id"]),
		boolToSQLiteInt(exported["system"]),
		cast.ToString(exported["type"]),
		cast.ToString(exported["name"]),
		fieldsJSON,
		rulePtrToSQLite(exported["listRule"]),
		rulePtrToSQLite(exported["viewRule"]),
		rulePtrToSQLite(exported["createRule"]),
		rulePtrToSQLite(exported["updateRule"]),
		rulePtrToSQLite(exported["deleteRule"]),
		optionsJSON,
		dateTimeToSQLiteText(exported["created"]),
		dateTimeToSQLiteText(exported["updated"]),
		indexesJSON,
	)

	return err
}

// exportSQLiteRecords creates the SQLite table for the given collection and
// copies every Postgres row into it, coercing each value to a SQLite-storable
// text form the importer can round-trip (see coercePGValueForSQLite).
func (app *BaseApp) exportSQLiteRecords(ctx context.Context, db *sql.DB, table string, info []*TableInfoRow) error {
	var createSQL, selectExpr strings.Builder
	createSQL.WriteString(`CREATE TABLE "` + table + `" (`)

	for i, r := range info {
		if i > 0 {
			createSQL.WriteString(", ")
			selectExpr.WriteString(", ")
		}

		colDecl := `"` + r.Name + `" ` + sqliteColumnType(r.Type)
		if r.Name == "id" {
			colDecl += " PRIMARY KEY NOT NULL"
		}
		createSQL.WriteString(colDecl)

		selectExpr.WriteString(pgColumnAsSQLiteText(r.Name, r.Type))
	}
	createSQL.WriteString(")")

	if _, err := db.Exec(createSQL.String()); err != nil {
		return fmt.Errorf("failed to create the SQLite table: %w", err)
	}

	query := `SELECT ` + selectExpr.String() + ` FROM ` + dbutils.DefaultDialect.QuoteIdentifier(table)
	rows, err := app.DB().NewQuery(query).WithContext(ctx).Rows()
	if err != nil {
		return fmt.Errorf("failed to read the Postgres rows: %w", err)
	}
	defer rows.Close()

	colNames := make([]string, len(info))
	placeholders := make([]string, len(info))
	for i, r := range info {
		colNames[i] = `"` + r.Name + `"`
		placeholders[i] = "?"
	}
	insertSQL := `INSERT INTO "` + table + `" (` + strings.Join(colNames, ", ") + `) VALUES (` + strings.Join(placeholders, ", ") + `)`

	stmt, err := db.Prepare(insertSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	// every column is projected to text (or NULL) in the SELECT above
	holders := make([]sql.NullString, len(info))
	scanPtrs := make([]any, len(info))
	for i := range holders {
		scanPtrs[i] = &holders[i]
	}
	args := make([]any, len(info))

	for rows.Next() {
		if err := rows.Scan(scanPtrs...); err != nil {
			return err
		}
		for i := range holders {
			if holders[i].Valid {
				args[i] = holders[i].String
			} else {
				args[i] = nil
			}
		}
		if _, err := stmt.Exec(args...); err != nil {
			return fmt.Errorf("failed to insert a row: %w", err)
		}
	}

	return rows.Err()
}

// exportSQLiteSettings copies the current "_params" settings row into the dump
// verbatim (encryption state preserved); the importer decrypts as needed.
func (app *BaseApp) exportSQLiteSettings(ctx context.Context, db *sql.DB) error {
	if _, err := db.Exec(sqliteExportParamsDDL); err != nil {
		return err
	}

	param := &Param{}
	err := app.ModelQuery(param).Model(paramsKeySettings, param)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	_, err = db.Exec(
		`INSERT INTO _params (id, value, created, updated) VALUES (?, ?, ?, ?)`,
		paramsKeySettings,
		string(param.Value),
		dateTimeToSQLiteText(param.Created),
		dateTimeToSQLiteText(param.Updated),
	)

	return err
}

// -------------------------------------------------------------------
// value / type coercion helpers (Postgres -> SQLite)
// -------------------------------------------------------------------

// pgColumnAsSQLiteText builds the SELECT expression that projects a Postgres
// column to the text form the importer expects. Timestamps are formatted in the
// app's default date layout ("YYYY-MM-DD HH:MI:SS.mmmZ") so they parse verbatim
// via types.ParseDateTime; everything else is cast to text (jsonb -> JSON text,
// booleans -> "true"/"false", numbers -> their decimal string). NULL stays NULL.
func pgColumnAsSQLiteText(name, pgType string) string {
	col := `"` + name + `"`
	if isPGDateType(pgType) {
		return `(to_char(` + col + ` AT TIME ZONE 'UTC', 'YYYY-MM-DD HH24:MI:SS.MS') || 'Z')`
	}
	return col + `::text`
}

// coercePGValueForSQLite converts a raw Postgres value (already projected to
// its text/NULL form) into the scalar written to SQLite. It is the inverse of
// coerceSQLiteValueForPG and is kept as a standalone, unit-testable function.
//
// The second return value reports whether the value is NULL (and should be
// stored as SQLite NULL).
func coercePGValueForSQLite(pgType string, val any) (any, bool) {
	if val == nil {
		return nil, true
	}

	switch v := val.(type) {
	case sql.NullString:
		if !v.Valid {
			return nil, true
		}
		return v.String, false
	case []byte:
		return string(v), false
	case types.DateTime:
		if v.IsZero() {
			return nil, true
		}
		return v.String(), false
	case bool:
		if v {
			return "true", false
		}
		return "false", false
	default:
		return cast.ToString(val), false
	}
}

// sqliteColumnType maps a Postgres column type to a cosmetic SQLite type
// (SQLite is dynamically typed; the importer re-derives the real types).
func sqliteColumnType(pgType string) string {
	t := strings.ToLower(pgType)
	switch {
	case strings.Contains(t, "json"):
		return "JSON"
	case strings.Contains(t, "bool"):
		return "BOOLEAN"
	case isPGDateType(pgType):
		return "TEXT"
	case strings.Contains(t, "numeric"),
		strings.Contains(t, "int"),
		strings.Contains(t, "real"),
		strings.Contains(t, "double"),
		strings.Contains(t, "decimal"):
		return "NUMERIC"
	default:
		return "TEXT"
	}
}

func isPGDateType(pgType string) bool {
	t := strings.ToLower(pgType)
	return strings.Contains(t, "timestamp") || strings.Contains(t, "date")
}

// toJSONText normalizes a composite value into its JSON text representation.
func toJSONText(v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", nil
	case string:
		return x, nil
	case []byte:
		return string(x), nil
	case types.JSONRaw:
		return string(x), nil
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
}

func boolToSQLiteInt(v any) int {
	if cast.ToBool(v) {
		return 1
	}
	return 0
}

// rulePtrToSQLite maps a collection rule (*string) to a SQLite value preserving
// the nil (locked / superusers-only) vs "" (public) distinction as NULL vs ”.
func rulePtrToSQLite(v any) any {
	switch r := v.(type) {
	case nil:
		return nil
	case *string:
		if r == nil {
			return nil
		}
		return *r
	case string:
		return r
	default:
		return cast.ToString(v)
	}
}

func dateTimeToSQLiteText(v any) any {
	switch d := v.(type) {
	case nil:
		return ""
	case types.DateTime:
		if d.IsZero() {
			return ""
		}
		return d.String()
	case string:
		return d
	default:
		return cast.ToString(v)
	}
}

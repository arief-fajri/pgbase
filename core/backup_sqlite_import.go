package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/arief-fajri/pgbase/tools/dbutils"
	"github.com/arief-fajri/pgbase/tools/filesystem"
	"github.com/arief-fajri/pgbase/tools/list"
	"github.com/arief-fajri/pgbase/tools/security"
	"github.com/arief-fajri/pgbase/tools/types"
	"github.com/pocketbase/dbx"
	"github.com/spf13/cast"

	// pure-Go SQLite driver (no cgo), used ONLY to read a legacy SQLite-based
	// "data.db" backup during restore. Registers the "sqlite" driver name.
	_ "modernc.org/sqlite"
)

// sqliteBackupDBName is the SQLite database file bundled inside legacy
// SQLite-based backups.
const sqliteBackupDBName = "data.db"

// Detected source schema generations of a bundled SQLite "data.db".
const (
	// sqliteBackupVersionV23 is the current SQLite schema (v0.23+):
	// a "_superusers" table and a "fields" column on "_collections".
	sqliteBackupVersionV23 = 23

	// sqliteBackupVersionV22 is any pre-v0.23 SQLite schema: an "_admins"
	// table and/or a legacy "schema" column on "_collections". Base-collection
	// data imports faithfully; auth/view collection options are best-effort
	// (recreated with the v0.23 defaults). See convertLegacyField.
	sqliteBackupVersionV22 = 22
)

// sqliteIdentRegex guards raw table names before they are interpolated into
// SQLite PRAGMA/SELECT statements (collection names are always [A-Za-z0-9_]).
var sqliteIdentRegex = regexp.MustCompile(`^\w+$`)

// sqliteCollMeta holds the minimal per-collection metadata needed to drive the
// record-copy phase after the schema has been imported.
type sqliteCollMeta struct {
	name   string
	isView bool
}

// ImportFromSQLiteDir imports the data of a legacy SQLite-based
// backup (extracted at extractedDir, containing a "data.db") into the current
// PostgreSQL-backed app.
//
// The steps are:
//  1. open data.db read-only and verify it is a supported (v0.23+) format;
//  2. rebuild the collection schema via [BaseApp.ImportCollections] (this also
//     creates the physical Postgres tables);
//  3. copy every non-view record verbatim, per column, with SQLite->Postgres
//     type coercion (preserving ids, password hashes, tokenKeys, timestamps);
//  4. best-effort import of the stored settings (_params);
//  5. copy the "storage/" files into the app filesystem (local or S3).
//
// It intentionally does NOT copy request/audit logs (they live in a separate
// auxiliary database and carry little value across an engine migration).
//
// The caller is responsible for restarting the app afterwards so the collection
// cache and settings are reloaded from the freshly imported data.
func (app *BaseApp) ImportFromSQLiteDir(ctx context.Context, extractedDir string) error {
	dbPath := filepath.Join(extractedDir, sqliteBackupDBName)

	db, err := openSQLiteReadonly(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open the SQLite backup %q: %w", sqliteBackupDBName, err)
	}
	defer db.Close()

	tables, err := sqliteTableNames(db)
	if err != nil {
		return fmt.Errorf("failed to inspect the SQLite backup: %w", err)
	}

	version, err := detectSQLiteBackupVersion(db, tables)
	if err != nil {
		return err
	}

	app.Logger().Info(
		"[SQLite import] Detected a compatible legacy SQLite backup, starting import",
		slog.Int("schemaVersion", version),
	)

	defs, meta, err := readSQLiteCollections(db, version)
	if err != nil {
		return fmt.Errorf("failed to read the collections from the backup: %w", err)
	}

	if err := app.importSQLiteSchema(defs, meta); err != nil {
		return fmt.Errorf("failed to import the collections schema: %w", err)
	}

	if err := app.importSQLiteRecords(ctx, db, meta); err != nil {
		return fmt.Errorf("failed to import the records: %w", err)
	}

	// pre-v0.23 backups store superusers in a separate "_admins" table rather
	// than a collection; migrate them best-effort so a failure here does not
	// abort an otherwise successful data restore.
	if version == sqliteBackupVersionV22 && slices.Contains(tables, "_admins") {
		if err := app.importLegacyAdmins(ctx, db); err != nil {
			app.Logger().Warn(
				"[SQLite import] Skipping legacy admins import (keeping the current superusers)",
				slog.String("error", err.Error()),
			)
		}
	}

	// settings are best-effort: an undecryptable or invalid blob must not
	// abort an otherwise successful data restore.
	if err := app.importSQLiteSettings(ctx, db); err != nil {
		app.Logger().Warn(
			"[SQLite import] Skipping settings import (keeping the current instance settings)",
			slog.String("error", err.Error()),
		)
	}

	if err := app.importBackupStorage(ctx, extractedDir); err != nil {
		return fmt.Errorf("failed to import the storage files: %w", err)
	}

	app.Logger().Info("[SQLite import] Import completed successfully")

	return nil
}

// -------------------------------------------------------------------
// SQLite access helpers
// -------------------------------------------------------------------

func openSQLiteReadonly(path string) (*sql.DB, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}

	// prefer an explicit read-only handle
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, err
	}
	if pingErr := db.Ping(); pingErr != nil {
		// fallback to a plain handle (e.g. if the URI form is unsupported)
		_ = db.Close()

		db, err = sql.Open("sqlite", path)
		if err != nil {
			return nil, err
		}
		if err := db.Ping(); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	return db, nil
}

func sqliteTableNames(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}

	return names, rows.Err()
}

func sqliteTableColumns(db *sql.DB, table string) ([]string, error) {
	if !sqliteIdentRegex.MatchString(table) {
		return nil, fmt.Errorf("invalid table name %q", table)
	}

	rows, err := db.Query(`SELECT name FROM pragma_table_info('` + table + `')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}

	return cols, rows.Err()
}

// querySQLiteRows reads every row of the provided table returning the column
// names and each row as a slice of native driver values (nil, int64, float64,
// string or []byte).
func querySQLiteRows(db *sql.DB, table string) ([]string, [][]any, error) {
	if !sqliteIdentRegex.MatchString(table) {
		return nil, nil, fmt.Errorf("invalid table name %q", table)
	}

	rows, err := db.Query(`SELECT * FROM "` + table + `"`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	var result [][]any
	for rows.Next() {
		holders := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range holders {
			ptrs[i] = &holders[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, nil, err
		}
		result = append(result, holders)
	}

	return cols, result, rows.Err()
}

// -------------------------------------------------------------------
// Version detection
// -------------------------------------------------------------------

// detectSQLiteBackupVersion inspects the backup markers and returns the source
// schema generation it was created with:
//
//   - sqliteBackupVersionV23 — a "_superusers" table and a "fields" column on
//     "_collections" (the current schema, imported verbatim).
//   - sqliteBackupVersionV22 — an "_admins" table and/or a legacy "schema"
//     column on "_collections" (pre-v0.23; converted on the fly, see
//     readSQLiteCollections/convertLegacyField).
//
// Any other shape is reported as an unrecognized backup.
func detectSQLiteBackupVersion(db *sql.DB, tables []string) (int, error) {
	has := func(name string) bool {
		for _, t := range tables {
			if t == name {
				return true
			}
		}
		return false
	}

	collectionsCols, err := sqliteTableColumns(db, "_collections")
	if err != nil {
		return 0, fmt.Errorf("invalid backup: unable to read the \"_collections\" table (%w)", err)
	}

	var hasFields, hasSchema bool
	for _, c := range collectionsCols {
		switch c {
		case "fields":
			hasFields = true
		case "schema":
			hasSchema = true
		}
	}

	// v0.23+ markers: a "_superusers" table and a "fields" column on "_collections".
	if has(CollectionNameSuperusers) && hasFields {
		return sqliteBackupVersionV23, nil
	}

	// pre-v0.23 markers: an "_admins" table or a legacy "schema" column.
	if has("_admins") || hasSchema {
		return sqliteBackupVersionV22, nil
	}

	return 0, fmt.Errorf(
		"unrecognized backup format; expected a legacy SQLite database (tables found: %s)",
		strings.Join(tables, ", "),
	)
}

// -------------------------------------------------------------------
// Schema import
// -------------------------------------------------------------------

// readSQLiteCollections reads the "_collections" rows and converts them into
// the flat map form expected by [BaseApp.ImportCollections].
//
// The type-specific "options" column is flattened onto the top-level map since
// that is how the fork's Collection JSON (un)marshaling expects auth/view
// options (see Collection.MarshalJSON / baseCollection.RawOptions json:"-").
//
// For a pre-v0.23 backup (version == sqliteBackupVersionV22) the legacy
// "schema" column is read instead of "fields", each field is converted via
// convertLegacyField, and the implicit created/updated autodate fields are
// prepended (they lived outside the schema JSON before v0.23).
func readSQLiteCollections(db *sql.DB, version int) ([]map[string]any, []sqliteCollMeta, error) {
	cols, rows, err := querySQLiteRows(db, "_collections")
	if err != nil {
		return nil, nil, err
	}

	idx := make(map[string]int, len(cols))
	for i, c := range cols {
		idx[c] = i
	}

	get := func(row []any, col string) any {
		if i, ok := idx[col]; ok {
			return row[i]
		}
		return nil
	}

	defs := make([]map[string]any, 0, len(rows))
	meta := make([]sqliteCollMeta, 0, len(rows))

	for _, row := range rows {
		name := cast.ToString(get(row, "name"))
		typ := cast.ToString(get(row, "type"))

		def := map[string]any{
			"id":     cast.ToString(get(row, "id")),
			"name":   name,
			"type":   typ,
			"system": cast.ToBool(get(row, "system")),
		}

		// preserve the nil (locked / superusers-only) vs "" (public) rule distinction
		for _, rk := range []string{"listRule", "viewRule", "createRule", "updateRule", "deleteRule"} {
			if v := get(row, rk); v != nil {
				def[rk] = cast.ToString(v)
			} else {
				def[rk] = nil
			}
		}

		if created := cast.ToString(get(row, "created")); created != "" {
			def["created"] = created
		}
		if updated := cast.ToString(get(row, "updated")); updated != "" {
			def["updated"] = updated
		}

		if version == sqliteBackupVersionV22 {
			// pre-v0.23 stored the user fields in a "schema" column with nested
			// per-type options; convert them and (re)materialize the implicit
			// created/updated autodate fields that lived outside that JSON.
			var legacy []map[string]any
			if raw := anyToJSONBytes(get(row, "schema")); len(raw) > 0 {
				if err := json.Unmarshal(raw, &legacy); err != nil {
					return nil, nil, fmt.Errorf("collection %q: invalid legacy \"schema\" json: %w", name, err)
				}
			}

			fields := make([]map[string]any, 0, len(legacy)+2)
			fields = append(fields,
				legacyAutodateField("created", false),
				legacyAutodateField("updated", true),
			)
			for _, f := range legacy {
				fields = append(fields, convertLegacyField(f))
			}
			def["fields"] = fields
		} else if raw := anyToJSONBytes(get(row, "fields")); len(raw) > 0 {
			var parsed any
			if err := json.Unmarshal(raw, &parsed); err != nil {
				return nil, nil, fmt.Errorf("collection %q: invalid \"fields\" json: %w", name, err)
			}
			def["fields"] = parsed
		}

		if raw := anyToJSONBytes(get(row, "indexes")); len(raw) > 0 {
			var idxList []string
			if err := json.Unmarshal(raw, &idxList); err != nil {
				return nil, nil, fmt.Errorf("collection %q: invalid \"indexes\" json: %w", name, err)
			}
			for i := range idxList {
				idxList[i] = normalizeSQLiteIndex(idxList[i])
			}
			def["indexes"] = idxList
		}

		if raw := anyToJSONBytes(get(row, "options")); len(raw) > 0 {
			var opts map[string]any
			if err := json.Unmarshal(raw, &opts); err != nil {
				return nil, nil, fmt.Errorf("collection %q: invalid \"options\" json: %w", name, err)
			}
			if version == sqliteBackupVersionV22 {
				// pre-v0.23 collection-level options have a different shape; only
				// the view query maps cleanly to v0.23. Auth options are dropped
				// in favor of the v0.23 defaults (best-effort).
				if typ == "view" {
					if q, ok := opts["query"]; ok {
						def["viewQuery"] = q
					}
				}
			} else {
				for k, v := range opts {
					if _, exists := def[k]; !exists {
						def[k] = v
					}
				}
			}
		}

		// Auth collections in SQLite relied on the implicit COLLATE NOCASE of
		// their identity fields; PostgreSQL carries the case-insensitivity in
		// the functional LOWER(...) unique index instead. Convert any plain
		// unique identity index to that form before ImportCollections validates
		// it (see IDX-1 / D-1).
		if typ == "auth" {
			if err := normalizeSQLiteIdentityIndexes(def); err != nil {
				return nil, nil, err
			}
		}

		defs = append(defs, def)
		meta = append(meta, sqliteCollMeta{name: name, isView: typ == "view"})
	}

	return defs, meta, nil
}

// convertLegacyField converts a single pre-v0.23 field definition (which nests
// its per-type settings under an "options" object) into the flat v0.23 field
// shape expected by [BaseApp.ImportCollections].
//
// The field TYPE strings are identical across versions, so only the nested
// options are hoisted to the top level, applying the handful of key renames
// that changed in v0.23:
//
//   - number:   noDecimal   -> onlyInt
//   - editor:   convertUrls -> convertURLs
//   - relation: displayFields dropped (presentation-only hint, removed in v0.23)
//
// A text field's unset min/max (stored as null pre-v0.23) is normalized to 0.
// The legacy top-level "unique" flag is intentionally not carried over: v0.23
// expresses uniqueness through the collection "indexes" (already read
// separately), not a field property.
func convertLegacyField(f map[string]any) map[string]any {
	out := map[string]any{}

	for _, k := range []string{"id", "name", "type", "system", "required", "presentable"} {
		if v, ok := f[k]; ok {
			out[k] = v
		}
	}

	typ := cast.ToString(f["type"])

	opts, _ := f["options"].(map[string]any)
	for k, v := range opts {
		switch {
		case typ == "number" && k == "noDecimal":
			out["onlyInt"] = v
		case typ == "editor" && k == "convertUrls":
			out["convertURLs"] = v
		case typ == "relation" && k == "displayFields":
			// dropped in v0.23
		default:
			out[k] = v
		}
	}

	if typ == "text" {
		for _, k := range []string{"min", "max"} {
			if v, ok := out[k]; !ok || v == nil {
				out[k] = 0
			}
		}
	}

	return out
}

// legacyAutodateField builds a v0.23 "autodate" system field for the implicit
// created/updated columns of a pre-v0.23 collection (which stored them outside
// the schema JSON). The timestamps themselves are copied verbatim during the
// record phase; the field only needs to exist so the column is created. The id
// and flags mirror the v0.23 defaults produced by a real upstream migration.
func legacyAutodateField(name string, onUpdate bool) map[string]any {
	return map[string]any{
		"id":          "_pbf_autodate_" + name + "_",
		"name":        name,
		"type":        "autodate",
		"system":      false,
		"presentable": false,
		"hidden":      false,
		"onCreate":    true,
		"onUpdate":    onUpdate,
	}
}

// importSQLiteSchema imports the collection definitions, recreating all physical
// Postgres tables. If the full import fails and the backup contains views, it
// retries without them since legacy SQLite view queries may not be valid
// PostgreSQL (in which case the affected views are logged and skipped).
func (app *BaseApp) importSQLiteSchema(defs []map[string]any, meta []sqliteCollMeta) error {
	err := app.ImportCollections(defs, true)
	if err == nil {
		return nil
	}

	nonView := make([]map[string]any, 0, len(defs))
	var skipped []string
	for i, d := range defs {
		if i < len(meta) && meta[i].isView {
			skipped = append(skipped, meta[i].name)
			continue
		}
		nonView = append(nonView, d)
	}

	// nothing view-related to blame -> surface the original error
	if len(skipped) == 0 {
		return err
	}

	app.Logger().Warn(
		"[SQLite import] Failed to import the schema with views; retrying without them "+
			"(legacy SQLite view definitions may be incompatible with PostgreSQL and must be recreated manually)",
		slog.Any("skippedViews", skipped),
		slog.String("error", err.Error()),
	)

	return app.ImportCollections(nonView, true)
}

// -------------------------------------------------------------------
// Record data import
// -------------------------------------------------------------------

// importSQLiteRecords copies every non-view collection's rows from the SQLite
// backup into the corresponding freshly-created Postgres table.
//
// The copy is raw and per-column (not through the record model) so that ids,
// password hashes, tokenKeys and timestamps are preserved verbatim and no model
// hooks are triggered. The whole operation runs in a single transaction.
func (app *BaseApp) importSQLiteRecords(ctx context.Context, db *sql.DB, meta []sqliteCollMeta) error {
	return app.RunInTransaction(func(txApp App) error {
		for _, m := range meta {
			if m.isView {
				continue
			}

			if !app.HasTable(m.name) {
				app.Logger().Warn(
					"[SQLite import] Skipping records for a collection without a target table",
					slog.String("collection", m.name),
				)
				continue
			}

			info, err := app.TableInfo(m.name)
			if err != nil {
				return fmt.Errorf("failed to inspect target table %q: %w", m.name, err)
			}
			targetCols := make(map[string]string, len(info))
			for _, r := range info {
				targetCols[r.Name] = r.Type
			}

			cols, rows, err := querySQLiteRows(db, m.name)
			if err != nil {
				return fmt.Errorf("failed to read %q rows from the backup: %w", m.name, err)
			}

			// clear any existing rows for a faithful (replace) restore
			quoted := dbutils.DefaultDialect.QuoteIdentifier(m.name)
			if _, err := txApp.DB().NewQuery("DELETE FROM " + quoted).WithContext(ctx).Execute(); err != nil {
				return fmt.Errorf("failed to clear target table %q: %w", m.name, err)
			}

			// PGB-M03: the raw INSERT path bypasses EditorField.ValidateValue, so
			// sanitize editor-field HTML (allow-list) before it lands in the DB.
			editorCols := map[string]bool{}
			if coll, colErr := app.FindCollectionByNameOrId(m.name); colErr == nil {
				for _, fld := range coll.Fields {
					if fld.Type() == FieldTypeEditor {
						editorCols[fld.GetName()] = true
					}
				}
			}

			for _, row := range rows {
				params := dbx.Params{}
				for i, c := range cols {
					pgType, ok := targetCols[c]
					if !ok {
						continue // column not present in the target table
					}
					out, omit := coerceSQLiteValueForPG(pgType, row[i])
					if omit {
						continue // let the Postgres column default apply
					}
					if editorCols[c] {
						if s, isStr := out.(string); isStr {
							params[c] = sanitizeEditorHTML(s)
							continue
						}
					}
					params[c] = out
				}

				if len(params) == 0 {
					continue
				}

				if _, err := txApp.DB().Insert(m.name, params).WithContext(ctx).Execute(); err != nil {
					return fmt.Errorf("failed to insert a row into %q: %w", m.name, err)
				}
			}

			app.Logger().Info(
				"[SQLite import] Imported records",
				slog.String("collection", m.name),
				slog.Int("count", len(rows)),
			)
		}

		return nil
	})
}

// importLegacyAdmins copies the rows of a pre-v0.23 "_admins" table into the
// target's existing "_superusers" auth collection table (its v0.23 successor).
//
// It runs in a single transaction that first clears the bootstrapped
// superuser(s) so the restore is faithful; if any step fails the transaction
// rolls back (leaving the current superusers intact) and the error is returned
// to the caller, which treats the whole step as best-effort. When the source
// "_admins" table is empty the bootstrapped superuser is left untouched to
// avoid locking the instance out.
func (app *BaseApp) importLegacyAdmins(ctx context.Context, db *sql.DB) error {
	cols, rows, err := querySQLiteRows(db, "_admins")
	if err != nil {
		return fmt.Errorf("failed to read the legacy \"_admins\" table: %w", err)
	}
	if len(rows) == 0 {
		return nil
	}

	if !app.HasTable(CollectionNameSuperusers) {
		return fmt.Errorf("missing target %q table", CollectionNameSuperusers)
	}

	idx := make(map[string]int, len(cols))
	for i, c := range cols {
		idx[c] = i
	}
	get := func(row []any, col string) any {
		if i, ok := idx[col]; ok {
			return row[i]
		}
		return nil
	}

	return app.RunInTransaction(func(txApp App) error {
		quoted := dbutils.DefaultDialect.QuoteIdentifier(CollectionNameSuperusers)
		if _, err := txApp.DB().NewQuery("DELETE FROM " + quoted).WithContext(ctx).Execute(); err != nil {
			return fmt.Errorf("failed to clear %q: %w", CollectionNameSuperusers, err)
		}

		for _, row := range rows {
			// password hash and tokenKey are copied verbatim so existing
			// superuser credentials keep working after the restore
			params := dbx.Params{
				"id":              cast.ToString(get(row, "id")),
				"email":           cast.ToString(get(row, "email")),
				"password":        cast.ToString(get(row, "passwordHash")),
				"tokenKey":        cast.ToString(get(row, "tokenKey")),
				"emailVisibility": false,
				"verified":        true,
			}
			if dt, err := types.ParseDateTime(cast.ToString(get(row, "created"))); err == nil && !dt.IsZero() {
				params["created"] = dt
			}
			if dt, err := types.ParseDateTime(cast.ToString(get(row, "updated"))); err == nil && !dt.IsZero() {
				params["updated"] = dt
			}

			if _, err := txApp.DB().Insert(CollectionNameSuperusers, params).WithContext(ctx).Execute(); err != nil {
				return fmt.Errorf("failed to insert a legacy admin: %w", err)
			}
		}

		app.Logger().Info(
			"[SQLite import] Migrated legacy admins into superusers",
			slog.Int("count", len(rows)),
		)

		return nil
	})
}

// coerceSQLiteValueForPG converts a raw SQLite value (as read by
// querySQLiteRows) into the value to bind for the corresponding PostgreSQL
// column, driven by the target column type. It is the inverse of
// coercePGValueForSQLite used by the exporter.
//
// The second return value reports whether the column should be omitted from the
// INSERT entirely (so the Postgres DEFAULT / NULL applies) - used for empty or
// NULL source values.
func coerceSQLiteValueForPG(pgType string, val any) (any, bool) {
	t := strings.ToLower(pgType)

	switch {
	case strings.Contains(t, "json"):
		raw := anyToJSONBytes(val)
		if len(raw) == 0 {
			return nil, true // e.g. multi-relation/select/file default to '[]'
		}
		return types.JSONRaw(raw), false

	case strings.Contains(t, "bool"):
		if val == nil {
			return nil, true
		}
		return cast.ToBool(val), false

	case strings.Contains(t, "timestamp"), strings.Contains(t, "date"), strings.Contains(t, "time"):
		dt, err := types.ParseDateTime(cast.ToString(val))
		if err != nil || dt.IsZero() {
			return nil, true // NULL timestamptz
		}
		return dt, false

	case strings.Contains(t, "numeric"),
		strings.Contains(t, "int"),
		strings.Contains(t, "real"),
		strings.Contains(t, "double"),
		strings.Contains(t, "decimal"):
		if val == nil {
			return nil, true
		}
		if cast.ToString(val) == "" {
			return nil, true // default 0
		}
		return val, false

	default: // text and everything else -> verbatim string
		if val == nil {
			return nil, true // default '' for NOT NULL text columns
		}
		return cast.ToString(val), false
	}
}

// anyToJSONBytes normalizes a raw SQLite value into its JSON byte
// representation (SQLite stores json/jsonb-mapped columns as text).
func anyToJSONBytes(v any) []byte {
	switch x := v.(type) {
	case nil:
		return nil
	case []byte:
		return x
	case string:
		return []byte(x)
	default:
		b, _ := json.Marshal(x)
		return b
	}
}

// normalizeSQLiteIndex rewrites a legacy SQLite "CREATE INDEX" expression into a
// PostgreSQL-compatible one.
//
// SQLite quotes identifiers with backticks (e.g. ON `users` (`email`)). While
// [dbutils.ParseIndex]/Build already strip those from the table/column parts,
// the trailing WHERE clause is preserved verbatim, so a partial-index
// expression like WHERE `email` != ” would keep its backticks and fail to
// parse in PostgreSQL. Backticks are only ever identifier quotes here, so
// converting them to double quotes yields valid PostgreSQL (string literals use
// single quotes and are unaffected).
func normalizeSQLiteIndex(expr string) string {
	return strings.ReplaceAll(expr, "`", `"`)
}

// normalizeSQLiteIdentityIndexes rewrites the plain (case-sensitive) unique
// indexes of the auth identity fields to the functional LOWER(...) form.
//
// SQLite stores auth identity fields with an implicit COLLATE NOCASE, but the
// translated index definitions do not carry a collation. Without this step the
// import would either fail the collection validator (which now requires
// functional identity indexes - see NF-2) or silently import a case-sensitive
// index that forces a sequential scan on every identity login (IDX-1).
func normalizeSQLiteIdentityIndexes(def map[string]any) error {
	rawIdx, ok := def["indexes"].([]string)
	if !ok {
		return nil // no indexes
	}

	var identityFields []string
	if rawPw, ok := cast.ToStringMap(def["passwordAuth"])["identityFields"].([]any); ok {
		for _, v := range rawPw {
			if s := cast.ToString(v); s != "" {
				identityFields = append(identityFields, s)
			}
		}
	}

	if len(identityFields) == 0 {
		return nil
	}

	for i, expr := range rawIdx {
		parsed := dbutils.ParseIndex(expr)
		if !parsed.Unique || len(parsed.Columns) != 1 {
			continue
		}

		colName := dbutils.NormalizeIndexColumnName(parsed.Columns[0].Name)
		if !list.ExistInSlice(colName, identityFields) {
			continue
		}

		// already functional -> nothing to do
		if dbutils.IsFunctionalIndexColumn(parsed.Columns[0].Name) {
			continue
		}

		rawIdx[i] = fmt.Sprintf(
			`CREATE UNIQUE INDEX "%s" ON "%s" (LOWER("%s")) WHERE "%s" <> ''`,
			parsed.IndexName,
			cast.ToString(def["name"]),
			colName,
			colName,
		)
	}

	return nil
}

// -------------------------------------------------------------------
// Settings import
// -------------------------------------------------------------------

// importSQLiteSettings copies the stored application settings from the backup's
// "_params" table into the current instance.
//
// If the source blob is encrypted it is decrypted using this instance's
// encryption key (when configured) and always stored back as plaintext so it
// remains loadable on the next boot regardless of the target key. An
// undecryptable or invalid blob results in an error (handled as best-effort by
// the caller).
func (app *BaseApp) importSQLiteSettings(ctx context.Context, db *sql.DB) error {
	var raw sql.NullString
	err := db.QueryRow(`SELECT value FROM "_params" WHERE id = 'settings' LIMIT 1`).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if !raw.Valid || raw.String == "" {
		return nil
	}

	value := raw.String

	// verify the value is loadable plaintext JSON; otherwise try to decrypt it
	var probe map[string]any
	if json.Unmarshal([]byte(value), &probe) != nil {
		key := os.Getenv(app.EncryptionEnv())
		if key == "" {
			return fmt.Errorf(
				"the backup settings appear to be encrypted but no %q key is configured on this instance",
				app.EncryptionEnv(),
			)
		}

		decrypted, decErr := security.Decrypt(value, key)
		if decErr != nil {
			return fmt.Errorf("failed to decrypt the backup settings: %w", decErr)
		}
		if json.Unmarshal(decrypted, &probe) != nil {
			return fmt.Errorf("the decrypted backup settings are not valid JSON")
		}

		value = string(decrypted) // store as plaintext so it is always loadable
	}

	now := types.NowDateTime()

	return app.RunInTransaction(func(txApp App) error {
		res, err := txApp.DB().Update("_params", dbx.Params{
			"value":   value,
			"updated": now,
		}, dbx.HashExp{"id": paramsKeySettings}).WithContext(ctx).Execute()
		if err != nil {
			return err
		}

		if n, _ := res.RowsAffected(); n == 0 {
			_, err = txApp.DB().Insert("_params", dbx.Params{
				"id":      paramsKeySettings,
				"value":   value,
				"created": now,
				"updated": now,
			}).WithContext(ctx).Execute()
		}

		return err
	})
}

// -------------------------------------------------------------------
// Storage files import
// -------------------------------------------------------------------

// importSQLiteStorage copies the backup's "storage/" files into the app
// filesystem (local pb_data/storage or the configured S3 bucket).
// importBackupStorage copies the "storage/" directory from an extracted backup
// (extractedDir) into the app filesystem (local or S3). It is shared by the
// legacy SQLite import and the native pg_restore import paths.
func (app *BaseApp) importBackupStorage(ctx context.Context, extractedDir string) error {
	srcStorage := filepath.Join(extractedDir, LocalStorageDirName)

	info, err := os.Stat(srcStorage)
	if err != nil || !info.IsDir() {
		return nil // nothing to copy
	}

	fsys, err := app.NewFilesystem()
	if err != nil {
		return err
	}
	defer fsys.Close()

	fsys.SetContext(ctx)

	var count int
	walkErr := filepath.WalkDir(srcStorage, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(srcStorage, path)
		if err != nil {
			return err
		}
		// storage keys always use forward slashes
		key := filepath.ToSlash(rel)

		file, err := filesystem.NewFileFromPath(path)
		if err != nil {
			return err
		}

		if err := fsys.UploadFile(file, key); err != nil {
			return err
		}

		count++
		return nil
	})
	if walkErr != nil {
		return walkErr
	}

	app.Logger().Info("[Backup] Imported storage files", slog.Int("count", count))

	return nil
}

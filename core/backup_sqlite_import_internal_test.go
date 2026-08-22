package core

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/arief-fajri/pgbase/tools/types"
	"github.com/spf13/cast"
)

// newTempSQLite creates a throwaway on-disk SQLite database seeded with the
// provided DDL/DML statements. The "sqlite" driver is registered by the blank
// import in backup_sqlite_import.go.
func newTempSQLite(t *testing.T, stmts ...string) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "data.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("exec %q: %v", s, err)
		}
	}

	return db
}

func TestDetectSQLiteBackupVersion(t *testing.T) {
	scenarios := []struct {
		name        string
		stmts       []string
		wantVersion int
		expectError bool
		errContains string
	}{
		{
			name: "v0.23+ (superusers table + fields column)",
			stmts: []string{
				`CREATE TABLE _superusers (id TEXT)`,
				`CREATE TABLE _collections (id TEXT, fields TEXT)`,
			},
			wantVersion: 23,
		},
		{
			name: "pre-v0.23 (_admins table + schema column)",
			stmts: []string{
				`CREATE TABLE _admins (id TEXT)`,
				`CREATE TABLE _collections (id TEXT, schema TEXT)`,
			},
			wantVersion: 22,
		},
		{
			name: "pre-v0.23 (legacy schema column only)",
			stmts: []string{
				`CREATE TABLE _collections (id TEXT, schema TEXT)`,
			},
			wantVersion: 22,
		},
		{
			name: "unrecognized (no markers)",
			stmts: []string{
				`CREATE TABLE _collections (id TEXT)`,
			},
			expectError: true,
			errContains: "unrecognized backup format",
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			db := newTempSQLite(t, s.stmts...)

			tables, err := sqliteTableNames(db)
			if err != nil {
				t.Fatalf("sqliteTableNames: %v", err)
			}

			version, err := detectSQLiteBackupVersion(db, tables)
			if s.expectError {
				if err == nil {
					t.Fatalf("expected an error, got nil")
				}
				if s.errContains != "" && !strings.Contains(err.Error(), s.errContains) {
					t.Fatalf("expected error to contain %q, got %q", s.errContains, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if version != s.wantVersion {
				t.Fatalf("expected version %d, got %d", s.wantVersion, version)
			}
		})
	}
}

func TestCoerceSQLiteValueForPG(t *testing.T) {
	scenarios := []struct {
		name     string
		pgType   string
		val      any
		wantOmit bool
		wantStr  string // string form of the coerced value (when not omitted)
	}{
		{"jsonb array", "jsonb", []byte(`[1,2]`), false, `[1,2]`},
		{"jsonb nil omitted", "jsonb", nil, true, ""},
		{"jsonb empty omitted", "jsonb", "", true, ""},
		{"bool from int 1", "boolean", int64(1), false, "true"},
		{"bool from int 0", "boolean", int64(0), false, "false"},
		{"bool nil omitted", "boolean", nil, true, ""},
		{"timestamp valid", "timestamp with time zone", "2024-01-02 03:04:05.000Z", false, "2024-01-02 03:04:05.000Z"},
		{"timestamp empty omitted", "timestamp with time zone", "", true, ""},
		{"numeric int", "numeric", int64(5), false, "5"},
		{"numeric empty omitted", "numeric", "", true, ""},
		{"numeric nil omitted", "numeric", nil, true, ""},
		{"text verbatim", "text", "hello", false, "hello"},
		{"text nil omitted", "text", nil, true, ""},
		{"text from int", "text", int64(7), false, "7"},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			out, omit := coerceSQLiteValueForPG(s.pgType, s.val)
			if omit != s.wantOmit {
				t.Fatalf("expected omit=%v, got %v", s.wantOmit, omit)
			}
			if omit {
				return
			}

			var got string
			switch v := out.(type) {
			case types.JSONRaw:
				got = string(v)
			case types.DateTime:
				got = v.String()
			case bool:
				if v {
					got = "true"
				} else {
					got = "false"
				}
			default:
				got = cast.ToString(out)
			}

			if got != s.wantStr {
				t.Fatalf("expected value %q, got %q", s.wantStr, got)
			}
		})
	}
}

func TestAnyToJSONBytes(t *testing.T) {
	scenarios := []struct {
		name string
		val  any
		want string
		nilB bool
	}{
		{"nil", nil, "", true},
		{"bytes", []byte(`{"a":1}`), `{"a":1}`, false},
		{"string", `[1,2]`, `[1,2]`, false},
		{"int marshaled", 5, "5", false},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			got := anyToJSONBytes(s.val)
			if s.nilB {
				if got != nil {
					t.Fatalf("expected nil, got %q", string(got))
				}
				return
			}
			if string(got) != s.want {
				t.Fatalf("expected %q, got %q", s.want, string(got))
			}
		})
	}
}

func TestConvertLegacyField(t *testing.T) {
	t.Run("number noDecimal -> onlyInt (+ drops legacy unique)", func(t *testing.T) {
		out := convertLegacyField(map[string]any{
			"id": "f1", "name": "views", "type": "number",
			"required": true, "presentable": false, "system": false,
			"unique":  true, // legacy top-level flag must NOT be carried over
			"options": map[string]any{"noDecimal": true, "min": float64(0)},
		})
		if _, ok := out["noDecimal"]; ok {
			t.Fatalf("expected noDecimal to be renamed away")
		}
		if v, ok := out["onlyInt"].(bool); !ok || !v {
			t.Fatalf("expected onlyInt=true, got %v", out["onlyInt"])
		}
		if _, ok := out["unique"]; ok {
			t.Fatalf("legacy top-level unique must be dropped")
		}
		if out["id"] != "f1" || out["name"] != "views" || out["type"] != "number" {
			t.Fatalf("identity fields not copied: %#v", out)
		}
		if v, ok := out["required"].(bool); !ok || !v {
			t.Fatalf("expected required=true")
		}
		if out["min"] != float64(0) {
			t.Fatalf("expected number min hoisted verbatim, got %v", out["min"])
		}
	})

	t.Run("editor convertUrls -> convertURLs", func(t *testing.T) {
		out := convertLegacyField(map[string]any{
			"id": "f2", "name": "body", "type": "editor",
			"options": map[string]any{"convertUrls": false},
		})
		if _, ok := out["convertUrls"]; ok {
			t.Fatalf("expected convertUrls to be renamed away")
		}
		if v, ok := out["convertURLs"].(bool); !ok || v {
			t.Fatalf("expected convertURLs=false, got %v", out["convertURLs"])
		}
	})

	t.Run("relation drops displayFields, hoists the rest", func(t *testing.T) {
		out := convertLegacyField(map[string]any{
			"id": "f3", "name": "author", "type": "relation",
			"options": map[string]any{
				"collectionId":  "authors_col",
				"maxSelect":     1,
				"displayFields": []string{"name"},
			},
		})
		if _, ok := out["displayFields"]; ok {
			t.Fatalf("expected displayFields to be dropped")
		}
		if out["collectionId"] != "authors_col" {
			t.Fatalf("expected collectionId hoisted, got %v", out["collectionId"])
		}
		if out["maxSelect"] != 1 {
			t.Fatalf("expected maxSelect hoisted, got %v", out["maxSelect"])
		}
	})

	t.Run("text null min/max -> 0, pattern hoisted", func(t *testing.T) {
		out := convertLegacyField(map[string]any{
			"id": "f4", "name": "title", "type": "text",
			"options": map[string]any{"min": nil, "max": nil, "pattern": "^x"},
		})
		if out["min"] != 0 || out["max"] != 0 {
			t.Fatalf("expected null min/max -> 0, got min=%v max=%v", out["min"], out["max"])
		}
		if out["pattern"] != "^x" {
			t.Fatalf("expected pattern hoisted, got %v", out["pattern"])
		}
	})

	t.Run("text missing min/max -> 0", func(t *testing.T) {
		out := convertLegacyField(map[string]any{
			"id": "f5", "name": "title", "type": "text",
			"options": map[string]any{},
		})
		if out["min"] != 0 || out["max"] != 0 {
			t.Fatalf("expected missing min/max defaulted to 0, got min=%v max=%v", out["min"], out["max"])
		}
	})

	t.Run("text explicit min/max preserved", func(t *testing.T) {
		out := convertLegacyField(map[string]any{
			"id": "f6", "name": "title", "type": "text",
			"options": map[string]any{"min": float64(3), "max": float64(10)},
		})
		if out["min"] != float64(3) || out["max"] != float64(10) {
			t.Fatalf("expected explicit min/max preserved, got min=%v max=%v", out["min"], out["max"])
		}
	})

	t.Run("select values hoisted verbatim", func(t *testing.T) {
		out := convertLegacyField(map[string]any{
			"id": "f7", "name": "cat", "type": "select",
			"options": map[string]any{"maxSelect": 2, "values": []string{"a", "b"}},
		})
		got, ok := out["values"].([]string)
		if !ok || len(got) != 2 || got[0] != "a" || got[1] != "b" {
			t.Fatalf("expected values hoisted verbatim, got %#v", out["values"])
		}
	})
}

func TestCoercePGValueForSQLite(t *testing.T) {
	dt, err := types.ParseDateTime("2024-01-02 03:04:05.000Z")
	if err != nil {
		t.Fatalf("ParseDateTime: %v", err)
	}

	scenarios := []struct {
		name     string
		val      any
		wantNull bool
		wantStr  string
	}{
		{"nil -> null", nil, true, ""},
		{"invalid NullString -> null", sql.NullString{Valid: false}, true, ""},
		{"valid NullString", sql.NullString{String: "hi", Valid: true}, false, "hi"},
		{"bytes -> string", []byte(`{"a":1}`), false, `{"a":1}`},
		{"zero DateTime -> null", types.DateTime{}, true, ""},
		{"DateTime -> string", dt, false, dt.String()},
		{"bool true -> \"true\"", true, false, "true"},
		{"bool false -> \"false\"", false, false, "false"},
		{"int default cast", int64(5), false, "5"},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			out, isNull := coercePGValueForSQLite("", s.val)
			if isNull != s.wantNull {
				t.Fatalf("expected isNull=%v, got %v", s.wantNull, isNull)
			}
			if isNull {
				if out != nil {
					t.Fatalf("expected nil value when null, got %#v", out)
				}
				return
			}
			if got := cast.ToString(out); got != s.wantStr {
				t.Fatalf("expected %q, got %q", s.wantStr, got)
			}
		})
	}
}

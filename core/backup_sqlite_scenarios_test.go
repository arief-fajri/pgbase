package core_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/arief-fajri/pgbase/tools/types"
)

// -------------------------------------------------------------------
// Scenario 1 - deep per-field-type export -> import fidelity
// -------------------------------------------------------------------

// TestScenarioFieldTypeRoundTrip exercises the exporter/importer against every
// tricky field type at once (text with unicode/special chars, decimal & int
// numbers, booleans, json, single/multi select, single/multi relation, date,
// url, email, editor and geoPoint), including empty/zero/default edge values.
//
// It captures the full row JSON of each record, exports the live PostgreSQL DB
// to a portable SQLite dump, mutates the live DB (update/delete/insert), then
// re-imports the dump and asserts every record's full row JSON is restored
// byte-for-byte. This proves the value coercion round-trips faithfully for the
// whole field-type surface (the existing scenario only checks row counts).
func TestScenarioFieldTypeRoundTrip(t *testing.T) {
	ctx := context.Background()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	// parent collection for the relation fields
	catalog := core.NewBaseCollection("catalog")
	catalog.Fields.Add(&core.TextField{Name: "sku"})
	if err := app.Save(catalog); err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	catIDs := make([]string, 0, 2)
	for _, sku := range []string{"cat-a", "cat-b"} {
		rec := core.NewRecord(catalog)
		rec.Set("sku", sku)
		if err := app.Save(rec); err != nil {
			t.Fatalf("create catalog record: %v", err)
		}
		catIDs = append(catIDs, rec.Id)
	}

	// products collection spanning every tricky field type
	products := core.NewBaseCollection("products")
	products.Fields.Add(
		&core.TextField{Name: "name"},
		&core.NumberField{Name: "price"},              // decimals allowed
		&core.NumberField{Name: "qty", OnlyInt: true}, // integer only
		&core.BoolField{Name: "active"},
		&core.JSONField{Name: "meta", MaxSize: 2000000},
		&core.SelectField{Name: "single_cat", MaxSelect: 1, Values: []string{"news", "tech", "sports"}},
		&core.SelectField{Name: "multi_cat", MaxSelect: 3, Values: []string{"a", "b", "c"}},
		&core.DateField{Name: "published"},
		&core.URLField{Name: "homepage"},
		&core.EmailField{Name: "contact"},
		&core.EditorField{Name: "bio"},
		&core.GeoPointField{Name: "location"},
		&core.RelationField{Name: "owner", CollectionId: catalog.Id, MaxSelect: 1},
		&core.RelationField{Name: "related", CollectionId: catalog.Id, MaxSelect: 3},
	)
	if err := app.Save(products); err != nil {
		t.Fatalf("create products: %v", err)
	}

	// record 1 - fully populated with edge values
	p1 := core.NewRecord(products)
	p1.Set("name", "Tricky \"quotes\" & <tags> \\ back\nslash 50% café 🚀")
	p1.Set("price", 19.99)
	p1.Set("qty", 42)
	p1.Set("active", true)
	p1.Set("meta", map[string]any{"a": []any{1, 2}, "b": map[string]any{"c": true}, "note": "x\"y"})
	p1.Set("single_cat", "tech")
	p1.Set("multi_cat", []string{"a", "c"})
	p1.Set("published", "2024-05-01 12:30:00.000Z")
	p1.Set("homepage", "https://example.com/path?x=1&y=2")
	p1.Set("contact", "author@example.com")
	p1.Set("bio", "<p>Hello <b>world</b></p>")
	p1.Set("location", types.GeoPoint{Lon: 12.34, Lat: 56.78})
	p1.Set("owner", catIDs[0])
	p1.Set("related", []string{catIDs[0], catIDs[1]})
	if err := app.Save(p1); err != nil {
		t.Fatalf("create product 1: %v", err)
	}

	// record 2 - empty / zero / default edge values (unset most fields)
	p2 := core.NewRecord(products)
	p2.Set("name", "")
	p2.Set("active", false)
	// price/qty default 0, meta default NULL, multi_cat/related default [],
	// single_cat/owner default "", date/url/email/editor default ""/NULL
	if err := app.Save(p2); err != nil {
		t.Fatalf("create product 2: %v", err)
	}

	// capture the authoritative full-row snapshots (read straight from PG)
	wantP1 := rowJSON(t, app, "products", p1.Id)
	wantP2 := rowJSON(t, app, "products", p2.Id)

	// guard against a vacuous pass: assert the rich values were actually stored
	// (a silently dropped Set would otherwise make the round-trip trivially equal)
	if wantP1["active"] != true {
		t.Fatalf("precondition: expected active=true, got %#v", wantP1["active"])
	}
	if wantP1["price"] != 19.99 {
		t.Fatalf("precondition: expected price=19.99, got %#v", wantP1["price"])
	}
	if mc, ok := wantP1["multi_cat"].([]any); !ok || len(mc) != 2 {
		t.Fatalf("precondition: expected multi_cat to have 2 entries, got %#v", wantP1["multi_cat"])
	}
	if rel, ok := wantP1["related"].([]any); !ok || len(rel) != 2 {
		t.Fatalf("precondition: expected related to have 2 entries, got %#v", wantP1["related"])
	}
	if loc, ok := wantP1["location"].(map[string]any); !ok || loc["lon"] != 12.34 {
		t.Fatalf("precondition: expected location.lon=12.34, got %#v", wantP1["location"])
	}
	if _, ok := wantP1["meta"].(map[string]any); !ok {
		t.Fatalf("precondition: expected meta to be a json object, got %#v", wantP1["meta"])
	}
	if wantP2["meta"] != nil {
		t.Fatalf("precondition: expected product 2 meta to be null, got %#v", wantP2["meta"])
	}

	// export the live DB into a portable SQLite dump
	dumpDir := t.TempDir()
	dumpPath := filepath.Join(dumpDir, "data.db")
	if err := app.ExportToSQLiteFile(ctx, dumpPath); err != nil {
		t.Fatalf("ExportToSQLiteFile: %v", err)
	}

	// mutate the live DB so it diverges from the snapshot
	p1.Set("name", "MUTATED")
	p1.Set("price", 0.0)
	p1.Set("active", false)
	if err := app.Save(p1); err != nil {
		t.Fatalf("mutate product 1: %v", err)
	}
	if err := app.Delete(p2); err != nil {
		t.Fatalf("delete product 2: %v", err)
	}
	p3 := core.NewRecord(products)
	p3.Set("name", "post-snapshot")
	if err := app.Save(p3); err != nil {
		t.Fatalf("create post-snapshot product: %v", err)
	}

	// re-import the dump (DELETE + reinsert every row through the coercion)
	if err := app.ImportFromSQLiteDir(ctx, dumpDir); err != nil {
		t.Fatalf("ImportFromSQLiteDir: %v", err)
	}

	// every field of both records must be restored byte-for-byte
	gotP1 := rowJSON(t, app, "products", p1.Id)
	gotP2 := rowJSON(t, app, "products", p2.Id)

	assertRowEqual(t, "product 1 (full)", wantP1, gotP1)
	assertRowEqual(t, "product 2 (empty/defaults)", wantP2, gotP2)

	// the post-snapshot record must be gone (faithful replace restore)
	if n := countRows(t, app, "products"); n != 2 {
		t.Fatalf("expected products reset to 2 rows, got %d", n)
	}
}

// -------------------------------------------------------------------
// Scenario 2 - encrypted settings import (with and without a key)
// -------------------------------------------------------------------

// TestScenarioEncryptedSettingsImport verifies that a backup whose "_params"
// settings blob is encrypted is decrypted on import (using this instance's
// encryption key) and stored back as plaintext so it stays loadable regardless
// of the target key.
func TestScenarioEncryptedSettingsImport(t *testing.T) {
	ctx := context.Background()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	const key = "0123456789abcdef0123456789abcdef" // 32 chars (AES-256)
	restore := setEnv(t, "pb_test_env", key)
	defer restore()

	// persist an encrypted settings blob (Save encrypts when the key env is set)
	app.Settings().Meta.AppName = "encrypted-restore"
	if err := app.Save(app.Settings()); err != nil {
		t.Fatalf("save encrypted settings: %v", err)
	}

	// sanity: the stored value really is encrypted (not plaintext JSON)
	stored := paramSettingsValue(t, app)
	if json.Unmarshal([]byte(stored), &map[string]any{}) == nil {
		t.Fatalf("precondition failed: settings expected to be encrypted, got plaintext:\n%s", stored)
	}

	// export -> the dump carries the encrypted blob verbatim
	dumpDir := t.TempDir()
	if err := app.ExportToSQLiteFile(ctx, filepath.Join(dumpDir, "data.db")); err != nil {
		t.Fatalf("ExportToSQLiteFile: %v", err)
	}

	// diverge the live settings so the restore has something to overwrite
	app.Settings().Meta.AppName = "live-diverged"
	if err := app.Save(app.Settings()); err != nil {
		t.Fatalf("diverge settings: %v", err)
	}

	// import -> decrypts the dump blob and stores it back as plaintext
	if err := app.ImportFromSQLiteDir(ctx, dumpDir); err != nil {
		t.Fatalf("ImportFromSQLiteDir: %v", err)
	}

	// the persisted value must now be loadable plaintext JSON carrying the name
	after := paramSettingsValue(t, app)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(after), &decoded); err != nil {
		t.Fatalf("imported settings should be stored as plaintext JSON, got: %s", after)
	}
	if !strings.Contains(after, "encrypted-restore") {
		t.Fatalf("imported settings should carry the backup appName, got: %s", after)
	}

	// and it is loadable into the live app settings
	if err := app.ReloadSettings(); err != nil {
		t.Fatalf("ReloadSettings after import: %v", err)
	}
	if got := app.Settings().Meta.AppName; got != "encrypted-restore" {
		t.Fatalf("expected AppName restored to %q, got %q", "encrypted-restore", got)
	}
}

// TestScenarioEncryptedSettingsWithoutKey verifies that when the backup's
// settings blob is encrypted but no key is configured on the target, the
// settings import is skipped best-effort (a warning) rather than aborting the
// whole restore.
func TestScenarioEncryptedSettingsWithoutKey(t *testing.T) {
	ctx := context.Background()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	const key = "0123456789abcdef0123456789abcdef"

	// keep the test hermetic: restore the original env on exit regardless of
	// the set/unset juggling below
	prev, had := os.LookupEnv("pb_test_env")
	defer func() {
		if had {
			os.Setenv("pb_test_env", prev)
		} else {
			os.Unsetenv("pb_test_env")
		}
	}()

	// build an encrypted-settings dump (needs the key present to encrypt)
	os.Setenv("pb_test_env", key)
	app.Settings().Meta.AppName = "secret-only-backup"
	if err := app.Save(app.Settings()); err != nil {
		t.Fatalf("save encrypted settings: %v", err)
	}
	dumpDir := t.TempDir()
	if err := app.ExportToSQLiteFile(ctx, filepath.Join(dumpDir, "data.db")); err != nil {
		t.Fatalf("ExportToSQLiteFile: %v", err)
	}

	// remove the key on the target: the encrypted blob is now undecryptable
	os.Unsetenv("pb_test_env")

	// the import must still succeed (settings skipped best-effort, no panic)
	if err := app.ImportFromSQLiteDir(ctx, dumpDir); err != nil {
		t.Fatalf("import must not fail on undecryptable settings, got: %v", err)
	}

	// records were still imported (the whole restore did not abort)
	if _, err := app.FindCollectionByNameOrId("demo1"); err != nil {
		t.Fatalf("demo1 collection should still be imported: %v", err)
	}
}

// -------------------------------------------------------------------
// Scenario 3 - corrupt / unrecognized backups fail cleanly (no panic)
// -------------------------------------------------------------------

// TestScenarioCorruptBackups asserts that malformed or unrecognized data.db
// files are rejected with a clear error rather than a panic or a partial import.
func TestScenarioCorruptBackups(t *testing.T) {
	ctx := context.Background()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	t.Run("missing data.db", func(t *testing.T) {
		dir := t.TempDir() // no data.db inside
		if err := app.ImportFromSQLiteDir(ctx, dir); err == nil {
			t.Fatalf("expected an error for a missing data.db")
		}
	})

	t.Run("garbage bytes", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "data.db"), []byte("this is not a sqlite database"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := app.ImportFromSQLiteDir(ctx, dir); err == nil {
			t.Fatalf("expected an error for a non-SQLite file")
		}
	})

	t.Run("valid sqlite but unrecognized backup", func(t *testing.T) {
		dir := t.TempDir()
		db, err := sql.Open("sqlite", filepath.Join(dir, "data.db"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE foo (id TEXT)`); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()

		if err := app.ImportFromSQLiteDir(ctx, dir); err == nil {
			t.Fatalf("expected an error for an unrecognized SQLite db")
		}
	})

	t.Run("valid layout but unrecognized version", func(t *testing.T) {
		dir := t.TempDir()
		db, err := sql.Open("sqlite", filepath.Join(dir, "data.db"))
		if err != nil {
			t.Fatal(err)
		}
		// a "_collections" table with neither a "fields" nor a "schema" column
		// and no "_superusers"/"_admins" markers
		if _, err := db.Exec(`CREATE TABLE _collections (id TEXT, name TEXT)`); err != nil {
			t.Fatal(err)
		}
		_ = db.Close()

		err = app.ImportFromSQLiteDir(ctx, dir)
		if err == nil {
			t.Fatalf("expected an unrecognized-format error")
		}
		if !strings.Contains(err.Error(), "unrecognized backup format") {
			t.Fatalf("expected an unrecognized-format error, got: %v", err)
		}
	})
}

// -------------------------------------------------------------------
// Scenario 4 - collection rule nil (locked) vs "" (public) preservation
// -------------------------------------------------------------------

// TestScenarioRuleNilVsEmptyRoundTrip verifies the export/import round-trip
// preserves the meaningful distinction between a nil rule (locked /
// superusers-only) and an empty-string rule (public), across all five rule
// slots.
func TestScenarioRuleNilVsEmptyRoundTrip(t *testing.T) {
	ctx := context.Background()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	public := ""
	custom := "@request.auth.id != ''"

	perms := core.NewBaseCollection("perms")
	perms.Fields.Add(&core.TextField{Name: "label"})
	perms.ListRule = nil        // locked
	perms.ViewRule = &public    // public
	perms.CreateRule = &custom  // custom expression
	perms.UpdateRule = nil      // locked
	perms.DeleteRule = &public  // public
	if err := app.Save(perms); err != nil {
		t.Fatalf("create perms: %v", err)
	}

	dumpDir := t.TempDir()
	if err := app.ExportToSQLiteFile(ctx, filepath.Join(dumpDir, "data.db")); err != nil {
		t.Fatalf("ExportToSQLiteFile: %v", err)
	}

	if err := app.Delete(perms); err != nil {
		t.Fatalf("delete perms before restore: %v", err)
	}

	if err := app.ImportFromSQLiteDir(ctx, dumpDir); err != nil {
		t.Fatalf("ImportFromSQLiteDir: %v", err)
	}

	got, err := app.FindCollectionByNameOrId("perms")
	if err != nil {
		t.Fatalf("find perms after import: %v", err)
	}

	assertRule(t, "listRule", got.ListRule, nil)
	assertRule(t, "viewRule", got.ViewRule, &public)
	assertRule(t, "createRule", got.CreateRule, &custom)
	assertRule(t, "updateRule", got.UpdateRule, nil)
	assertRule(t, "deleteRule", got.DeleteRule, &public)
}

// -------------------------------------------------------------------
// Scenario 5 - legacy v22 backup with an empty "_admins" table
// -------------------------------------------------------------------

// TestScenarioEmptyLegacyAdmins verifies that importing a pre-v0.23 backup whose
// "_admins" table is empty does NOT wipe the existing superusers (which would
// lock the instance out); the bootstrapped superuser must be retained.
func TestScenarioEmptyLegacyAdmins(t *testing.T) {
	ctx := context.Background()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	before := countRows(t, app, core.CollectionNameSuperusers)
	if before == 0 {
		t.Fatalf("precondition failed: expected the seeded app to have superusers")
	}

	// build a legacy v22 backup and empty its "_admins" table
	legacyDir := t.TempDir()
	legacyDB := filepath.Join(legacyDir, "data.db")
	if err := buildLegacyV22DB(legacyDB); err != nil {
		t.Fatalf("buildLegacyV22DB: %v", err)
	}
	db, err := sql.Open("sqlite", legacyDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM _admins`); err != nil {
		_ = db.Close()
		t.Fatalf("empty _admins: %v", err)
	}
	_ = db.Close()

	clearNonSystemCollections(t, app)

	if err := app.ImportFromSQLiteDir(ctx, legacyDir); err != nil {
		t.Fatalf("ImportFromSQLiteDir: %v", err)
	}

	// the legacy base collections were still imported
	if got := countRows(t, app, "authors"); got != 2 {
		t.Fatalf("expected 2 authors imported, got %d", got)
	}

	// and the existing superusers were retained (not wiped by an empty _admins)
	if after := countRows(t, app, core.CollectionNameSuperusers); after != before {
		t.Fatalf("empty _admins must retain the existing superusers: before=%d after=%d", before, after)
	}
}

// -------------------------------------------------------------------
// helpers
// -------------------------------------------------------------------

// rowJSON returns the full row of the given collection record as a decoded
// JSON map (via PostgreSQL row_to_json), giving an engine-normalized view of
// every column for order-independent comparison.
func rowJSON(t *testing.T, app core.App, table, id string) map[string]any {
	t.Helper()

	var raw string
	err := app.DB().
		NewQuery(`SELECT row_to_json(r) FROM "` + table + `" r WHERE id = '` + id + `'`).
		Row(&raw)
	if err != nil {
		t.Fatalf("row_to_json %q/%q: %v", table, id, err)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decode row json for %q/%q: %v", table, id, err)
	}
	return m
}

func assertRowEqual(t *testing.T, label string, want, got map[string]any) {
	t.Helper()

	if reflect.DeepEqual(want, got) {
		return
	}

	// report the specific mismatching columns for easier debugging
	for k, wv := range want {
		gv, ok := got[k]
		if !ok {
			t.Errorf("%s: column %q missing after restore", label, k)
			continue
		}
		if !reflect.DeepEqual(wv, gv) {
			t.Errorf("%s: column %q mismatch:\n  want %#v\n  got  %#v", label, k, wv, gv)
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("%s: unexpected extra column %q after restore", label, k)
		}
	}
	t.Fatalf("%s: full row mismatch after restore", label)
}

func assertRule(t *testing.T, name string, got, want *string) {
	t.Helper()

	switch {
	case got == nil && want == nil:
		return
	case got == nil && want != nil:
		t.Fatalf("%s: expected %q, got nil (rule lost its public/empty state)", name, *want)
	case got != nil && want == nil:
		t.Fatalf("%s: expected nil (locked), got %q (rule lost its locked state)", name, *got)
	case *got != *want:
		t.Fatalf("%s: expected %q, got %q", name, *want, *got)
	}
}

// paramSettingsValue reads the raw stored "_params" settings value.
func paramSettingsValue(t *testing.T, app core.App) string {
	t.Helper()

	var v string
	if err := app.DB().NewQuery(`SELECT value FROM "_params" WHERE id = 'settings'`).Row(&v); err != nil {
		t.Fatalf("read _params settings value: %v", err)
	}
	return v
}

// setEnv sets an environment variable and returns a restore func that returns
// it to its previous state.
func setEnv(t *testing.T, key, value string) func() {
	t.Helper()

	prev, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("setenv %q: %v", key, err)
	}
	return func() {
		if had {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	}
}

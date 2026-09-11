package core_test

import (
	"testing"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/arief-fajri/pgbase/tools/list"
	"github.com/arief-fajri/pgbase/tools/types"
	"github.com/pocketbase/dbx"
)

// numericIndexOids returns the physical index relational identifiers for a table,
// keyed by index name. An index that is ever rebuilt (dropped + created) gets a
// NEW oid, so a stable oid after a collection update proves the index was NOT
// touched by the sync.
func numericIndexOids(t testing.TB, app *tests.TestApp, tableName string) map[string]int64 {
	t.Helper()

	rows := []struct {
		Name string `db:"name"`
		Oid  int64  `db:"oid"`
	}{}

	err := app.DB().NewQuery(`
		SELECT c.relname AS name, c.oid::bigint AS oid
		FROM pg_class c
		JOIN pg_index i ON i.indexrelid = c.oid
		JOIN pg_class t ON t.oid = i.indrelid
		WHERE t.relname = {:tableName}
		  AND i.indisprimary = false
	`).Bind(dbx.Params{"tableName": tableName}).All(&rows)
	if err != nil {
		t.Fatal(err)
	}

	result := make(map[string]int64, len(rows))

	for _, row := range rows {
		result[row.Name] = row.Oid
	}

	return result
}

func TestSyncCollectionIndexesCosmeticChangeDoesNotRebuild(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	collection := core.NewBaseCollection("diff_col_cos")
	collection.Fields.Add(&core.TextField{Name: "title"})
	collection.Indexes = types.JSONArray[string]{
		`CREATE INDEX "idx_cos_title" ON "diff_col_cos" ("title")`,
	}

	if err := app.Save(collection); err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}

	before := numericIndexOids(t, app, "diff_col_cos")
	if len(before) != 1 {
		t.Fatalf("expected 1 index, got %d: %v", len(before), before)
	}

	// cosmetic change: adjust a validation-only field option (Min) that does NOT
	// alter the physical column definition
	reloaded, err := app.FindCollectionByNameOrId(collection.Id)
	if err != nil {
		t.Fatal(err)
	}

	titleField := reloaded.Fields.GetByName("title").(*core.TextField)
	titleField.Min = 5

	if err := app.Save(reloaded); err != nil {
		t.Fatalf("failed to update collection: %v", err)
	}

	after := numericIndexOids(t, app, "diff_col_cos")
	if len(after) != 1 {
		t.Fatalf("expected 1 index after update, got %d: %v", len(after), after)
	}

	if before["idx_cos_title"] != after["idx_cos_title"] {
		t.Fatalf("expected cosmetic change to NOT rebuild the index, oid changed from %d to %d", before["idx_cos_title"], after["idx_cos_title"])
	}
}

func TestSyncCollectionIndexesDiffOnlyRebuildsChanged(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	collection := core.NewBaseCollection("diff_col_only")
	collection.Fields.Add(&core.TextField{Name: "title"})
	collection.Fields.Add(&core.TextField{Name: "desc"})
	collection.Indexes = types.JSONArray[string]{
		`CREATE INDEX "idx_only_a" ON "diff_col_only" ("title")`,
		`CREATE INDEX "idx_only_b" ON "diff_col_only" ("desc")`,
	}

	if err := app.Save(collection); err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}

	before := numericIndexOids(t, app, "diff_col_only")
	if len(before) != 2 {
		t.Fatalf("expected 2 indexes at create, got %d: %v", len(before), before)
	}

	// index-only change within a single save:
	//   - keep idx_only_a as-is (must NOT be rebuilt)
	//   - change idx_only_b (must be rebuilt)
	//   - remove nothing, add idx_only_c
	reloaded, err := app.FindCollectionByNameOrId(collection.Id)
	if err != nil {
		t.Fatal(err)
	}

	reloaded.Indexes = types.JSONArray[string]{
		`CREATE INDEX "idx_only_a" ON "diff_col_only" ("title")`,
		`CREATE UNIQUE INDEX "idx_only_b" ON "diff_col_only" ("desc")`,
		`CREATE INDEX "idx_only_c" ON "diff_col_only" ("title", "desc")`,
	}

	if err := app.Save(reloaded); err != nil {
		t.Fatalf("failed to update collection: %v", err)
	}

	after := numericIndexOids(t, app, "diff_col_only")
	expectedNames := []string{"idx_only_a", "idx_only_b", "idx_only_c"}
	if len(after) != len(expectedNames) {
		t.Fatalf("expected %d indexes after update, got %d: %v", len(expectedNames), len(after), after)
	}

	for name := range after {
		if !list.ExistInSlice(name, expectedNames) {
			t.Fatalf("unexpected index %q: %v", name, after)
		}
	}

	// idx_only_a unchanged -> must keep its original oid
	if before["idx_only_a"] != after["idx_only_a"] {
		t.Fatalf("expected unchanged index idx_only_a to keep its oid, got %d -> %d", before["idx_only_a"], after["idx_only_a"])
	}

	// idx_only_b changed -> must have a new oid
	if before["idx_only_b"] == after["idx_only_b"] {
		t.Fatalf("expected changed index idx_only_b to be rebuilt with a new oid, got %d twice", before["idx_only_b"])
	}
}

func TestSyncCollectionIndexesRemovedIsDropped(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	collection := core.NewBaseCollection("diff_col_rm")
	collection.Fields.Add(&core.TextField{Name: "title"})
	collection.Fields.Add(&core.TextField{Name: "desc"})
	collection.Indexes = types.JSONArray[string]{
		`CREATE INDEX "idx_rm_a" ON "diff_col_rm" ("title")`,
		`CREATE INDEX "idx_rm_b" ON "diff_col_rm" ("desc")`,
	}

	if err := app.Save(collection); err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}

	reloaded, err := app.FindCollectionByNameOrId(collection.Id)
	if err != nil {
		t.Fatal(err)
	}

	reloaded.Indexes = types.JSONArray[string]{
		`CREATE INDEX "idx_rm_a" ON "diff_col_rm" ("title")`,
	}

	if err := app.Save(reloaded); err != nil {
		t.Fatalf("failed to update collection: %v", err)
	}

	after := numericIndexOids(t, app, "diff_col_rm")
	if len(after) != 1 {
		t.Fatalf("expected exactly idx_rm_a after removal, got %d: %v", len(after), after)
	}

	if _, ok := after["idx_rm_b"]; ok {
		t.Fatalf("expected idx_rm_b to be dropped, still present: %v", after)
	}
}

func TestSyncCollectionIndexesStructuralChangeFallsBackToRebuildAll(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	collection := core.NewBaseCollection("diff_col_struct")
	collection.Fields.Add(&core.TextField{Name: "title"})
	collection.Indexes = types.JSONArray[string]{
		`CREATE INDEX "idx_struct_a" ON "diff_col_struct" ("title")`,
		`CREATE UNIQUE INDEX "idx_struct_b" ON "diff_col_struct" ("title")`,
	}

	if err := app.Save(collection); err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}

	before := numericIndexOids(t, app, "diff_col_struct")

	// structural change: add a new field AND keep both indexes identical.
	// The conservative fallback rebuilds ALL indexes on a structural change,
	// even when no single index definition changed.
	reloaded, err := app.FindCollectionByNameOrId(collection.Id)
	if err != nil {
		t.Fatal(err)
	}

	reloaded.Fields.Add(&core.TextField{Name: "extra"})

	if err := app.Save(reloaded); err != nil {
		t.Fatalf("failed to update collection: %v", err)
	}

	after := numericIndexOids(t, app, "diff_col_struct")
	if len(after) != len(before) {
		t.Fatalf("expected %d indexes after structural change, got %d: %v", len(before), len(after), after)
	}

	for name, oldOid := range before {
		newOid, ok := after[name]
		if !ok {
			t.Fatalf("index %q missing after structural change", name)
		}
		if oldOid == newOid {
			t.Fatalf("expected structural change to rebuild index %q (new oid), got %d twice", name, oldOid)
		}
	}
}

func TestSyncCollectionRenamePreservesIndexes(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	collection := core.NewBaseCollection("diff_col_ren")
	collection.Fields.Add(&core.TextField{Name: "title"})
	collection.Indexes = types.JSONArray[string]{
		`CREATE INDEX "idx_ren_title" ON "diff_col_ren" ("title")`,
	}

	if err := app.Save(collection); err != nil {
		t.Fatalf("failed to create collection: %v", err)
	}

	reloaded, err := app.FindCollectionByNameOrId(collection.Id)
	if err != nil {
		t.Fatal(err)
	}

	reloaded.Name = "diff_col_ren_new"

	if err := app.Save(reloaded); err != nil {
		t.Fatalf("failed to rename collection: %v", err)
	}

	after := numericIndexOids(t, app, "diff_col_ren_new")
	if len(after) != 1 {
		t.Fatalf("expected 1 index on renamed table, got %d: %v", len(after), after)
	}

	if _, ok := after["idx_ren_title"]; !ok {
		t.Fatalf("expected idx_ren_title to survive the rename, got %v", after)
	}
}

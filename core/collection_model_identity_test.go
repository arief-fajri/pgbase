package core_test

import (
	"strings"
	"testing"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/arief-fajri/pgbase/tools/dbutils"
	"github.com/arief-fajri/pgbase/tools/types"
)

// TestCannotDeleteFieldStillInIdentityFields verifies guard (b) of NF-2: a
// saved collection that removes a field which is still listed in
// PasswordAuth.IdentityFields must be rejected with a clear validation error.
func TestCannotDeleteFieldStillInIdentityFields(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// create an auth collection with a deletable (non-system) username field
	c := core.NewAuthCollection("guard_delete_identity")
	c.Fields.Add(&core.TextField{Name: "username"})
	c.PasswordAuth.IdentityFields = []string{"email", "username"}

	if err := app.Save(c); err != nil {
		t.Fatalf("failed to create auth collection: %v", err)
	}

	// remove the username field but keep it in IdentityFields
	reloaded, err := app.FindCollectionByNameOrId(c.Id)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.Fields.RemoveByName("username")

	err = app.Save(reloaded)
	if err == nil {
		t.Fatal("expected a validation error when deleting a field that is still an identity field")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "identity field") && !strings.Contains(strings.ToLower(err.Error()), "missing") {
		t.Fatalf("expected a clear identity-field missing error, got: %v", err)
	}
}

// TestIdentityFieldIndexMustBeFunctional verifies guard (a) of NF-2: an active
// identity field whose unique index is plain (case-sensitive) is rejected with
// a clear validation error even though the collection validator previously
// accepted any single-column unique index.
func TestIdentityFieldIndexMustBeFunctional(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	c := core.NewAuthCollection("guard_functional_identity")
	c.Fields.Add(&core.TextField{Name: "custom_id"})
	// plain (case-sensitive) unique index -> must be rejected
	c.Indexes = types.JSONArray[string]{
		`CREATE UNIQUE INDEX "idx_guard_functional_identity_custom_id" ON "guard_functional_identity" (custom_id)`,
	}
	c.PasswordAuth.IdentityFields = []string{"email", "custom_id"}

	err := app.Save(c)
	if err == nil {
		t.Fatal("expected a validation error for a non-functional identity unique index")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "lower") {
		t.Fatalf("expected a clear functional/LOWER index error, got: %v", err)
	}
}

// TestInitIdentityFieldIndexesOnSave verifies that saving an auth collection
// whose username identity field has NO unique index appends a functional
// LOWER(...) index at runtime (the onCollectionSave self-healing path).
func TestInitIdentityFieldIndexesOnSave(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	c := core.NewAuthCollection("runtime_identity_fix")
	c.Fields.Add(&core.TextField{Name: "username"})
	c.PasswordAuth.IdentityFields = []string{"email", "username"}
	// deliberately no username index -> the runtime generator must append one

	if err := app.Save(c); err != nil {
		t.Fatalf("failed to save auth collection: %v", err)
	}

	// the runtime generator must have appended the functional form
	reloaded, err := app.FindCollectionByNameOrId(c.Id)
	if err != nil {
		t.Fatal(err)
	}

	idx, ok := dbutils.FindSingleColumnUniqueIndex(reloaded.Indexes, "username")
	if !ok {
		t.Fatalf("expected a unique username index, got %v", reloaded.Indexes)
	}
	if !strings.Contains(strings.ToLower(idx.Columns[0].Name), "lower(") {
		t.Fatalf("expected the saved username index to be functional (LOWER), got %q", idx.Columns[0].Name)
	}
}

// TestRemovingIdentityFieldKeepsIndexAndSaves verifies the D-2 "keep"
// lifecycle end-to-end: removing a field from PasswordAuth.IdentityFields must
// (a) still allow the save, and (b) keep the functional unique index intact.
func TestRemovingIdentityFieldKeepsIndexAndSaves(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	c := core.NewAuthCollection("keep_identity_idx")
	c.Fields.Add(&core.TextField{Name: "username"})
	c.PasswordAuth.IdentityFields = []string{"email", "username"}

	if err := app.Save(c); err != nil {
		t.Fatalf("failed to create auth collection: %v", err)
	}

	reloaded, err := app.FindCollectionByNameOrId(c.Id)
	if err != nil {
		t.Fatal(err)
	}

	// drop username from identity fields, keep the field itself
	reloaded.PasswordAuth.IdentityFields = []string{"email"}

	if err := app.Save(reloaded); err != nil {
		t.Fatalf("failed to update auth collection after removing identity field: %v", err)
	}

	after, err := app.FindCollectionByNameOrId(c.Id)
	if err != nil {
		t.Fatal(err)
	}

	// the functional username index must be preserved (D-2 keep semantics)
	idx, ok := dbutils.FindSingleColumnUniqueIndex(after.Indexes, "username")
	if !ok {
		t.Fatalf("expected the username unique index to be kept after removing it from identity fields, got %v", after.Indexes)
	}
	if !strings.Contains(strings.ToLower(idx.Columns[0].Name), "lower(") {
		t.Fatalf("expected the kept username index to remain functional, got %q", idx.Columns[0].Name)
	}
}

// TestInitIdentityFieldIndexesDoesNotDuplicateFunctionalIndex verifies that
// the runtime generator leaves an existing functional (LOWER) unique index
// untouched instead of appending a duplicate — the idempotency property needed
// because it runs on every auth collection save.
func TestInitIdentityFieldIndexesDoesNotDuplicateFunctionalIndex(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	c := core.NewAuthCollection("runtime_identity_idem")
	c.Fields.Add(&core.TextField{Name: "username"})
	c.PasswordAuth.IdentityFields = []string{"email", "username"}
	c.Indexes = types.JSONArray[string]{
		`CREATE UNIQUE INDEX "idx_runtime_identity_idem_username" ON "runtime_identity_idem" (LOWER("username")) WHERE "username" <> ''`,
	}

	if err := app.Save(c); err != nil {
		t.Fatalf("failed to save auth collection: %v", err)
	}

	reloaded, err := app.FindCollectionByNameOrId(c.Id)
	if err != nil {
		t.Fatal(err)
	}

	// exactly one unique username index must exist (no duplicate appended)
	indexes := 0
	for _, raw := range reloaded.Indexes {
		parsed := dbutils.ParseIndex(raw)
		if parsed.Unique && dbutils.NormalizeIndexColumnName(parsed.Columns[0].Name) == "username" {
			indexes++
		}
	}
	if indexes != 1 {
		t.Fatalf("expected exactly 1 unique username index after save, got %d: %v", indexes, reloaded.Indexes)
	}
}

// TestInitIdentityFieldIndexesOnNewAuthCollection verifies that saving a
// freshly constructed auth collection with a non-email identity field appends
// the functional index at runtime (initIdentityFieldIndexes in the
// onCollectionSave path).
func TestInitIdentityFieldIndexesOnNewAuthCollection(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	c := core.NewAuthCollection("fresh_company_users")
	c.Fields.Add(&core.TextField{Name: "employee_code"})
	c.PasswordAuth.IdentityFields = []string{"email", "employee_code"}

	if err := app.Save(c); err != nil {
		t.Fatalf("failed to save auth collection: %v", err)
	}

	reloaded, err := app.FindCollectionByNameOrId(c.Id)
	if err != nil {
		t.Fatal(err)
	}

	idx, ok := dbutils.FindSingleColumnUniqueIndex(reloaded.Indexes, "employee_code")
	if !ok {
		t.Fatalf("expected a unique employee_code index to exist, got %v", reloaded.Indexes)
	}
	if !strings.Contains(strings.ToLower(idx.Columns[0].Name), "lower(") {
		t.Fatalf("expected the employee_code index to be functional, got %q", idx.Columns[0].Name)
	}
}

package migrations_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/migrations"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/arief-fajri/pgbase/tools/dbutils"
	"github.com/arief-fajri/pgbase/tools/types"
)

// newCaseSensitiveUsernameAuthCollection creates and saves a minimal auth
// collection whose username unique index is the legacy case-sensitive form,
// simulating a database created before the functional-index change.
//
// Unlike the email twin, the username index is created AFTER saving the
// collection (username is not a system field and the collection is valid
// without its index), which mirrors how the legacy v0.23 migration produced a
// plain username index.
func newCaseSensitiveUsernameAuthCollection(t *testing.T, app core.App, name, indexName string) *core.Collection {
	t.Helper()

	c := core.NewAuthCollection(name)
	c.PasswordAuth = core.PasswordAuthConfig{
		Enabled:        true,
		IdentityFields: []string{"email", "username"},
	}
	c.Fields.Add(&core.TextField{Name: "username"})
	c.Indexes = types.JSONArray[string]{
		`CREATE UNIQUE INDEX "` + indexName + `" ON "` + name + `" (username) WHERE username <> ''`,
	}

	// SaveNoValidate is used (not Save) because it simulates a database that
	// predates the functional-index validator: a plain case-sensitive identity
	// index is stored without the collection validator rejecting it. The
	// runtime initIdentityFieldIndexes still runs appending nothing (it finds
	// the plain single-column unique index via FindSingleColumnUniqueIndex),
	// so the stored physical index remains case-sensitive.
	//
	// Migrations themselves bypass the validator the same way, so this matches
	// how the index would exist on a real pre-upgrade database.
	if err := app.SaveNoValidate(c); err != nil {
		t.Fatalf("failed to save %q auth collection: %v", name, err)
	}

	// sanity: the physical index must be case-sensitive (no LOWER()) at this point
	if def := readIndexDef(t, app, indexName); strings.Contains(strings.ToLower(def), "lower(") {
		t.Fatalf("expected a case-sensitive username index, got %q", def)
	}

	return c
}

func TestConvertAuthIdentityIndexesToFunctional(t *testing.T) {
	t.Run("converts case-sensitive username index to functional", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		const name = "test_identity_conv"
		const indexName = "idx_test_identity_conv_username"
		newCaseSensitiveUsernameAuthCollection(t, app, name, indexName)

		if err := migrations.ConvertAuthIdentityIndexesToFunctional(app); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// physical index must now be functional
		if def := readIndexDef(t, app, indexName); !strings.Contains(strings.ToLower(def), "lower(") {
			t.Fatalf("expected a functional LOWER() username index, got %q", def)
		}

		// metadata must reflect the functional form and keep the same index name
		reloaded, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		idx, ok := dbutils.FindSingleColumnUniqueIndex(reloaded.Indexes, "username")
		if !ok {
			t.Fatalf("expected to still find a single-column unique username index in %v", reloaded.Indexes)
		}
		if idx.IndexName != indexName {
			t.Fatalf("expected the index name %q to be preserved, got %q", indexName, idx.IndexName)
		}
		if !strings.Contains(strings.ToLower(idx.Columns[0].Name), "lower(") {
			t.Fatalf("expected metadata to use LOWER(username), got %q", idx.Columns[0].Name)
		}
	})

	t.Run("idempotent on already-functional index", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		// the seeded users collection now has a functional username index
		before, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			t.Fatal(err)
		}
		beforeIdx, ok := dbutils.FindSingleColumnUniqueIndex(before.Indexes, "username")
		if !ok {
			t.Fatal("expected users to have a unique username index")
		}
		beforeDef := readIndexDef(t, app, beforeIdx.IndexName)

		if err := migrations.ConvertAuthIdentityIndexesToFunctional(app); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// nothing should have changed (same name + same physical definition)
		if afterDef := readIndexDef(t, app, beforeIdx.IndexName); afterDef != beforeDef {
			t.Fatalf("expected the functional index to be untouched\n%q\nvs\n%q", beforeDef, afterDef)
		}
	})

	t.Run("aborts on case-variant duplicate usernames", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		const name = "test_identity_dup"
		const indexName = "idx_test_identity_dup_username"
		collection := newCaseSensitiveUsernameAuthCollection(t, app, name, indexName)

		// two records whose usernames differ only by letter case (allowed by
		// the case-sensitive index, rejected by the case-insensitive one);
		// distinct emails so only the username drives the duplicate
		for i, username := range []string{"bob", "BOB"} {
			r := core.NewRecord(collection)
			r.Set("username", username)
			r.SetEmail(fmt.Sprintf("bob%d@example.com", i))
			r.SetPassword("1234567890")
			if err := app.Save(r); err != nil {
				t.Fatalf("failed to save record %q: %v", username, err)
			}
		}

		err := migrations.ConvertAuthIdentityIndexesToFunctional(app)
		if err == nil {
			t.Fatal("expected an error due to case-variant duplicate usernames, got nil")
		}
		if !strings.Contains(err.Error(), name) || !strings.Contains(strings.ToLower(err.Error()), "case") {
			t.Fatalf("expected a descriptive duplicate-identity error, got: %v", err)
		}

		// the index must remain untouched (still case-sensitive) so the boot
		// fails loudly instead of silently leaving a half-applied state
		if def := readIndexDef(t, app, indexName); strings.Contains(strings.ToLower(def), "lower(") {
			t.Fatalf("expected the username index to remain case-sensitive after abort, got %q", def)
		}
	})

	t.Run("skips non-text identity fields", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		// a number identity field cannot be indexed with LOWER() and must be
		// left untouched (no attempt to build a broken functional index)
		const name = "test_identity_numeric"
		c := core.NewAuthCollection(name)
		c.Fields.Add(&core.NumberField{Name: "emp_no"})
		c.PasswordAuth = core.PasswordAuthConfig{
			Enabled:        true,
			IdentityFields: []string{"email", "emp_no"},
		}
		c.Indexes = types.JSONArray[string]{
			`CREATE UNIQUE INDEX "idx_test_identity_numeric_emp_no" ON "test_identity_numeric" (emp_no)`,
		}

		// SaveNoValidate: a number identity with a plain index would be
		// rejected by the functional-index validator (NF-2), but the converter
		// must still handle existing legacy databases that predate it.
		if err := app.SaveNoValidate(c); err != nil {
			t.Fatalf("failed to save auth collection: %v", err)
		}

		if err := migrations.ConvertAuthIdentityIndexesToFunctional(app); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		reloaded, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		idx, ok := dbutils.FindSingleColumnUniqueIndex(reloaded.Indexes, "emp_no")
		if !ok {
			t.Fatalf("expected the plain emp_no unique index to remain, got %v", reloaded.Indexes)
		}
		if strings.Contains(strings.ToLower(idx.Columns[0].Name), "lower(") {
			t.Fatalf("expected the non-text emp_no index to remain plain, got %q", idx.Columns[0].Name)
		}
	})
}

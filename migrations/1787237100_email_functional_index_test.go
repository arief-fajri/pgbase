package migrations_test

import (
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/migrations"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/arief-fajri/pgbase/tools/dbutils"
	"github.com/arief-fajri/pgbase/tools/types"
)

// readIndexDef returns the physical PostgreSQL definition of the named index
// (empty string if it doesn't exist).
func readIndexDef(t *testing.T, app core.App, name string) string {
	t.Helper()
	var def string
	err := app.DB().NewQuery(`SELECT indexdef FROM pg_indexes WHERE indexname = {:name}`).
		Bind(dbx.Params{"name": name}).
		Row(&def)
	if err != nil && !strings.Contains(err.Error(), "no rows") {
		t.Fatalf("failed to read index def for %q: %v", name, err)
	}
	return def
}

// newCaseSensitiveAuthCollection creates and saves a minimal auth collection
// whose email unique index is the legacy case-sensitive form, simulating a
// database created before the functional-index change.
func newCaseSensitiveAuthCollection(t *testing.T, app core.App, name, indexName string) *core.Collection {
	t.Helper()

	c := core.NewAuthCollection(name)
	c.PasswordAuth = core.PasswordAuthConfig{
		Enabled:        true,
		IdentityFields: []string{"email"},
	}
	c.Indexes = types.JSONArray[string]{
		`CREATE UNIQUE INDEX "` + indexName + `" ON "` + name + `" (email) WHERE email <> ''`,
	}

	if err := app.SaveNoValidate(c); err != nil {
		t.Fatalf("failed to save %q auth collection: %v", name, err)
	}

	// sanity: the physical index must be case-sensitive (no LOWER()) at this point
	if def := readIndexDef(t, app, indexName); strings.Contains(strings.ToLower(def), "lower(") {
		t.Fatalf("expected a case-sensitive email index, got %q", def)
	}

	return c
}

func TestConvertAuthEmailIndexesToFunctional(t *testing.T) {
	t.Run("converts case-sensitive email index to functional", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		const name = "test_email_conv"
		const indexName = "idx_test_email_conv_email"
		newCaseSensitiveAuthCollection(t, app, name, indexName)

		if err := migrations.ConvertAuthEmailIndexesToFunctional(app); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// physical index must now be functional
		if def := readIndexDef(t, app, indexName); !strings.Contains(strings.ToLower(def), "lower(") {
			t.Fatalf("expected a functional LOWER() email index, got %q", def)
		}

		// metadata must reflect the functional form and keep the same index name
		reloaded, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		idx, ok := dbutils.FindSingleColumnUniqueIndex(reloaded.Indexes, core.FieldNameEmail)
		if !ok {
			t.Fatalf("expected to still find a single-column unique email index in %v", reloaded.Indexes)
		}
		if idx.IndexName != indexName {
			t.Fatalf("expected the index name %q to be preserved, got %q", indexName, idx.IndexName)
		}
		if !strings.Contains(strings.ToLower(idx.Columns[0].Name), "lower(") {
			t.Fatalf("expected metadata to use LOWER(email), got %q", idx.Columns[0].Name)
		}
	})

	t.Run("idempotent on already-functional index", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		// the default users collection already has a functional email index
		before, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			t.Fatal(err)
		}
		beforeIdx, ok := dbutils.FindSingleColumnUniqueIndex(before.Indexes, core.FieldNameEmail)
		if !ok {
			t.Fatal("expected users to have a unique email index")
		}
		beforeDef := readIndexDef(t, app, beforeIdx.IndexName)

		if err := migrations.ConvertAuthEmailIndexesToFunctional(app); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// nothing should have changed (same name + same physical definition)
		if afterDef := readIndexDef(t, app, beforeIdx.IndexName); afterDef != beforeDef {
			t.Fatalf("expected the functional index to be untouched\n%q\nvs\n%q", beforeDef, afterDef)
		}
	})

	t.Run("aborts on case-variant duplicate emails", func(t *testing.T) {
		app, _ := tests.NewTestApp()
		defer app.Cleanup()

		const name = "test_email_dup"
		const indexName = "idx_test_email_dup_email"
		collection := newCaseSensitiveAuthCollection(t, app, name, indexName)

		// two records whose emails differ only by letter case (allowed by the
		// case-sensitive index, rejected by the case-insensitive one)
		for _, email := range []string{"dup@example.com", "DUP@example.com"} {
			r := core.NewRecord(collection)
			r.SetEmail(email)
			r.SetPassword("1234567890")
			if err := app.Save(r); err != nil {
				t.Fatalf("failed to save record %q: %v", email, err)
			}
		}

		err := migrations.ConvertAuthEmailIndexesToFunctional(app)
		if err == nil {
			t.Fatal("expected an error due to case-variant duplicate emails, got nil")
		}
		if !strings.Contains(err.Error(), name) || !strings.Contains(strings.ToLower(err.Error()), "case") {
			t.Fatalf("expected a descriptive duplicate-email error, got: %v", err)
		}

		// the index must remain untouched (still case-sensitive) so the boot
		// fails loudly instead of silently leaving a half-applied state
		if def := readIndexDef(t, app, indexName); strings.Contains(strings.ToLower(def), "lower(") {
			t.Fatalf("expected the email index to remain case-sensitive after abort, got %q", def)
		}
	})
}

package core

import (
	"strings"
	"testing"
)

// w13ProbeModel is a minimal Model implementation whose type does not match
// the collection/record system hook type switches, so Save/Delete exercise
// the bare model write path (the db.go create/delete inner functions)
// without any db-touching hooks in between.
type w13ProbeModel struct {
	BaseModel
}

func (m *w13ProbeModel) TableName() string { return "w13_probe" }

// TestQueriesOnResetAppFailExplicitly verifies W-13 (failure-modes.md):
// ResetBootstrapState nils the db handles, so every entry point of the model
// and record query family must fail with an explicit error instead of
// nil-dereferencing its dbx.Builder - the panic that used to be recovered
// inside the FireAndForget installer goroutine racing terminate.
func TestQueriesOnResetAppFailExplicitly(t *testing.T) {
	app, _ := newTimeoutTestApp(t, BaseAppConfig{})

	// positive control: the same read paths work on a live app so a guard
	// that misfires while bootstrapped would be caught here
	if _, err := app.FindAllCollections(); err != nil {
		t.Fatalf("live FindAllCollections: %v", err)
	}
	var liveCols []*Collection
	if err := app.ModelQuery(&Collection{}).Limit(1).All(&liveCols); err != nil {
		t.Fatalf("live ModelQuery: %v", err)
	}
	var liveRecs []*Record
	if err := app.RecordQuery(CollectionNameSuperusers).Limit(1).All(&liveRecs); err != nil {
		t.Fatalf("live RecordQuery: %v", err)
	}

	if err := app.ResetBootstrapState(); err != nil {
		t.Fatalf("ResetBootstrapState: %v", err)
	}

	// every entry point must return the explicit not-bootstrapped error
	// (and never panic) once the db handles are gone
	mustFail := func(name string, fn func() error) {
		t.Helper()

		err := fn()
		if err == nil {
			t.Fatalf("%s: expected an explicit error after reset, got nil", name)
		}
		if !strings.Contains(err.Error(), "not bootstrapped") {
			t.Fatalf("%s: expected the not-bootstrapped error, got: %v", name, err)
		}
	}

	mustFail("FindAllCollections", func() error {
		_, err := app.FindAllCollections()
		return err
	})

	mustFail("ModelQuery", func() error {
		var cols []*Collection
		return app.ModelQuery(&Collection{}).Limit(1).All(&cols)
	})

	mustFail("AuxModelQuery", func() error {
		var cols []*Collection
		return app.AuxModelQuery(&Collection{}).Limit(1).All(&cols)
	})

	mustFail("RecordQuery", func() error {
		var recs []*Record
		return app.RecordQuery(CollectionNameSuperusers).Limit(1).All(&recs)
	})

	mustFail("Save", func() error {
		return app.Save(&w13ProbeModel{})
	})

	mustFail("Delete", func() error {
		m := &w13ProbeModel{}
		m.Id = "w13probe1234567"
		m.MarkAsNotNew() // non-empty LastSavedPK to pass the delete pk check
		return app.Delete(m)
	})
}

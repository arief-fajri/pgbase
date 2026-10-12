package apis_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
)

// TestRealtimeOutboxDeletePublishesOnlyAfterCommit is the W-01 regression
// test: a delete whose transaction fails must not publish any outbox event
// (nor leave a stashed snapshot behind that a later delete could flush).
// The failure is injected at OnModelDeleteExecute — i.e. AFTER the pre-commit
// OnModelDelete hook (dry-cache + snapshot stash) has run but BEFORE the row
// is removed — which is exactly the window the old pre-commit publish got
// wrong.
func TestRealtimeOutboxDeletePublishesOnlyAfterCommit(t *testing.T) {
	t.Setenv("PB_REALTIME_OUTBOX", "1")

	scenario := tests.ApiScenario{
		Name:   "failed delete publishes no outbox event",
		Method: http.MethodGet,
		URL:    "/api/collections/demo1/records",
		Headers: map[string]string{
			"Authorization": superuserTestToken,
		},
		ExpectedStatus:  200,
		ExpectedContent: []string{`"items":`},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			collection, err := app.FindCollectionByNameOrId("demo1")
			if err != nil {
				t.Fatal(err)
			}
			rec := core.NewRecord(collection)
			rec.Set("email", "w01-victim@example.com")
			rec.Set("text", "w01 must not publish")
			if err := app.Save(rec); err != nil {
				t.Fatal(err)
			}

			// fail the delete AFTER the pre-commit hooks ran
			hookId := app.OnModelDeleteExecute("demo1").BindFunc(func(e *core.ModelEvent) error {
				return errors.New("w01 injected delete failure")
			})
			defer app.OnModelDeleteExecute().Unbind(hookId)

			if err := app.Delete(rec); err == nil {
				t.Fatal("expected the injected delete failure, got nil")
			}

			// the record must still exist (the delete really failed)
			if _, err := app.FindRecordById(collection, rec.Id); err != nil {
				t.Fatalf("expected the record to survive the failed delete, got %v", err)
			}
		},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			var deletes int64
			if err := app.DB().
				NewQuery(`SELECT COUNT(*) FROM "_realtime_outbox" WHERE "action" = 'delete'`).
				Row(&deletes); err != nil {
				t.Fatal(err)
			}
			if deletes != 0 {
				t.Fatalf("expected 0 outbox delete events after a failed delete, got %d", deletes)
			}
		},
	}
	scenario.Test(t)
}

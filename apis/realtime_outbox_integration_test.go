package apis_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
)

// superuserTestToken is a statically-signed superuser JWT matching the test
// seed data (see apis/record_crud_test.go).
const superuserTestToken = "eyJhbGciOiJIUzI1NiJ9.eyJpZCI6InN5d2JoZWNuaDQ2cmhtMCIsInR5cGUiOiJhdXRoIiwiY29sbGVjdGlvbklkIjoicGJjXzMxNDI2MzU4MjMiLCJleHAiOjI1MjQ2MDQ0NjEsInJlZnJlc2hhYmxlIjp0cnVlfQ.UXgO3j-0BumcugrFjbd7j0M4MQvbrLggLlcu_YNGjoY"

// TestRealtimeOutboxPublishesOnRecordSave verifies the D-4 step-1 wiring
// end-to-end through the served app (realtime hooks bound): saving a record
// appends an outbox create event when enabled, and nothing when disabled.
func TestRealtimeOutboxPublishesOnRecordSave(t *testing.T) {
	check := func(enabled bool) func(t testing.TB, app *tests.TestApp, res *http.Response) {
		return func(t testing.TB, app *tests.TestApp, res *http.Response) {
			var count int64
			err := app.DB().
				NewQuery(`SELECT COUNT(*) FROM "_realtime_outbox" WHERE "collection" = 'demo1' AND "action" = 'create'`).
				Row(&count)
			if err != nil {
				t.Fatal(err)
			}

			if enabled && count != 1 {
				t.Fatalf("expected 1 outbox create event when enabled, got %d", count)
			}
			if !enabled && count != 0 {
				t.Fatalf("expected 0 outbox events when disabled, got %d", count)
			}
		}
	}

	for _, enabled := range []bool{false, true} {
		enabled := enabled
		t.Run(fmt.Sprintf("enabled=%v", enabled), func(t *testing.T) {
			if enabled {
				t.Setenv("PB_REALTIME_OUTBOX", "1")
			} else {
				t.Setenv("PB_REALTIME_OUTBOX", "0")
			}

			scenario := tests.ApiScenario{
				Method: http.MethodPost,
				URL:    "/api/collections/demo1/records",
				Body:   strings.NewReader(`{"email":"outbox@example.com","text":"outbox integration"}`),
				Headers: map[string]string{
					"Authorization": superuserTestToken,
				},
				ExpectedStatus: 200,
				ExpectedContent: []string{
					`"id":`,
					`"email":"outbox@example.com"`,
				},
				AfterTestFunc: check(enabled),
			}
			scenario.Test(t)
		})
	}
}

// TestRealtimeOutboxDeleteSnapshot verifies the delete flow stores a snapshot
// row so other instances can re-evaluate their subscribers after the delete
// commits. The create + delete are performed on the served app in
// BeforeTestFunc (realtime hooks bound), and the outbox is asserted after.
func TestRealtimeOutboxDeleteSnapshot(t *testing.T) {
	t.Setenv("PB_REALTIME_OUTBOX", "1")

	scenario := tests.ApiScenario{
		Name:   "delete stores record snapshot",
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
			rec.Set("email", "to@delete.com")
			rec.Set("text", "to delete")
			if err := app.Save(rec); err != nil {
				t.Fatal(err)
			}
			if err := app.Delete(rec); err != nil {
				t.Fatal(err)
			}
		},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			rows := []struct {
				Action   string `db:"action"`
				Snapshot string `db:"snapshot"`
			}{}
			if err := app.DB().
				NewQuery(`SELECT "action", COALESCE("snapshot"::text, '') AS "snapshot" FROM "_realtime_outbox" ORDER BY "created"`).
				All(&rows); err != nil {
				t.Fatal(err)
			}

			// create + delete events; only the delete carries a snapshot
			if len(rows) != 2 {
				t.Fatalf("expected 2 outbox rows (create + delete), got %d", len(rows))
			}
			if rows[0].Action != "create" || rows[0].Snapshot != "" {
				t.Fatalf("expected the create event to carry no snapshot, got %+v", rows[0])
			}
			if rows[1].Action != "delete" {
				t.Fatalf("expected the second row to be the delete event, got %+v", rows[1])
			}
			if !strings.Contains(rows[1].Snapshot, `"text": "to delete"`) || !strings.Contains(rows[1].Snapshot, `"id": "`) {
				t.Fatalf("expected the delete snapshot to carry the record, got %q", rows[1].Snapshot)
			}
		},
	}
	scenario.Test(t)
}

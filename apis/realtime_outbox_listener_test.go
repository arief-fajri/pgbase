package apis_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/arief-fajri/pgbase/tools/subscriptions"
)

// TestRealtimeOutboxListenerRebroadcasts verifies the D-4 step-2 listener
// end-to-end through the served app: an event published to the outbox (as if
// by another instance) is re-broadcast to THIS instance's local realtime
// subscriber.
func TestRealtimeOutboxListenerRebroadcasts(t *testing.T) {
	t.Setenv("PB_REALTIME_OUTBOX", "1")

	client := subscriptions.NewDefaultClient()

	scenario := tests.ApiScenario{
		Name:           "listener re-broadcasts a create event",
		Method:         http.MethodGet,
		URL:            "/api/collections/demo2/records",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			`"items"`,
		},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			// realtime subscription topic: "<collectionName>/<recordId>?"
			client.Subscribe("demo2/0yxhwia2amd8gec")
			app.SubscriptionsBroker().Register(client)

			// simulate an event written by ANOTHER instance for a demo2 record
			if err := app.PublishRealtimeEvent(core.RealtimeActionCreate, "demo2", "0yxhwia2amd8gec", nil); err != nil {
				t.Fatalf("failed to publish realtime event: %v", err)
			}
		},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			// the async listener re-fetches the demo2 record and broadcasts it
			// to the local subscriber; wait (bounded) for the message
			deadline := time.Now().Add(6 * time.Second)
			for time.Now().Before(deadline) {
				select {
				case msg := <-client.Channel():
					if !strings.Contains(msg.Name, "demo2/0yxhwia2amd8gec") {
						t.Fatalf("unexpected message topic %q", msg.Name)
					}
					if len(msg.Data) == 0 {
						t.Fatalf("expected the broadcast to carry record data, got empty for %q", msg.Name)
					}
					return // success
				case <-time.After(200 * time.Millisecond):
				}
			}
			t.Fatal("timed out waiting for the listener to re-broadcast the outbox event")
		},
	}
	scenario.Test(t)
}

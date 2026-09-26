package core

import (
	"os"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
)

func newTestInstanceHeartbeatApp(t *testing.T, dataDir string) *BaseApp {
	t.Helper()

	app := NewBaseApp(BaseAppConfig{
		DataDir:          dataDir,
		IsDev:            true,
		DataMaxOpenConns: 2,
		AuxMaxOpenConns:  2,
	})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("failed to bootstrap test app: %v", err)
	}

	return app
}

func cleanupInstanceHeartbeatApp(t *testing.T, app *BaseApp) {
	t.Helper()

	// remove this instance's heartbeat row so it does not leak into the shared DB
	// (nil-safe: the app may already have been terminated or reset, which
	// closes the pools before this helper runs)
	guard := app.instanceHeartbeatGuard
	if db := app.AuxNonconcurrentDB(); guard != nil && db != nil {
		_, _ = db.
			NewQuery(`DELETE FROM "_instance_heartbeats" WHERE "instance_id" = {:id}`).
			Bind(dbx.Params{"id": guard.instanceID}).
			Execute()
	}

	app.ResetBootstrapState()
}

// TestInstanceHeartbeatGuardMultiInstance verifies that two apps sharing the
// same database detect each other via the heartbeat table (R-1b, two-instance
// acceptance), without warning in the single-instance case.
//
// This test uses the raw (non-isolated) DB path like notify_watcher_test, so it
// must not run parallel with other raw-DB tests and must start from a clean
// heartbeat table.
func TestInstanceHeartbeatGuardMultiInstance(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "pb_heartbeat*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	app1 := newTestInstanceHeartbeatApp(t, dataDir)
	defer cleanupInstanceHeartbeatApp(t, app1)

	// start from a clean table so no leftover rows from other raw-DB tests
	// could masquerade as a live peer
	if _, err := app1.AuxNonconcurrentDB().
		NewQuery(`DELETE FROM "_instance_heartbeats"`).
		Execute(); err != nil {
		t.Fatal(err)
	}

	// single instance: no peers, no transition
	guard1 := app1.instanceHeartbeatGuard
	if guard1 == nil {
		t.Fatal("expected app1 to have an instance heartbeat guard")
	}
	multi, err := guard1.hasPeer()
	if err != nil {
		t.Fatal(err)
	}
	if multi {
		t.Fatal("expected no multi-instance peers when running alone")
	}
	if guard1.lastMulti {
		t.Fatal("expected lastMulti to stay false when running alone")
	}

	// start a second instance against the same database
	app2 := newTestInstanceHeartbeatApp(t, dataDir)
	defer cleanupInstanceHeartbeatApp(t, app2)

	guard2 := app2.instanceHeartbeatGuard
	if guard2 == nil {
		t.Fatal("expected app2 to have an instance heartbeat guard")
	}

	// wait for both tickers to write a heartbeat, then detect
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(200 * time.Millisecond)

		guard1.runCycle()
		guard2.runCycle()

		if guard1.lastMulti && guard2.lastMulti {
			break
		}
	}

	if !guard1.lastMulti {
		t.Fatal("expected app1 to detect app2 (lastMulti=true)")
	}
	if !guard2.lastMulti {
		t.Fatal("expected app2 to detect app1 (lastMulti=true)")
	}

	// the transition must be one-way: once multi, stay multi (no repeated invites)
	guard1.runCycle()
	if !guard1.lastMulti {
		t.Fatal("expected app1 to remain multi after further cycles")
	}
}

// TestInstanceHeartbeatGuardStaleRowsPurged verifies the hourly cleanup cron
// removes rows of dead instances (stale beyond the configured age).
func TestInstanceHeartbeatGuardStaleRowsPurged(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "pb_heartbeat_purge*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	app := newTestInstanceHeartbeatApp(t, dataDir)
	defer cleanupInstanceHeartbeatApp(t, app)

	// insert a stale (dead peer) row and a fresh one
	_, err = app.AuxNonconcurrentDB().
		NewQuery(`INSERT INTO "_instance_heartbeats" ("instance_id", "seen_at") VALUES
			('@dead'      , NOW() - interval '2 hours'),
			('@just_seen' , NOW())
		`).
		Execute()
	if err != nil {
		t.Fatal(err)
	}

	guard := app.instanceHeartbeatGuard
	if err := guard.purgeStaleRows(); err != nil {
		t.Fatal(err)
	}

	var count int64
	err = app.AuxNonconcurrentDB().
		NewQuery(`SELECT COUNT(*) FROM "_instance_heartbeats" WHERE "instance_id" IN ('@dead','@just_seen')`).
		Row(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected only the fresh row to survive purge, got %d", count)
	}
}

// TestInstanceHeartbeatGuardBootstrapChurn (W-12) verifies that re-bootstrap
// WITHOUT the terminate chain (Bootstrap -> ResetBootstrapState -> Bootstrap,
// where ResetBootstrapState never runs cleanup) stops and drains the previous
// heartbeat goroutine instead of sharing its channels: a reused done channel
// would double-close ("close of closed channel") at terminate and the leaked
// goroutine would tick forever.
//
// It uses the raw (non-isolated) DB path like the tests above, so it must not
// run parallel with other raw-DB tests.
func TestInstanceHeartbeatGuardBootstrapChurn(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "pb_heartbeat_churn*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	app := newTestInstanceHeartbeatApp(t, dataDir)
	defer cleanupInstanceHeartbeatApp(t, app)

	guard := app.instanceHeartbeatGuard
	if guard == nil {
		t.Fatal("expected app to have an instance heartbeat guard")
	}

	guard.mu.Lock()
	firstDone := guard.done
	guard.mu.Unlock()
	if firstDone == nil {
		t.Fatal("expected a live heartbeat goroutine after bootstrap")
	}

	// churn: reset without the terminate chain, then bootstrap again
	if err := app.ResetBootstrapState(); err != nil {
		t.Fatalf("failed to reset test app state: %v", err)
	}
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("failed to re-bootstrap test app: %v", err)
	}

	guard.mu.Lock()
	secondDone := guard.done
	running := guard.running
	guard.mu.Unlock()

	if !running {
		t.Fatal("expected the guard to be running after re-bootstrap")
	}
	if secondDone == firstDone {
		t.Fatal("re-bootstrap reused the previous heartbeat goroutine's done channel (W-12 double close)")
	}

	// the previous goroutine must have been stopped and drained by the
	// re-bootstrap (pre-W-12 it kept ticking forever)
	select {
	case <-firstDone:
	case <-time.After(3 * time.Second):
		t.Fatal("the previous heartbeat goroutine leaked across re-bootstrap (W-12)")
	}

	// clean up our heartbeat row while the pools are still open so it cannot
	// masquerade as a live peer for other raw-DB tests
	if _, err := app.AuxNonconcurrentDB().
		NewQuery(`DELETE FROM "_instance_heartbeats" WHERE "instance_id" = {:id}`).
		Bind(dbx.Params{"id": guard.instanceID}).
		Execute(); err != nil {
		t.Fatal(err)
	}

	// terminate must stop the fresh goroutine exactly once (a shared channel
	// would panic "close of closed channel" here)
	event := &TerminateEvent{App: app}
	if err := app.OnTerminate().Trigger(event, func(e *TerminateEvent) error {
		return e.Next()
	}); err != nil {
		t.Fatalf("terminate chain failed: %v", err)
	}

	select {
	case <-secondDone:
	case <-time.After(3 * time.Second):
		t.Fatal("the heartbeat goroutine did not stop on terminate")
	}

	guard.mu.Lock()
	running = guard.running
	stopCh := guard.stopCh
	guard.mu.Unlock()
	if running || stopCh != nil {
		t.Fatal("expected the guard to be fully stopped after terminate")
	}

	if err := app.ResetBootstrapState(); err != nil {
		t.Fatalf("failed to close test app pools: %v", err)
	}
}

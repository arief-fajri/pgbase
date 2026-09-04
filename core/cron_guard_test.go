package core

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCronLockKeyStable(t *testing.T) {
	a := cronLockKey("__pbDBVacuum__")
	b := cronLockKey("__pbDBVacuum__")
	c := cronLockKey("__pbLogsCleanup__")

	if a != b {
		t.Fatalf("expected config key to be stable, got %d and %d", a, b)
	}
	if a == c {
		t.Fatalf("expected different jobs to have different keys, got %d", a)
	}
	if a < cronAdvisoryLockBase {
		t.Fatalf("expected key %d to stay within the cron key range (>= %d)", a, cronAdvisoryLockBase)
	}
}

// TestRunWithCronLockCrossInstance simulates two instances running the same
// cron job: only one acquires the advisory lock, so the body runs exactly once.
func TestRunWithCronLockCrossInstance(t *testing.T) {
	dataDir, err := os.MkdirTemp("", "pb_cronlock*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dataDir)

	app1 := newTestInstanceHeartbeatApp(t, dataDir)
	defer app1.ResetBootstrapState()

	app2 := newTestInstanceHeartbeatApp(t, dataDir)
	defer app2.ResetBootstrapState()

	const jobID = "test_job_lock"

	var callCount int32
	run := func(app *BaseApp) {
		err := app.runWithCronLock(jobID, app.Logger(), func() error {
			// sleep to widen the race window so both instances overlap
			time.Sleep(50 * time.Millisecond)
			atomic.AddInt32(&callCount, 1)
			return nil
		})
		if err != nil {
			t.Fatalf("runWithCronLock failed: %v", err)
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); run(app1) }()
	go func() { defer wg.Done(); run(app2) }()
	wg.Wait()

	if callCount != 1 {
		t.Fatalf("expected the cron body to run exactly once across two instances, got %d", callCount)
	}
}

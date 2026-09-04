package core_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/pocketbase/dbx"
)

// TestWriteClientDeadlineAbortsLockBlockedWrite is a DB-backed integration test
// for the L1 client-side write deadline (withWriteDeadline, wired into the write
// execute closures in core/db.go). It proves a write blocked on a held row lock
// aborts at ~QueryTimeout via the CLIENT deadline, decisively before the
// server-side lock_timeout (30s) / statement_timeout (60s) would ever fire.
//
// This is the write-path counterpart to the read-side queryTimeoutHook: on a
// silent connection drop the same deadline frees the pooled connection instead
// of letting it hang for minutes on the OS TCP retransmit.
func TestWriteClientDeadlineAbortsLockBlockedWrite(t *testing.T) {
	// QueryTimeout=1s is far below the role lock_timeout (30s), so if the client
	// deadline works the blocked Save must fail at ~1s regardless of whether the
	// harness applied the server-side role timeouts. DataMaxOpenConns=5 leaves
	// plenty of free connections (the lock holder uses only one), so the block is
	// on the ROW lock, not on pool acquisition.
	app, err := tests.NewTestAppWithConfig(core.BaseAppConfig{
		QueryTimeout:     1 * time.Second,
		DataMaxOpenConns: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	const (
		collectionName = "demo1"
		recordID       = "84nmscqy84lsi1t"
	)

	record, err := app.FindRecordById(collectionName, recordID)
	if err != nil {
		t.Fatal(err)
	}

	acquired := make(chan struct{})
	release := make(chan struct{})

	// rolls the lock-holding tx back once the test releases it
	errReleaseLock := errors.New("release row lock (force rollback)")

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Hold a row lock on the target record by issuing a no-op UPDATE inside an
		// open transaction, then block (uncommitted) until release is closed.
		_ = app.RunInTransaction(func(txApp core.App) error {
			if _, execErr := txApp.DB().NewQuery(
				"UPDATE {{" + collectionName + "}} SET [[text]] = [[text]] WHERE [[id]] = {:id}",
			).Bind(dbx.Params{"id": recordID}).Execute(); execErr != nil {
				return execErr
			}
			close(acquired) // row lock is now held by this open tx
			<-release       // keep the tx (and the lock) open until released
			return errReleaseLock
		})
	}()

	// wait until the lock holder actually owns the row lock
	select {
	case <-acquired:
	case <-time.After(15 * time.Second):
		close(release)
		wg.Wait()
		t.Fatal("timed out waiting for the lock-holding tx to acquire the row lock")
	}

	// The blocked write must abort at ~QueryTimeout (1s) — not at the 30s/60s
	// server-side timeouts — proving the CLIENT deadline fired.
	start := time.Now()
	saveErr := app.Save(record)
	elapsed := time.Since(start)

	// let the lock holder roll back and release the row lock
	close(release)
	wg.Wait()

	if saveErr == nil {
		t.Fatal("expected the lock-blocked Save to fail with a deadline error, got nil")
	}
	if !errors.Is(saveErr, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", saveErr)
	}
	// Generous upper bound (5s) that is still decisively below lock_timeout(30s)
	// and statement_timeout(60s): passing proves the client deadline won the race.
	if elapsed < 500*time.Millisecond || elapsed > 5*time.Second {
		t.Fatalf("expected Save to abort in ~1s (client deadline), got %s", elapsed)
	}
}

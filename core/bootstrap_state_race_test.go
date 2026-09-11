package core_test

import (
	"sync"
	"testing"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
)

// TestBootstrapStateConcurrentAccess is the regression test for the data race
// the CI -race job caught (2026-09-11): background goroutines — the notify
// dir watcher, the log/audit batch writers and the realtime outbox listener —
// read the bootstrap state (IsBootstrapped/DB/AuxDB/dataDB/dbConfig) without
// joining the owner goroutine, while ResetBootstrapState()/Bootstrap() nil-ed
// and reassigned the underlying fields unsynchronized, tainting the whole
// core test binary (8 cascading "race detected" failures from a single root
// cause). The fix serializes the fields with BaseApp.bootstrapMu; this test
// permanently guards it by churning the lifecycle while spinning readers.
func TestBootstrapStateConcurrentAccess(t *testing.T) {
	// no t.Parallel() — the churned app shares the test DB bootstrap state
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	const (
		cycles  = 3
		readers = 10
	)

	for i := 0; i < cycles; i++ {
		var wg sync.WaitGroup
		stop := make(chan struct{})

		// spin-readers mimicking the detached background goroutines
		for j := 0; j < readers; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					select {
					case <-stop:
						return
					default:
						_ = app.IsBootstrapped()
						_ = app.DB()
						_ = app.AuxDB()
					}
				}
			}()
		}

		// lifecycle churn from the owner goroutine (this one) — Bootstrap()
		// itself resets the previous state first
		if churnErr := app.Bootstrap(); churnErr != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("cycle %d Bootstrap: %v", i, churnErr)
		}
		if churnErr := app.ResetBootstrapState(); churnErr != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("cycle %d ResetBootstrapState: %v", i, churnErr)
		}

		close(stop)
		wg.Wait()
	}

	// leave the app in a bootstrapped state for the deferred Cleanup()
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if !app.IsBootstrapped() {
		t.Fatal("expected the app to be bootstrapped after the final Bootstrap()")
	}

	// keep the core.App interface assertion honest (the readers above go
	// through it in production code paths)
	var _ core.App = app
}

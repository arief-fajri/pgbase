package core_test

import (
	"strings"
	"testing"

	"github.com/arief-fajri/pgbase/tests"
	"github.com/arief-fajri/pgbase/tools/cron"
)

func findCronJob(app *tests.TestApp, id string) *cron.Job {
	for _, job := range app.Cron().Jobs() {
		if job.Id() == id {
			return job
		}
	}
	return nil
}

// TestCronVacuumOffByDefault verifies CRON-1: the daily whole-DB VACUUM cron
// is NOT registered by default (PostgreSQL autovacuum covers it).
func TestCronVacuumOffByDefault(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	if job := findCronJob(app, "__pbDBVacuum__"); job != nil {
		t.Fatalf("expected __pbDBVacuum__ to be OFF by default, got registered: %v", job)
	}
}

// TestCronVacuumEnabledByEnv verifies a PB_DB_VACUUM_CRON schedule registration
// when explicitly set.
func TestCronVacuumEnabledByEnv(t *testing.T) {
	t.Setenv("PB_DB_VACUUM_CRON", "0 3 * * *")

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	job := findCronJob(app, "__pbDBVacuum__")
	if job == nil {
		t.Fatal("expected __pbDBVacuum__ to be registered when PB_DB_VACUUM_CRON is set")
	}
	if !strings.Contains(job.Expression(), "3") || !strings.Contains(job.Expression(), "3 * * *") {
		t.Fatalf("expected the vacuum cron to honor the schedule, got %q", job.Expression())
	}
}

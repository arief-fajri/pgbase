package migrations_test

import (
	"testing"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/pocketbase/dbx"
)

// TestLogsAutovacuumTuning verifies that the _logs table carries the tuned
// per-table autovacuum reloptions (IDX-3 lighter alternative).
func TestLogsAutovacuumTuning(t *testing.T) {
	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	// read the table reloptions from the aux (logs) database
	var reloptions []string
	err := app.AuxDB().NewQuery(`SELECT COALESCE(reloptions, '{}') FROM pg_class WHERE relname = {:name}`).
		Bind(dbx.Params{"name": core.LogsTableName}).
		Column(&reloptions)
	if err != nil {
		t.Fatal(err)
	}

	joined := ""
	for _, r := range reloptions {
		joined += r + ";"
	}

	for _, want := range []string{
		"autovacuum_vacuum_scale_factor=0.02",
		"autovacuum_vacuum_threshold=10000",
		"autovacuum_analyze_scale_factor=0.02",
	} {
		if !contains(joined, want) {
			t.Fatalf("expected _logs reloptions to include %q, got %q", want, joined)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

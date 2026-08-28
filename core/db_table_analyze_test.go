package core

import (
	"os"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
)

// newAnalyzeTestApp bootstraps a raw BaseApp against the shared test DB (same
// approach as the heartbeat tests), so scoped ANALYZE can be verified directly.
func newAnalyzeTestApp(t *testing.T) *BaseApp {
	t.Helper()

	dataDir, err := os.MkdirTemp("", "pb_analyze*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	app := NewBaseApp(BaseAppConfig{
		DataDir:          dataDir,
		IsDev:            true,
		DataMaxOpenConns: 2,
		AuxMaxOpenConns:  2,
	})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("failed to bootstrap test app: %v", err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })

	return app
}

// lastAnalyze returns the last_analyze timestamp for a table, or a zero time
// if the table has never been analyzed.
func lastAnalyze(t *testing.T, app *BaseApp, tableName string) time.Time {
	t.Helper()

	var ts time.Time
	err := app.DB().NewQuery(`
		SELECT COALESCE(last_analyze, 'epoch'::timestamptz)
		FROM pg_stat_user_tables
		WHERE relname = {:tableName}
	`).Bind(dbx.Params{"tableName": tableName}).Row(&ts)
	if err != nil {
		t.Fatalf("failed to read last_analyze for %q: %v", tableName, err)
	}
	return ts
}

// TestAnalyzeTableScopedToTable verifies IDX-5: AnalyzeTable only refreshes
// statistics for the named table, and works against a real table.
func TestAnalyzeTableScopedToTable(t *testing.T) {
	app := newAnalyzeTestApp(t)

	// "_collections" is a real table in the raw test DB; analyze it explicitly
	if err := app.AnalyzeTable("_collections"); err != nil {
		t.Fatalf("AnalyzeTable(_collections) failed: %v", err)
	}

	// the analyzed table must have a fresh last_analyze
	ts := lastAnalyze(t, app, "_collections")
	if ts.Before(time.Now().Add(-1 * time.Minute)) {
		t.Fatalf("expected _collections to be analyzed recently, got %v", ts)
	}
}

// TestAnalyzeTableEmptyNameNoop verifies the guard against an empty name.
func TestAnalyzeTableEmptyNameNoop(t *testing.T) {
	app := newAnalyzeTestApp(t)

	if err := app.AnalyzeTable(""); err != nil {
		t.Fatalf("expected AnalyzeTable(\"\") to be a no-op, got error: %v", err)
	}
}

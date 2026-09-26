package core_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
)

// pgBackupDumpNameForTest mirrors the (unexported) core.pgBackupDumpName so the
// external tests can locate/produce the native dump inside a backup dir.
const pgBackupDumpNameForTest = "pgdata.dump"

// requireNativePGTools skips the calling test when the external pg_dump /
// pg_restore client binaries are not on PATH, so local runs without the
// PostgreSQL client package stay green (CI installs them; see release.yaml).
func requireNativePGTools(t *testing.T) {
	t.Helper()

	for _, bin := range []string{"pg_dump", "pg_restore"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("native PostgreSQL backup test skipped: %q not found in PATH (%v)", bin, err)
		}
	}
}

// TestPGDumpExportImportRoundTrip exercises the native PostgreSQL backup format
// end-to-end by calling ExportToPGDump / ImportFromPGDump directly. The real
// RestoreBackup finishes with an execve Restart that would replace the test
// process, so the test drives the same data path minus that restart.
//
// It also proves the dump is a FULL-database snapshot: besides the app records
// and settings, the seeded _logs rows (which live on the aux connection that
// shares the same physical database) must survive the
// export -> mutate -> import round trip.
func TestPGDumpExportImportRoundTrip(t *testing.T) {
	requireNativePGTools(t)

	ctx := context.Background()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	// -----------------------------------------------------------------
	// seed a known state to snapshot
	// -----------------------------------------------------------------
	demo1, err := app.FindCollectionByNameOrId("demo1")
	if err != nil {
		t.Fatalf("find demo1: %v", err)
	}

	baseDemo1 := countRows(t, app, "demo1")

	const markerText = "pgdump-roundtrip-marker"
	marker := core.NewRecord(demo1)
	marker.Set("text", markerText)
	if err := app.SaveNoValidate(marker); err != nil {
		t.Fatalf("create marker record: %v", err)
	}
	markerId := marker.Id

	if got := countRows(t, app, "demo1"); got != baseDemo1+1 {
		t.Fatalf("seed: expected demo1 count %d, got %d", baseDemo1+1, got)
	}

	// Snapshot a settings value (persisted to _params) BEFORE seeding _logs.
	//
	// Saving settings triggers OnSettingsReload, whose handler prunes logs
	// older than Logs.MaxDays. StubLogsData inserts rows dated 2022, so seeding
	// them AFTER this save keeps them intact through the export.
	const snapshotAppName = "pgdump_snapshot_app"
	app.Settings().Meta.AppName = snapshotAppName
	if err := app.Save(app.Settings()); err != nil {
		t.Fatalf("save snapshot settings: %v", err)
	}

	// seed two known rows in the aux _logs table (full-DB snapshot proof)
	if err := tests.StubLogsData(app); err != nil {
		t.Fatalf("StubLogsData: %v", err)
	}
	const wantLogs = 2
	if got := auxCountRows(t, app, "_logs"); got != wantLogs {
		t.Fatalf("seed: expected %d _logs rows, got %d", wantLogs, got)
	}

	// -----------------------------------------------------------------
	// export the native pg_dump snapshot
	// -----------------------------------------------------------------
	dumpDir := t.TempDir()
	dumpPath := filepath.Join(dumpDir, pgBackupDumpNameForTest)
	if err := app.ExportToPGDump(ctx, dumpPath); err != nil {
		t.Fatalf("ExportToPGDump: %v", err)
	}
	if fi, err := os.Stat(dumpPath); err != nil || fi.Size() == 0 {
		t.Fatalf("expected a non-empty dump at %q (size err=%v)", dumpPath, err)
	}

	// -----------------------------------------------------------------
	// mutate the live database *after* the snapshot
	// -----------------------------------------------------------------
	if err := app.Delete(marker); err != nil {
		t.Fatalf("delete marker record: %v", err)
	}
	if got := countRows(t, app, "demo1"); got != baseDemo1 {
		t.Fatalf("mutate: expected demo1 count %d, got %d", baseDemo1, got)
	}

	if _, err := app.AuxDB().NewQuery(`DELETE FROM {{_logs}}`).Execute(); err != nil {
		t.Fatalf("clear _logs: %v", err)
	}
	if got := auxCountRows(t, app, "_logs"); got != 0 {
		t.Fatalf("mutate: expected 0 _logs rows, got %d", got)
	}

	app.Settings().Meta.AppName = "pgdump_mutated_app"
	if err := app.Save(app.Settings()); err != nil {
		t.Fatalf("save mutated settings: %v", err)
	}

	// -----------------------------------------------------------------
	// import the snapshot back (native pg_restore --clean --if-exists)
	// -----------------------------------------------------------------
	if err := app.ImportFromPGDump(ctx, dumpDir); err != nil {
		t.Fatalf("ImportFromPGDump: %v", err)
	}

	// -----------------------------------------------------------------
	// assert the database is reset to the snapshot state
	// -----------------------------------------------------------------
	if got := countRows(t, app, "demo1"); got != baseDemo1+1 {
		t.Fatalf("restore: expected demo1 count %d, got %d", baseDemo1+1, got)
	}

	// the deleted marker record is back verbatim
	var gotText string
	if err := app.DB().NewQuery(`SELECT text FROM "demo1" WHERE id = '` + markerId + `'`).Row(&gotText); err != nil {
		t.Fatalf("read restored marker record: %v", err)
	}
	if gotText != markerText {
		t.Fatalf("restore: marker text mismatch: want %q, got %q", markerText, gotText)
	}

	// the seeded _logs rows are carried back -> full-DB snapshot (not just app
	// data). Asserted BEFORE ReloadSettings, whose handler would prune the
	// 2022-dated rows.
	if got := auxCountRows(t, app, "_logs"); got != wantLogs {
		t.Fatalf("restore: expected _logs restored to %d, got %d", wantLogs, got)
	}

	// settings restored (reload from _params since there was no app restart)
	if err := app.ReloadSettings(); err != nil {
		t.Fatalf("ReloadSettings: %v", err)
	}
	if got := app.Settings().Meta.AppName; got != snapshotAppName {
		t.Fatalf("restore: expected AppName %q, got %q", snapshotAppName, got)
	}
}

// TestImportFromPGDumpRecordsVerifiedTimestamp verifies the G-REL-04 signal:
// only a restore that passes the verification gate records the
// last_verified_backup timestamp — a rejected (corrupt) restore leaves both
// the _params row and the in-memory mirror untouched, a verified restore
// writes a parseable, recent RFC3339 timestamp.
func TestImportFromPGDumpRecordsVerifiedTimestamp(t *testing.T) {
	requireNativePGTools(t)

	ctx := context.Background()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	countLastVerified := func() int {
		var n int
		if err := app.DB().NewQuery(
			`SELECT count(*) FROM "_params" WHERE id = 'last_verified_backup'`,
		).Row(&n); err != nil {
			t.Fatalf("count last_verified_backup rows: %v", err)
		}
		return n
	}

	// fresh app: never verified — no row, mirror zero
	if got := app.DBEventStats().BackupLastVerified; got != 0 {
		t.Fatalf("fresh app must have no verified timestamp, got %d", got)
	}
	if n := countLastVerified(); n != 0 {
		t.Fatalf("fresh app must have no last_verified_backup row, got %d", n)
	}

	dumpDir := t.TempDir()
	dumpPath := filepath.Join(dumpDir, pgBackupDumpNameForTest)

	// a rejected restore must not record: corrupt the archive header
	if err := app.ExportToPGDump(ctx, dumpPath); err != nil {
		t.Fatalf("ExportToPGDump: %v", err)
	}
	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	for i := 0; i < 64 && i < len(data); i++ {
		data[i] = 0x00
	}
	if err := os.WriteFile(dumpPath, data, 0o644); err != nil {
		t.Fatalf("write corrupted dump: %v", err)
	}
	if err := app.ImportFromPGDump(ctx, dumpDir); err == nil {
		t.Fatal("expected the corrupt archive to be rejected")
	}
	if got := app.DBEventStats().BackupLastVerified; got != 0 {
		t.Fatalf("a rejected restore must not record a timestamp, got %d", got)
	}
	if n := countLastVerified(); n != 0 {
		t.Fatalf("a rejected restore must not write the _params row, got %d rows", n)
	}

	// a verified restore records: re-export a clean archive and import it.
	// The persisted format is second-precision RFC3339, so the window bounds
	// are compared at second granularity (a sub-second restoreStart would
	// false-fail against the truncated timestamp).
	restoreStart := time.Now().UTC().Truncate(time.Second)
	if err := app.ExportToPGDump(ctx, dumpPath); err != nil {
		t.Fatalf("ExportToPGDump (clean re-export): %v", err)
	}
	if err := app.ImportFromPGDump(ctx, dumpDir); err != nil {
		t.Fatalf("ImportFromPGDump: %v", err)
	}

	var raw string
	if err := app.DB().NewQuery(
		`SELECT "value" FROM "_params" WHERE "id" = 'last_verified_backup'`,
	).Row(&raw); err != nil {
		t.Fatalf("read last_verified_backup row: %v", err)
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		t.Fatalf("last_verified_backup is not RFC3339: %q (%v)", raw, err)
	}
	if ts.Before(restoreStart) || ts.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf(
			"last_verified_backup %v is not within the restore window [%v, %v]",
			ts, restoreStart, time.Now().UTC().Add(time.Second),
		)
	}
	if got := app.DBEventStats().BackupLastVerified; got != ts.Unix() {
		t.Fatalf("mirror = %d, want the persisted timestamp %d", got, ts.Unix())
	}
}

// auxCountRows returns the row count of a table on the aux connection (e.g. _logs).
func auxCountRows(t *testing.T, app core.App, table string) int {
	t.Helper()

	var n int
	if err := app.AuxDB().NewQuery(`SELECT count(*) FROM "` + table + `"`).Row(&n); err != nil {
		t.Fatalf("aux count %q: %v", table, err)
	}
	return n
}

// TestImportFromPGDumpRejectsCorruptArchive is a W-08 regression test: a
// corrupt archive must be rejected BEFORE the destructive pg_restore wipes
// the live database (pre-restore TOC validation), not "succeed" silently
// afterwards. The live data must still be answerable after the rejection.
func TestImportFromPGDumpRejectsCorruptArchive(t *testing.T) {
	requireNativePGTools(t)

	ctx := context.Background()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	// seed a marker so the survival of the live database is observable
	demo1, err := app.FindCollectionByNameOrId("demo1")
	if err != nil {
		t.Fatalf("find demo1: %v", err)
	}
	baseDemo1 := countRows(t, app, "demo1")

	marker := core.NewRecord(demo1)
	marker.Set("text", "corrupt-archive-marker")
	if err := app.SaveNoValidate(marker); err != nil {
		t.Fatalf("create marker record: %v", err)
	}
	markerId := marker.Id

	// a real dump of the live database, then corrupted in place (destroy the
	// custom-format header so pg_restore --list cannot even read the TOC)
	dumpDir := t.TempDir()
	dumpPath := filepath.Join(dumpDir, pgBackupDumpNameForTest)
	if err := app.ExportToPGDump(ctx, dumpPath); err != nil {
		t.Fatalf("ExportToPGDump: %v", err)
	}
	data, err := os.ReadFile(dumpPath)
	if err != nil {
		t.Fatalf("read dump: %v", err)
	}
	for i := 0; i < 64 && i < len(data); i++ {
		data[i] = 0x00
	}
	if err := os.WriteFile(dumpPath, data, 0o644); err != nil {
		t.Fatalf("write corrupted dump: %v", err)
	}

	// the corrupt archive must be rejected loudly
	importErr := app.ImportFromPGDump(ctx, dumpDir)
	if importErr == nil {
		t.Fatal("expected the corrupt archive to be rejected, got success")
	}

	// ... and rejected BEFORE the destructive restore: the live database is
	// still answerable with its data intact (the marker record survived)
	if got := countRows(t, app, "demo1"); got != baseDemo1+1 {
		t.Fatalf("live database was wiped by the rejected restore: demo1 count = %d, want %d", got, baseDemo1+1)
	}
	var gotText string
	if err := app.DB().NewQuery(`SELECT text FROM "demo1" WHERE id = '` + markerId + `'`).Row(&gotText); err != nil {
		t.Fatalf("read marker record after the rejected restore: %v", err)
	}
	if gotText != "corrupt-archive-marker" {
		t.Fatalf("marker text mismatch: got %q", gotText)
	}
}

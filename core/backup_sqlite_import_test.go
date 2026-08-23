package core_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/arief-fajri/pgbase/tools/archive"
)

const legacySQLiteFixture = "testdata/legacy_sqlite_backup/data.db"

// TestImportFromSQLiteDir performs an end-to-end import of an authentic
// PocketBase v0.23+ SQLite backup into the current PostgreSQL-backed test app
// and asserts schema, record fidelity (incl. verbatim auth secrets), settings
// and storage files are all restored.
func TestImportFromSQLiteDir(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	fixture, err := filepath.Abs(legacySQLiteFixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("missing fixture %q: %v", fixture, err)
	}

	// -----------------------------------------------------------------
	// read the expected values straight from the SQLite fixture so the
	// assertions self-verify the round-trip (no hardcoded secrets)
	// -----------------------------------------------------------------
	src, err := sql.Open("sqlite", "file:"+fixture+"?mode=ro")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}

	const knownUserId = "4q1xlclmfloku33"
	var wantEmail, wantPassword, wantToken string
	err = src.QueryRow(`SELECT email, password, tokenKey FROM users WHERE id = ?`, knownUserId).
		Scan(&wantEmail, &wantPassword, &wantToken)
	if err != nil {
		_ = src.Close()
		t.Fatalf("read known user from fixture: %v", err)
	}

	countCollections := []string{"users", "demo1", "demo2", "demo3", "demo4", "demo5", "clients", "nologin", "_superusers"}
	wantCounts := make(map[string]int, len(countCollections))
	for _, name := range countCollections {
		var n int
		if err := src.QueryRow(`SELECT count(*) FROM "` + name + `"`).Scan(&n); err != nil {
			_ = src.Close()
			t.Fatalf("count %q in fixture: %v", name, err)
		}
		wantCounts[name] = n
	}

	var wantSettings string
	if err := src.QueryRow(`SELECT value FROM _params WHERE id = 'settings'`).Scan(&wantSettings); err != nil {
		_ = src.Close()
		t.Fatalf("read settings from fixture: %v", err)
	}
	_ = src.Close()

	// -----------------------------------------------------------------
	// build the "extracted backup" dir: data.db + a storage file
	// -----------------------------------------------------------------
	extractedDir := t.TempDir()

	copyFile(t, fixture, filepath.Join(extractedDir, "data.db"))

	const storageKey = "testbucket/hello.txt"
	const storageContent = "hello-storage"
	storagePath := filepath.Join(extractedDir, "storage", filepath.FromSlash(storageKey))
	if err := os.MkdirAll(filepath.Dir(storagePath), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(storagePath, []byte(storageContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// -----------------------------------------------------------------
	// simulate a fresh restore target: drop all non-system collections so
	// the import recreates them from the backup verbatim. A real restore
	// runs against a freshly bootstrapped instance that only has the base
	// system collections, so importing over the fully-seeded test app would
	// otherwise surface artificial conflicts (e.g. a collection whose
	// "system" flag differs between the seed and the backup).
	// -----------------------------------------------------------------
	clearNonSystemCollections(t, app)

	// -----------------------------------------------------------------
	// import
	// -----------------------------------------------------------------
	if err := app.ImportFromSQLiteDir(context.Background(), extractedDir); err != nil {
		t.Fatalf("ImportFromSQLiteDir: %v", err)
	}

	// collections exist (non-view)
	for _, name := range []string{"users", "demo1", "demo2", "demo3", "demo4", "demo5", "clients", "nologin"} {
		if _, err := app.FindCollectionByNameOrId(name); err != nil {
			t.Fatalf("missing collection %q after import: %v", name, err)
		}
	}

	// record counts match the source verbatim
	for name, want := range wantCounts {
		var got int
		if err := app.DB().NewQuery(`SELECT count(*) FROM "` + name + `"`).Row(&got); err != nil {
			t.Fatalf("count %q after import: %v", name, err)
		}
		if got != want {
			t.Fatalf("collection %q: expected %d rows, got %d", name, want, got)
		}
	}

	// auth secrets preserved verbatim (read raw from the table to avoid
	// any model-level redaction/rehashing)
	var gotEmail, gotPassword, gotToken string
	err = app.DB().NewQuery(`SELECT email, password, "tokenKey" FROM users WHERE id = '`+knownUserId+`'`).
		Row(&gotEmail, &gotPassword, &gotToken)
	if err != nil {
		t.Fatalf("read imported user: %v", err)
	}
	if gotEmail != wantEmail {
		t.Fatalf("email mismatch: want %q, got %q", wantEmail, gotEmail)
	}
	if gotPassword != wantPassword {
		t.Fatalf("password hash not preserved verbatim: want %q, got %q", wantPassword, gotPassword)
	}
	if gotToken != wantToken {
		t.Fatalf("tokenKey not preserved verbatim: want %q, got %q", wantToken, gotToken)
	}

	// settings imported into _params (fixture value is plaintext -> verbatim)
	var gotSettings string
	if err := app.DB().NewQuery(`SELECT value FROM "_params" WHERE id = 'settings'`).Row(&gotSettings); err != nil {
		t.Fatalf("read imported settings: %v", err)
	}
	if gotSettings != wantSettings {
		t.Fatalf("settings not imported verbatim:\nwant %q\ngot  %q", wantSettings, gotSettings)
	}

	// storage file copied into the app filesystem
	fsys, err := app.NewFilesystem()
	if err != nil {
		t.Fatalf("NewFilesystem: %v", err)
	}
	defer fsys.Close()

	if ok, _ := fsys.Exists(storageKey); !ok {
		t.Fatalf("expected storage file %q to be imported", storageKey)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()

	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %q: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("write %q: %v", dst, err)
	}
}

// TestSQLiteImportBackupLifecycle walks the full user lifecycle against the
// real committed v0.23 SQLite fixture:
//
//	import a legacy SQLite backup -> create/update records + storage -> back up
//	the database -> create/update more records -> import the latest backup ->
//	the DB + storage are reset to the backup's state.
//
// Two hard architectural facts of this PostgreSQL fork shape the test:
//
//   - CreateBackup dumps the live PostgreSQL database into a portable,
//     v0.23-format SQLite "data.db" bundled inside the archive (alongside the
//     pb_data storage). A native backup is therefore a full record+storage
//     snapshot, not storage-only (asserted in step 3).
//
//   - RestoreBackup finishes by calling App.Restart(), which is an execve that
//     replaces the running process image and would kill the test binary. Steps
//     5-6 therefore drive the same data path RestoreBackup performs: swap the
//     extracted storage back into pb_data and re-run the SQLite import of the
//     backup's bundled data.db (the actual full DB+storage restore), stopping
//     short of the untestable Restart.
func TestSQLiteImportBackupLifecycle(t *testing.T) {
	ctx := context.Background()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	fixture, err := filepath.Abs(legacySQLiteFixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("missing fixture %q: %v", fixture, err)
	}

	// read the fixture's baseline users count so the assertions self-verify
	src, err := sql.Open("sqlite", "file:"+fixture+"?mode=ro")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	var fixtureUsers int
	if err := src.QueryRow(`SELECT count(*) FROM "users"`).Scan(&fixtureUsers); err != nil {
		_ = src.Close()
		t.Fatalf("count users in fixture: %v", err)
	}
	_ = src.Close()

	// -----------------------------------------------------------------
	// step 1 - import the legacy SQLite backup
	// -----------------------------------------------------------------
	extractedDir := t.TempDir()
	copyFile(t, fixture, filepath.Join(extractedDir, "data.db"))

	// simulate a fresh restore target so the import recreates the collections
	// from the backup verbatim (see clearNonSystemCollections)
	clearNonSystemCollections(t, app)

	if err := app.ImportFromSQLiteDir(ctx, extractedDir); err != nil {
		t.Fatalf("step 1 ImportFromSQLiteDir: %v", err)
	}

	if got := countRows(t, app, "users"); got != fixtureUsers {
		t.Fatalf("step 1: expected %d users after import, got %d", fixtureUsers, got)
	}
	baseDemo1 := countRows(t, app, "demo1")

	// -----------------------------------------------------------------
	// step 2 - create/update some records
	// -----------------------------------------------------------------
	demo1, err := app.FindCollectionByNameOrId("demo1")
	if err != nil {
		t.Fatalf("step 2 find demo1: %v", err)
	}

	rec1 := core.NewRecord(demo1)
	rec1.Set("text", "lifecycle-1")
	if err := app.SaveNoValidate(rec1); err != nil {
		t.Fatalf("step 2 create record: %v", err)
	}
	rec1.Set("text", "lifecycle-1-updated")
	if err := app.SaveNoValidate(rec1); err != nil {
		t.Fatalf("step 2 update record: %v", err)
	}

	if got := countRows(t, app, "demo1"); got != baseDemo1+1 {
		t.Fatalf("step 2: expected demo1 count %d, got %d", baseDemo1+1, got)
	}

	// add a storage file the upcoming backup should capture (written directly
	// under pb_data/storage, which is exactly what CreateBackup archives)
	const storageKey = "lifecycle/file.txt"
	storageDiskPath := filepath.Join(app.DataDir(), core.LocalStorageDirName, filepath.FromSlash(storageKey))
	writeFile(t, storageDiskPath, "v1")

	// -----------------------------------------------------------------
	// step 3 - back up the database
	//
	// NB: a native backup now bundles a portable, v0.23-format SQLite dump of
	// the live PostgreSQL database as "data.db", alongside the pb_data storage.
	// We assert both are present: the storage file AND the DB dump.
	// -----------------------------------------------------------------
	const backupName = "lifecycle_snapshot.zip"
	// opt into the portable SQLite dump so the assertions below (and this
	// hermetic test) don't require an external pg_dump client.
	app.Settings().Backups.Format = core.BackupFormatSQLite
	if err := app.CreateBackup(ctx, backupName); err != nil {
		t.Fatalf("step 3 CreateBackup: %v", err)
	}

	backupPath := filepath.Join(app.DataDir(), core.LocalBackupsDirName, backupName)
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("step 3: backup file not created: %v", err)
	}

	snapshotDir := t.TempDir()
	if err := archive.Extract(backupPath, snapshotDir); err != nil {
		t.Fatalf("step 3 extract backup: %v", err)
	}

	// the storage file is in the snapshot ...
	snapStorageFile := filepath.Join(snapshotDir, core.LocalStorageDirName, filepath.FromSlash(storageKey))
	if got := readFile(t, snapStorageFile); got != "v1" {
		t.Fatalf("step 3: expected storage %q=v1 in backup, got %q", storageKey, got)
	}
	// ... and the PostgreSQL data IS now dumped as data.db (Gap B fixed:
	// a native backup is a full record snapshot, not storage-only)
	snapDumpPath := filepath.Join(snapshotDir, sqliteBackupDBNameForTest)
	if _, err := os.Stat(snapDumpPath); err != nil {
		t.Fatalf("step 3: a native PG backup must bundle %q: %v", sqliteBackupDBNameForTest, err)
	}

	// the DB state captured by the backup (before the step-4 mutations)
	snapshotDemo1 := countRows(t, app, "demo1") // baseDemo1 + 1 (rec1)

	// -----------------------------------------------------------------
	// step 4 - create/update more records (post-snapshot mutations)
	// -----------------------------------------------------------------
	rec2 := core.NewRecord(demo1)
	rec2.Set("text", "lifecycle-2")
	if err := app.SaveNoValidate(rec2); err != nil {
		t.Fatalf("step 4 create record: %v", err)
	}
	afterStep4Demo1 := countRows(t, app, "demo1")
	if afterStep4Demo1 != baseDemo1+2 {
		t.Fatalf("step 4: expected demo1 count %d, got %d", baseDemo1+2, afterStep4Demo1)
	}

	// mutate the storage file *after* the snapshot was taken
	writeFile(t, storageDiskPath, "v2")

	// -----------------------------------------------------------------
	// step 5 + 6 - import & restore the latest backup
	//
	// RestoreBackup ends with App.Restart() (execve) and cannot run in-test, so
	// we drive the same data path it uses: swap the extracted snapshot's storage
	// back into pb_data, then import the backup's bundled data.db (the full DB
	// restore). This mirrors RestoreBackup minus the process restart.
	// -----------------------------------------------------------------
	writeFile(t, storageDiskPath, readFile(t, snapStorageFile))

	// storage is back to the snapshot value ...
	if got := readFile(t, storageDiskPath); got != "v1" {
		t.Fatalf("step 6: expected storage restored to v1, got %q", got)
	}

	// ... and importing the backup's own bundled data.db (the exported
	// PostgreSQL dump) resets the database to the backup's state, dropping the
	// records created in step 4. This proves the export -> import round-trip.
	if err := app.ImportFromSQLiteDir(ctx, snapshotDir); err != nil {
		t.Fatalf("step 6 ImportFromSQLiteDir (bundled dump): %v", err)
	}
	if got := countRows(t, app, "users"); got != fixtureUsers {
		t.Fatalf("step 6: expected users reset to %d, got %d", fixtureUsers, got)
	}
	if got := countRows(t, app, "demo1"); got != snapshotDemo1 {
		t.Fatalf("step 6: expected demo1 reset to the backup snapshot %d, got %d", snapshotDemo1, got)
	}
}

// sqliteBackupDBNameForTest mirrors the (unexported) core.sqliteBackupDBName so
// the external tests can assert a native backup bundles it.
const sqliteBackupDBNameForTest = "data.db"

// clearNonSystemCollections drops every non-system collection, retrying until
// none remain. The retry naturally handles view->view and view->base
// dependencies (PostgreSQL refuses to drop a collection that still has
// dependents, so it is simply retried on the next pass). This simulates a fresh
// restore target: a real restore runs against a freshly bootstrapped instance
// that only has the base system collections, so importing over a fully-seeded
// app would otherwise surface artificial conflicts (e.g. a collection whose
// "system" flag differs between the seed and the backup).
func clearNonSystemCollections(t *testing.T, app core.App) {
	t.Helper()

	colls, err := app.FindAllCollections()
	if err != nil {
		t.Fatalf("FindAllCollections: %v", err)
	}

	remaining := make([]*core.Collection, 0, len(colls))
	for _, c := range colls {
		if !c.System {
			remaining = append(remaining, c)
		}
	}

	for len(remaining) > 0 {
		var failed []*core.Collection
		var lastErr error
		for _, c := range remaining {
			if delErr := app.Delete(c); delErr != nil {
				lastErr = delErr
				failed = append(failed, c)
			}
		}
		if len(failed) == len(remaining) {
			t.Fatalf("failed to clear collections (no progress, %d remaining): %v", len(remaining), lastErr)
		}
		remaining = failed
	}
}

// countRows returns the row count of the given collection table.
func countRows(t *testing.T, app core.App, table string) int {
	t.Helper()

	var n int
	if err := app.DB().NewQuery(`SELECT count(*) FROM "` + table + `"`).Row(&n); err != nil {
		t.Fatalf("count %q: %v", table, err)
	}
	return n
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return string(b)
}

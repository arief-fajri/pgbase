package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pocketbase/dbx"
)

// TestRecordDBQueryError verifies the diagnostic classification: a client
// deadline or a server statement_timeout (57014) counts as a query timeout,
// a server lock_timeout (55P03) counts as a lock timeout, and everything
// else (e.g. unique violations, arbitrary errors, no error) counts nothing.
func TestRecordDBQueryError(t *testing.T) {
	scenarios := []struct {
		name           string
		err            error
		wantQueryDelta int64
		wantLockDelta  int64
	}{
		{"client deadline", context.DeadlineExceeded, 1, 0},
		{"wrapped client deadline", fmt.Errorf("save failed: %w", context.DeadlineExceeded), 1, 0},
		{
			"server statement_timeout (57014)",
			&pgconn.PgError{Code: "57014", Message: "canceling statement due to statement timeout"},
			1, 0,
		},
		{
			"server lock_timeout (55P03)",
			&pgconn.PgError{Code: "55P03", Message: "canceling statement due to lock timeout"},
			0, 1,
		},
		{"unique violation (23505) is not counted", &pgconn.PgError{Code: "23505"}, 0, 0},
		{"deadlock (40P01) is deliberately not counted", &pgconn.PgError{Code: "40P01"}, 0, 0},
		{"no error", nil, 0, 0},
		{"arbitrary error", errors.New("boom"), 0, 0},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			app := NewBaseApp(BaseAppConfig{}) // fresh counters

			app.recordDBQueryError(s.err, false)

			got := app.DBEventStats()
			if d := got.DataQueryTimeouts; d != s.wantQueryDelta {
				t.Fatalf("data query timeouts = %d, want %d", d, s.wantQueryDelta)
			}
			if d := got.DataLockTimeouts; d != s.wantLockDelta {
				t.Fatalf("data lock timeouts = %d, want %d", d, s.wantLockDelta)
			}
		})
	}

	// the aux label must be classified independently of data
	app := NewBaseApp(BaseAppConfig{})
	app.recordDBQueryError(context.DeadlineExceeded, true)
	if got := app.DBEventStats(); got.AuxQueryTimeouts != 1 || got.DataQueryTimeouts != 0 {
		t.Fatalf("expected the deadline counted on aux only, got %+v", got)
	}
}

// TestQueryTimeoutHookClassifiesDeadline verifies the record query hook wires
// the classification in: a deadline-exceeded execution error must land in the
// data query-timeout counter (and ErrNoRows must not).
func TestQueryTimeoutHookClassifiesDeadline(t *testing.T) {
	app := NewBaseApp(BaseAppConfig{})

	fakeDB := dbx.NewFromDB(&sql.DB{}, "postgres")

	hook := queryTimeoutHook(app, time.Second)

	err := hook(fakeDB.NewQuery("SELECT 1"), func() error { return context.DeadlineExceeded })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected the hook to wrap the deadline error, got: %v", err)
	}
	if got := app.DBEventStats().DataQueryTimeouts; got != 1 {
		t.Fatalf("expected 1 data query timeout, got %d", got)
	}

	// sql.ErrNoRows is a non-error outcome and must never be counted
	err = hook(fakeDB.NewQuery("SELECT 1"), func() error { return sql.ErrNoRows })
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows to pass through, got: %v", err)
	}
	if got := app.DBEventStats().DataQueryTimeouts; got != 1 {
		t.Fatalf("ErrNoRows must not be counted, got %d query timeouts", got)
	}
}

// newDBEventsTestApp boots a dedicated-database app for the diagnostic
// counter integration tests (coldboot pattern): every mutation happens inside
// the dedicated database, dropped at cleanup.
func newDBEventsTestApp(t *testing.T) *BaseApp {
	t.Helper()

	app, _ := newDBEventsTestAppDB(t)
	return app
}

// newDBEventsTestAppDB is newDBEventsTestApp but also returns the dedicated
// database name, so tests can boot a SECOND app on the same database (e.g.
// the bootstrap-load test below).
func newDBEventsTestAppDB(t *testing.T) (*BaseApp, string) {
	t.Helper()

	dbName := fmt.Sprintf("pb_dbevents_%d_%d", os.Getpid(), time.Now().UnixNano())

	maint, err := DefaultDBConnect(ResolveDBConfig(DBConfig{
		SSLMode:      "disable",
		MaxOpenConns: 2,
		MaxIdleConns: 1,
	}))
	if err != nil {
		t.Fatalf("maintenance connect: %v", err)
	}
	t.Cleanup(func() { maint.Close() })

	if _, err := maint.NewQuery("DROP DATABASE IF EXISTS " + dbName + " WITH (FORCE)").Execute(); err != nil {
		t.Fatalf("failed to drop leftover db: %v", err)
	}
	if _, err := maint.NewQuery("CREATE DATABASE " + dbName).Execute(); err != nil {
		t.Fatalf("failed to create database %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		maint.NewQuery(
			"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='" + dbName + "' AND pid <> pg_backend_pid()",
		).Execute()
		maint.NewQuery("DROP DATABASE IF EXISTS " + dbName + " WITH (FORCE)").Execute()
	})

	dataDir, err := os.MkdirTemp("", "pb_dbevents_data_")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	app := NewBaseApp(BaseAppConfig{
		DataDir:       dataDir,
		EncryptionEnv: "pb_dbevents_test_env",
		DBConnect: func(c DBConfig) (*dbx.DB, error) {
			c.DBName = dbName
			return DefaultDBConnect(c)
		},
	})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })

	return app, dbName
}

// TestTxRollbackCounters verifies the G-REL-01 rollback counter: a failed
// top-level transaction counts exactly once (data or aux by target), and
// nested transactions inside an existing transaction do not double-count.
func TestTxRollbackCounters(t *testing.T) {
	app := newDBEventsTestApp(t)

	// data rollback
	if err := app.RunInTransaction(func(txApp App) error { return errors.New("data boom") }); err == nil {
		t.Fatal("expected the data transaction to fail")
	}
	got := app.DBEventStats()
	if got.DataTxRollbacks != 1 || got.AuxTxRollbacks != 0 {
		t.Fatalf("expected 1 data rollback, got data=%d aux=%d", got.DataTxRollbacks, got.AuxTxRollbacks)
	}

	// a nested transaction inside a failed outer transaction counts once in
	// total (the nested call reuses the outer transaction)
	outerErr := app.RunInTransaction(func(txApp App) error {
		return txApp.RunInTransaction(func(txApp2 App) error { return errors.New("nested boom") })
	})
	if outerErr == nil {
		t.Fatal("expected the outer transaction to fail")
	}
	got = app.DBEventStats()
	if got.DataTxRollbacks != 2 {
		t.Fatalf("nested failure must count exactly one rollback (total 2), got %d", got.DataTxRollbacks)
	}

	// aux rollback
	if err := app.AuxRunInTransaction(func(txApp App) error { return errors.New("aux boom") }); err == nil {
		t.Fatal("expected the aux transaction to fail")
	}
	got = app.DBEventStats()
	if got.AuxTxRollbacks != 1 || got.DataTxRollbacks != 2 {
		t.Fatalf("expected 1 aux rollback, got data=%d aux=%d", got.DataTxRollbacks, got.AuxTxRollbacks)
	}
}

// TestBackupCounters verifies the backup lifecycle counters: attempts count
// only backups that actually started (the active-backup guard does not
// count), successes and failures are counted per completion, duration and the
// last successful size are recorded.
func TestBackupCounters(t *testing.T) {
	app := newDBEventsTestApp(t)

	// exercise the portable SQLite dump format so this test stays hermetic
	// (no external pg_dump client needed; same pattern as TestCreateBackup)
	app.Settings().Backups.Format = BackupFormatSQLite

	// the active-backup guard rejects before the backup starts: no attempt
	app.Store().Set(StoreKeyActiveBackup, "")
	if err := app.CreateBackup(context.Background(), "guard.zip"); err == nil {
		t.Fatal("expected the pending-backup guard error")
	}
	if got := app.DBEventStats(); got.BackupAttempts != 0 {
		t.Fatalf("a rejected backup must not count as an attempt, got %d", got.BackupAttempts)
	}
	app.Store().Remove(StoreKeyActiveBackup)

	// two successful backups
	for _, name := range []string{"one.zip", "two.zip"} {
		if err := app.CreateBackup(context.Background(), name); err != nil {
			t.Fatalf("CreateBackup(%q): %v", name, err)
		}
	}
	got := app.DBEventStats()
	if got.BackupAttempts != 2 || got.BackupSuccesses != 2 || got.BackupFailures != 0 {
		t.Fatalf("expected 2 attempts / 2 successes / 0 failures, got %+v", got)
	}
	if got.BackupLastSize <= 0 {
		t.Fatalf("expected a positive last backup size, got %d", got.BackupLastSize)
	}
	if got.BackupDuration <= 0 {
		t.Fatalf("expected a positive backup duration, got %s", got.BackupDuration)
	}

	// a failure after the attempt counts: block the backup temp dir with a
	// regular file so the in-trigger MkdirAll fails deterministically
	tempDirBlock := filepath.Join(app.DataDir(), LocalTempDirName)
	if err := os.Remove(tempDirBlock); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove the temp dir: %v", err)
	}
	if err := os.WriteFile(tempDirBlock, []byte("blocked"), 0o644); err != nil {
		t.Fatalf("failed to block the temp dir: %v", err)
	}
	if err := app.CreateBackup(context.Background(), "fails.zip"); err == nil {
		t.Fatal("expected the backup to fail on the blocked temp dir")
	}

	got = app.DBEventStats()
	if got.BackupAttempts != 3 || got.BackupFailures != 1 || got.BackupSuccesses != 2 {
		t.Fatalf("expected 3 attempts / 2 successes / 1 failure, got %+v", got)
	}
}

// newDBEventsPeerApp boots a SECOND app on an existing dedicated database
// (fresh pb_data) — the restore-path reality: the old process is gone, a new
// boot must load the persisted verified-restore timestamp.
func newDBEventsPeerApp(t *testing.T, dbName string) *BaseApp {
	t.Helper()

	dataDir, err := os.MkdirTemp("", "pb_dbevents_peer_")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	peer := NewBaseApp(BaseAppConfig{
		DataDir:       dataDir,
		EncryptionEnv: "pb_dbevents_test_env",
		DBConnect: func(c DBConfig) (*dbx.DB, error) {
			c.DBName = dbName
			return DefaultDBConnect(c)
		},
	})
	if err := peer.Bootstrap(); err != nil {
		t.Fatalf("peer Bootstrap: %v", err)
	}
	t.Cleanup(func() { peer.ResetBootstrapState() })

	return peer
}

// TestBootstrapLoadsLastVerifiedBackup verifies the G-REL-04 mirror survives
// restarts: a new boot on a database with a last_verified_backup row loads it
// into the in-memory mirror (exposed as
// pgbase_backup_last_verified_timestamp_seconds), an absent row stays 0, and
// an unparseable value degrades to 0 instead of failing the boot.
func TestBootstrapLoadsLastVerifiedBackup(t *testing.T) {
	app, dbName := newDBEventsTestAppDB(t)

	// fresh boot: never verified
	if got := app.DBEventStats().BackupLastVerified; got != 0 {
		t.Fatalf("fresh app must have no verified timestamp, got %d", got)
	}

	// persist a known timestamp exactly as the restore path does
	ts := time.Date(2026, 9, 26, 1, 30, 0, 0, time.UTC)
	if _, err := app.NonconcurrentDB().NewQuery(
		`INSERT INTO "_params" ("id", "value") VALUES ({:id}, {:value})
		 ON CONFLICT ("id") DO UPDATE SET "value" = {:value}`,
	).
		Bind(dbx.Params{"id": lastVerifiedBackupParamsKey, "value": ts.Format(time.RFC3339)}).
		Execute(); err != nil {
		t.Fatalf("seed last_verified_backup row: %v", err)
	}

	// a new boot on the same database loads the row into its mirror
	if got := newDBEventsPeerApp(t, dbName).DBEventStats().BackupLastVerified; got != ts.Unix() {
		t.Fatalf("peer boot mirror = %d, want %d", got, ts.Unix())
	}

	// an unparseable value degrades to "never verified", never fails the boot
	if _, err := app.NonconcurrentDB().NewQuery(
		`UPDATE "_params" SET "value" = 'not-a-timestamp' WHERE "id" = {:id}`,
	).
		Bind(dbx.Params{"id": lastVerifiedBackupParamsKey}).
		Execute(); err != nil {
		t.Fatalf("corrupt last_verified_backup row: %v", err)
	}
	if got := newDBEventsPeerApp(t, dbName).DBEventStats().BackupLastVerified; got != 0 {
		t.Fatalf("unparseable value must degrade to 0, got %d", got)
	}
}

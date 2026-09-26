package core

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pocketbase/dbx"
)

// newTimeoutTestApp boots a dedicated-database app for the reliability timeout
// tests (same coldboot pattern as newDBEventsTestAppDB): an empty database is
// created and dropped per test, and Bootstrap runs the migrations itself.
//
// The provided config is used as-is except for DataDir/EncryptionEnv/DBConnect,
// which the helper fills with the dedicated-DB wiring.
func newTimeoutTestApp(t *testing.T, config BaseAppConfig) (*BaseApp, string) {
	t.Helper()

	dbName := fmt.Sprintf("pb_timeout_%d_%d", os.Getpid(), time.Now().UnixNano())

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

	config.DataDir = t.TempDir()
	config.EncryptionEnv = "pb_timeout_test_env"
	config.DBConnect = func(c DBConfig) (*dbx.DB, error) {
		c.DBName = dbName
		return DefaultDBConnect(c)
	}

	app := NewBaseApp(config)
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })

	return app, dbName
}

// isQueryTimeoutError reports whether err is the expected shape of a bounded
// query cancellation: a client-side deadline or a server-side statement_timeout
// (SQLSTATE 57014).
func isQueryTimeoutError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "57014"
}

// TestQueryTimeoutBoundsSlowDataQuery verifies §2 "Query timeout works" (Phase
// 0 reliability tests): a slow data query on the RecordQuery path is cancelled
// client-side by QueryTimeout within its bound, and the cancellation lands in
// the observable G-REL-01 diagnostic counter.
//
// The sleep is injected in the WHERE clause against _migrations, which every
// bootstrapped database has and which always has rows (the applied-migration
// history), so pg_sleep is evaluated on every execution.
func TestQueryTimeoutBoundsSlowDataQuery(t *testing.T) {
	app, _ := newTimeoutTestApp(t, BaseAppConfig{
		QueryTimeout: 500 * time.Millisecond,
	})

	start := time.Now()
	var v any
	err := app.RecordQuery(NewBaseCollection("_migrations")).
		AndWhere(dbx.NewExp("pg_sleep(2) IS NULL")).
		Row(&v)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected the slow query to fail with a timeout, got nil")
	}
	if !isQueryTimeoutError(err) {
		t.Fatalf("expected a client deadline or 57014, got: %v", err)
	}
	// the query must have waited ~QueryTimeout (proving it was the timeout and
	// not an immediate error such as a missing relation) and must return well
	// before the 2s sleep finishes
	if elapsed < 400*time.Millisecond {
		t.Fatalf("expected the query to be bounded by QueryTimeout (500ms), returned after %v", elapsed)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("expected the query to return within a few seconds, took %v", elapsed)
	}
	if got := app.DBEventStats().DataQueryTimeouts; got != 1 {
		t.Fatalf("expected 1 data query timeout counter, got %d", got)
	}
}

// TestLockTimeoutBoundsBlockedRowLock verifies §2 "Lock timeout works" (lock
// wait bounded server-side by lock_timeout): a row lock held by another
// connection blocks the app until SET LOCAL lock_timeout fires, then fails
// with SQLSTATE 55P03 within the bound — observable as a transaction rollback
// (the 55P03 -> DataLockTimeouts classification itself is unit-tested in
// TestRecordDBQueryError).
func TestLockTimeoutBoundsBlockedRowLock(t *testing.T) {
	app, dbName := newTimeoutTestApp(t, BaseAppConfig{})

	// second connection into the SAME database to hold a conflicting lock
	harness, err := DefaultDBConnect(ResolveDBConfig(DBConfig{
		DBName:       dbName,
		SSLMode:      "disable",
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	}))
	if err != nil {
		t.Fatalf("harness connect: %v", err)
	}
	defer harness.Close()

	harnessTx, err := harness.DB().Begin()
	if err != nil {
		t.Fatalf("harness begin: %v", err)
	}
	defer harnessTx.Rollback()

	if _, err := harnessTx.Exec(`LOCK TABLE "_migrations" IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("harness lock: %v", err)
	}

	start := time.Now()
	txErr := app.RunInTransaction(func(txApp App) error {
		if _, err := txApp.NonconcurrentDB().NewQuery("SET LOCAL lock_timeout = '1s'").Execute(); err != nil {
			return err
		}
		_, err := txApp.NonconcurrentDB().NewQuery(`SELECT 1 FROM "_migrations" LIMIT 1 FOR UPDATE`).Execute()
		return err
	})
	elapsed := time.Since(start)

	if txErr == nil {
		t.Fatal("expected the blocked transaction to fail with a lock timeout, got nil")
	}
	if !strings.Contains(txErr.Error(), "55P03") {
		t.Fatalf("expected SQLSTATE 55P03 in the error, got: %v", txErr)
	}
	// bounded by SET LOCAL lock_timeout = '1s', not by the harness holding the
	// lock forever
	if elapsed < 900*time.Millisecond {
		t.Fatalf("expected the lock wait to be bounded by lock_timeout (1s), failed after %v", elapsed)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("expected the lock wait to end within a few seconds, took %v", elapsed)
	}
	// the failed transaction must be observable as a rollback (G-REL-01)
	if got := app.DBEventStats().DataTxRollbacks; got != 1 {
		t.Fatalf("expected 1 data transaction rollback counter, got %d", got)
	}
}

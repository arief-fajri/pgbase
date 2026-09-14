package core_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/arief-fajri/pgbase/core"
	"github.com/pocketbase/dbx"
)

// TestColdBootMigrationsSingleConnection is the W-10 regression test.
//
// It runs Bootstrap() + RunAllMigrations() against a TRULY EMPTY database (no
// template pre-migrated schema, no pre-installed pgcrypto), i.e. the exact
// conditions of a fresh install / quickstart / first "serve" boot.
//
// Before the fix the migrations runner ran the auxiliary transaction (advisory
// lock + aux DDL like CREATE TABLE IF NOT EXISTS _logs) and the data
// transaction (data DDL like CREATE EXTENSION pgcrypto) on two separate pool
// connections. On a cold database the two streams interleaved catalog DDL on a
// single goroutine and deadlocked each other until the role-level lock_timeout
// aborted the whole boot (SQLSTATE 55P03). All migrations must therefore be
// applied on a single connection (see core/migrations_runner.go runMigrationTx
// and docs/FAILURE-MODES.md W-10).
func TestColdBootMigrationsSingleConnection(t *testing.T) {
	// Dedicated database, so this test can safely run in parallel with the
	// rest of the suite (dropOrphanDatabases only ever cleans the harness's
	// own pb_test_*/pb_template_* databases).
	dbName := fmt.Sprintf("pb_coldboot_%d_%d", os.Getpid(), time.Now().UnixNano())

	// maintenance connection (connects to the shared test database)
	cfg := core.ResolveDBConfig(core.DBConfig{SSLMode: "disable", MaxOpenConns: 2, MaxIdleConns: 1})
	maint, err := core.DefaultDBConnect(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer maint.Close()

	// create a fresh empty database
	if _, err := maint.NewQuery("DROP DATABASE IF EXISTS " + dbName + " WITH (FORCE)").Execute(); err != nil {
		t.Fatalf("failed to drop leftover db: %v", err)
	}
	if _, err := maint.NewQuery("CREATE DATABASE " + dbName).Execute(); err != nil {
		t.Fatalf("failed to create database %s: %v", dbName, err)
	}
	defer func() {
		maint.NewQuery("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='" + dbName + "' AND pid <> pg_backend_pid()").Execute()
		maint.NewQuery("DROP DATABASE IF EXISTS " + dbName + " WITH (FORCE)").Execute()
	}()

	dataDir, err := os.MkdirTemp("", "pb_coldboot_data_")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	app := core.NewBaseApp(core.BaseAppConfig{
		DataDir:       dataDir,
		EncryptionEnv: "pb_coldboot_test_env",
		DBConnect: func(c core.DBConfig) (*dbx.DB, error) {
			// explicit DBName; keep caller-provided fields otherwise
			c.DBName = dbName
			return core.DefaultDBConnect(c)
		},
	})
	defer app.ResetBootstrapState()

	// NB! The point of the test is the DEFAULT pool layout (data 80 / aux 10).
	// Bootstrap() triggers RunSystemMigrations (pgcrypto + _params +
	// _collections on the data side, pgcrypto + _logs on the aux side) — with
	// the pre-fix two-connection runner this deadlocked on a cold database.
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap on empty db failed (W-10 regression): %v", err)
	}

	// complete the bundle of the "serve" cold-start path: apply the full
	// (system + app) migration list on top of the freshly bootstrapped db
	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations on cold boot failed: %v", err)
	}

	// assert the expected catalog objects actually exist
	verifyTableExists(t, app.DB(), "_collections")
	verifyTableExists(t, app.AuxDB(), "_logs")
	verifyTableExists(t, app.DB(), core.DefaultMigrationsTable)

	var creates bool
	if err := app.DB().NewQuery(
		`SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname='pgcrypto')`,
	).Row(&creates); err != nil || !creates {
		t.Fatalf("pgcrypto was not installed after cold boot migrations (err=%v, exists=%v)", err, creates)
	}

	// sanity: nothing is left pending (the runner must have recorded all migrations)
	var applied int
	if err := app.DB().NewQuery(
		`SELECT count(*) FROM ` + core.DefaultMigrationsTable,
	).Row(&applied); err != nil {
		t.Fatal(err)
	}
	if applied < len(core.SystemMigrations.Items())+len(core.AppMigrations.Items())-1 {
		t.Fatalf("expected all migrations to be recorded, got %d", applied)
	}
}

func verifyTableExists(t *testing.T, db dbx.Builder, table string) {
	t.Helper()

	var exists bool
	if err := db.NewQuery(
		`SELECT EXISTS (SELECT 1 FROM pg_tables WHERE tablename='` + table + `')`,
	).Row(&exists); err != nil {
		t.Fatalf("failed to check table %s: %v", table, err)
	}
	if !exists {
		t.Fatalf("expected table %s to exist after cold boot migrations", table)
	}
}

package core

import (
	"os"
	"testing"
)

// TestMain defaults the PB_POSTGRES_* connection env vars to the local test
// database (tests/docker-compose.test.yml, port 5433) when they are not already
// set.
//
// A few tests in this package build an app via NewBaseApp without a custom
// DBConnect (base_test.go, log_printer_test.go, system_alert_test.go,
// notify_watcher_test.go). Those fall back to DefaultDBConnect, which reads
// PB_POSTGRES_* and otherwise defaults to the *production* connection
// (localhost:5432, user "pgbase"). Without this, `go test ./core/...` fails with
// SQLSTATE 28P01 unless the caller first sources a test env (e.g. /tmp/pgtest.env).
//
// Only unset vars are defaulted, so an explicit environment (CI, a sourced env
// file) always wins. Harness-based tests are unaffected because NewTestApp fills
// the DBConfig explicitly from PGTEST_*.
func TestMain(m *testing.M) {
	setDefaultEnv("PB_POSTGRES_HOST", "localhost")
	setDefaultEnv("PB_POSTGRES_PORT", "5433")
	setDefaultEnv("PB_POSTGRES_USER", "test")
	setDefaultEnv("PB_POSTGRES_PASSWORD", "test")
	setDefaultEnv("PB_POSTGRES_DBNAME", "pgbase_test")

	os.Exit(m.Run())
}

func setDefaultEnv(key, value string) {
	if os.Getenv(key) == "" {
		os.Setenv(key, value)
	}
}

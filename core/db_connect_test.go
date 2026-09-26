package core_test

import (
	"database/sql"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/pocketbase/dbx"
)

func TestPostgresRoleTimeouts(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	rows := []struct {
		RolConfig sql.NullString `db:"rolconfig"`
	}{}
	err := app.DB().
		NewQuery(`SELECT rolconfig::text AS rolconfig FROM pg_roles WHERE rolname = current_user`).
		All(&rows)
	if err != nil {
		t.Fatal(err)
	}

	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 role row, got %d", len(rows))
	}

	config := rows[0].RolConfig.String

	if !strings.Contains(config, "statement_timeout=60s") {
		t.Fatalf("expected statement_timeout=60s role default, got %q", config)
	}
	if !strings.Contains(config, "lock_timeout=30s") {
		t.Fatalf("expected lock_timeout=30s role default, got %q", config)
	}
}

// failFastDBConnect forces every connection attempt to a specific address,
// bypassing the PB_POSTGRES_* env resolution so a test app can never leak onto
// the shared test database.
func failFastDBConnect(host string, port int) core.DBConnectFunc {
	return func(c core.DBConfig) (*dbx.DB, error) {
		c.Host = host
		c.Port = port
		c.User = "nonexistent"
		c.Password = "nonexistent"
		c.DBName = "nonexistent"
		c.SSLMode = "disable"
		return core.DefaultDBConnect(c)
	}
}

// TestBootstrapConnectRefusedFailsFast verifies §2 "Connection acquisition
// timeout works" (refused case): dialing a closed port fails Bootstrap
// immediately with an explicit, diagnosable error instead of hanging on the
// OS connect timeout.
func TestBootstrapConnectRefusedFailsFast(t *testing.T) {
	// reserve a port, then close it so the address is guaranteed refused
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}

	app := core.NewBaseApp(core.BaseAppConfig{
		DataDir:       t.TempDir(),
		EncryptionEnv: "pb_connrefused_env",
		DBConnect:     failFastDBConnect("127.0.0.1", closedPort),
	})

	start := time.Now()
	bootErr := app.Bootstrap()
	elapsed := time.Since(start)
	t.Cleanup(func() { app.ResetBootstrapState() })

	if bootErr == nil {
		t.Fatal("expected Bootstrap to fail against a refused address")
	}
	if !strings.Contains(bootErr.Error(), "refused") {
		t.Fatalf("expected a diagnosable connection refused error, got: %v", bootErr)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("expected an immediate refusal, Bootstrap took %v", elapsed)
	}
}

// TestBootstrapConnectTimeoutBounded verifies §2 "Connection acquisition
// timeout works" (blackhole case): dialing an unroutable address (RFC 5737
// TEST-NET-1) is bounded by the libpq connect_timeout DSN parameter instead of
// the OS default (~75s+), so a Bootstrap during an outage fails fast and
// explicitly (fail-fast guarantee documented in core/db_connect.go).
func TestBootstrapConnectTimeoutBounded(t *testing.T) {
	// seconds (libpq semantics); read by buildDSN at connect time
	t.Setenv("PB_POSTGRES_CONNECT_TIMEOUT", "1")

	app := core.NewBaseApp(core.BaseAppConfig{
		DataDir:       t.TempDir(),
		EncryptionEnv: "pb_conntimeout_env",
		DBConnect:     failFastDBConnect("192.0.2.1", 5432), // TEST-NET-1, non-routable
	})

	start := time.Now()
	bootErr := app.Bootstrap()
	elapsed := time.Since(start)
	t.Cleanup(func() { app.ResetBootstrapState() })

	if bootErr == nil {
		t.Fatal("expected Bootstrap to fail against a blackhole address")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("expected the dial to be bounded by connect_timeout=1s, Bootstrap took %v", elapsed)
	}
}

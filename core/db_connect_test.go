package core_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/arief-fajri/pgbase/tests"
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

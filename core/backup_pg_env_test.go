package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPGRestoreListTablesPassesConnectionEnv is the W-14 regression test: the
// pre-restore TOC gate must receive the same libpq environment as the export
// and the destructive restore.
//
// On Debian/Ubuntu hosts pg_dump/pg_restore on PATH are pg_wrapper symlinks
// whose client-VERSION choice depends on the connection environment and local
// cluster state (pg_wrapper(1): PGHOST set → the default/newest client is
// used; no env → a cluster is selected by port/only-cluster rules). With
// mixed client majors installed, an env-less `--list` can read the archive
// TOC through a different client major than the one that wrote the archive —
// observed in CI pg-matrix (17) as pg_restore 16.15 rejecting the 17.11
// archive header ("unsupported version (1.16) in file header").
//
// The shim records the environment it was invoked with and emits a minimal
// valid TOC, so the assertion needs no PostgreSQL, no pg_wrapper and no
// client binaries.
func TestPGRestoreListTablesPassesConnectionEnv(t *testing.T) {
	dir := t.TempDir()

	envFile := filepath.Join(dir, "invocation.env")
	shim := filepath.Join(dir, "pg_restore")

	script := fmt.Sprintf(`#!/bin/sh
env | grep '^PG' | sort > %q
printf '232; 1259 17656 TABLE public demo1 test\n'
`, envFile)
	if err := os.WriteFile(shim, []byte(script), 0o755); err != nil {
		t.Fatalf("write shim: %v", err)
	}

	dumpPath := filepath.Join(dir, "pgdata.dump")
	if err := os.WriteFile(dumpPath, []byte("placeholder"), 0o644); err != nil {
		t.Fatalf("write dump placeholder: %v", err)
	}

	conn := pgConn{
		Host:     "db.internal",
		Port:     6543,
		User:     "pgbase",
		Password: "s3cret",
		DBName:   "appdb",
		SSLMode:  "require",
	}

	toc, err := pgRestoreListTables(context.Background(), shim, dumpPath, conn.envList())
	if err != nil {
		t.Fatalf("pgRestoreListTables: %v", err)
	}

	recorded, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("the shim invocation recorded no environment (was pgRestoreListTables invoked?): %v", err)
	}
	for _, want := range []string{
		"PGHOST=db.internal",
		"PGPORT=6543",
		"PGUSER=pgbase",
		"PGPASSWORD=s3cret",
		"PGDATABASE=appdb",
		"PGSSLMODE=require",
	} {
		if !strings.Contains(string(recorded), want+"\n") {
			t.Errorf("TOC gate invocation environment is missing %s:\n%s", want, recorded)
		}
	}

	if len(toc.Tables) != 1 || toc.Tables[0].Schema != "public" || toc.Tables[0].Name != "demo1" {
		t.Fatalf("expected the shim TOC table public.demo1, got %+v", toc.Tables)
	}
}

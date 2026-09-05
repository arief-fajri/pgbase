package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// pgBackupDumpName is the pg_dump (custom-format) archive bundled inside a
// native PostgreSQL backup. Its presence in an extracted backup selects the
// native pg_restore path (see RestoreBackup / ImportFromPGDump).
const pgBackupDumpName = "pgdata.dump"

// pgConn holds the effective connection parameters used to invoke the external
// pg_dump / pg_restore binaries.
type pgConn struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
	Schema   string
}

// envList renders the connection as libpq environment variables consumed by
// pg_dump / pg_restore (which keeps the password off the command line).
func (c pgConn) envList() []string {
	env := []string{
		"PGHOST=" + c.Host,
		"PGPORT=" + strconv.Itoa(c.Port),
		"PGUSER=" + c.User,
		"PGPASSWORD=" + c.Password,
		"PGDATABASE=" + c.DBName,
	}
	if c.SSLMode != "" {
		env = append(env, "PGSSLMODE="+c.SSLMode)
	}
	// restrict object resolution to the app schema when a non-default one is in use
	if c.Schema != "" && c.Schema != "public" {
		env = append(env, "PGOPTIONS=-c search_path="+c.Schema+",public")
	}
	return env
}

// pgConnInfo resolves the effective PostgreSQL connection for the running app.
//
// host/port/user/password/sslmode come from the shared env resolver
// (ResolveDBConfig); the database name and schema are read from the live
// connection via current_database()/current_schema() because a custom
// DBConnect may override the database name in a way the env vars don't reflect
// (e.g. the per-test databases used by the test harness).
func (app *BaseApp) pgConnInfo() (pgConn, error) {
	cfg := ResolveDBConfig(DBConfig{})

	var dbName string
	if err := app.ConcurrentDB().NewQuery("SELECT current_database()").Row(&dbName); err != nil {
		return pgConn{}, fmt.Errorf("failed to resolve the current database name: %w", err)
	}

	var schema string
	if err := app.ConcurrentDB().NewQuery("SELECT current_schema()").Row(&schema); err != nil {
		return pgConn{}, fmt.Errorf("failed to resolve the current schema: %w", err)
	}

	return pgConn{
		Host:     cfg.Host,
		Port:     cfg.Port,
		User:     cfg.User,
		Password: cfg.Password,
		DBName:   dbName,
		SSLMode:  cfg.SSLMode,
		Schema:   schema,
	}, nil
}

// lookupPGBinary resolves an external PostgreSQL client binary, honoring an
// explicit env override before falling back to a PATH lookup.
func lookupPGBinary(name, envOverride string) (string, error) {
	if custom := os.Getenv(envOverride); custom != "" {
		// validate the override: it must reference a regular file with the
		// executable bit set (guard against a path override silently pointing
		// at a missing/non-executable file).
		info, err := os.Stat(custom)
		if err != nil {
			return "", fmt.Errorf("%s (%q) is not accessible: %w", envOverride, custom, err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("%s (%q) does not reference a regular file", envOverride, custom)
		}
		if info.Mode().Perm()&0o111 == 0 {
			return "", fmt.Errorf("%s (%q) is not executable", envOverride, custom)
		}
		return custom, nil
	}

	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf(
			"%q executable not found in PATH: %w (install the postgresql-client package matching your server major version, set %s, or use the \"sqlite\" backup format)",
			name, err, envOverride,
		)
	}

	return path, nil
}

// pgDumpBinary returns the pg_dump executable path (honoring PB_PG_DUMP_BIN).
func pgDumpBinary() (string, error) {
	return lookupPGBinary("pg_dump", "PB_PG_DUMP_BIN")
}

// ExportToPGDump writes a native, custom-format pg_dump archive of the whole
// PostgreSQL database (schema, records, settings, logs, audits and migration
// history) at destPath.
//
// It is the primary native backup format; the inverse [BaseApp.ImportFromPGDump]
// restores it via pg_restore. Unlike [BaseApp.ExportToSQLiteFile] the produced
// dump is NOT engine-portable (it can only be restored onto PostgreSQL) but it
// is a faithful, full-fidelity snapshot.
//
// The method is exported so it can also back the CLI backup command and be
// exercised directly in tests.
func (app *BaseApp) ExportToPGDump(ctx context.Context, destPath string) error {
	bin, err := pgDumpBinary()
	if err != nil {
		return err
	}

	conn, err := app.pgConnInfo()
	if err != nil {
		return err
	}

	// start from a clean file so a re-run never appends to a stale dump
	_ = os.Remove(destPath)

	// -Fc = custom (compressed) archive restorable with pg_restore --clean;
	// --no-owner/--no-privileges keep the dump portable across role names.
	args := []string{"-Fc", "--no-owner", "--no-privileges", "-f", destPath}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), conn.envList()...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pg_dump failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	return nil
}

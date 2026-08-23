package core

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// pgRestoreBinary returns the pg_restore executable path (honoring PB_PG_RESTORE_BIN).
func pgRestoreBinary() (string, error) {
	return lookupPGBinary("pg_restore", "PB_PG_RESTORE_BIN")
}

// ImportFromPGDump restores a native pg_dump archive (extractedDir/pgdata.dump)
// into the current PostgreSQL database using pg_restore, then copies the backed
// up storage files.
//
// It is the inverse of [BaseApp.ExportToPGDump] and the primary native restore
// path. The restore runs "--clean --if-exists" so it fully replaces the current
// database objects with the snapshot. Because the running app is connected to
// the same database, the caller is responsible for restarting the app afterwards
// (RestoreBackup does this) so the collection cache and settings are reloaded.
//
// The method is exported so it can also back the CLI restore command and be
// exercised directly in tests (which skip the process restart).
func (app *BaseApp) ImportFromPGDump(ctx context.Context, extractedDir string) error {
	dumpPath := filepath.Join(extractedDir, pgBackupDumpName)
	if _, err := os.Stat(dumpPath); err != nil {
		return fmt.Errorf("missing %q in the backup: %w", pgBackupDumpName, err)
	}

	bin, err := pgRestoreBinary()
	if err != nil {
		return err
	}

	conn, err := app.pgConnInfo()
	if err != nil {
		return err
	}

	app.Logger().Info("[PG restore] Restoring native PostgreSQL dump", slog.String("database", conn.DBName))

	// --clean --if-exists drops existing objects before recreating them;
	// --no-owner/--no-privileges avoid role mismatches.
	//
	// NB: we intentionally do NOT pass --exit-on-error. A full-database restore
	// emits benign noise (e.g. "DROP EXTENSION pgcrypto" / "already exists")
	// that sets a non-zero exit code; success is gated by the post-restore
	// sanity check below, not by the process exit code.
	args := []string{
		"--clean", "--if-exists", "--no-owner", "--no-privileges",
		"-d", conn.DBName, dumpPath,
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), conn.envList()...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	stderrTail := strings.TrimSpace(stderr.String())
	if runErr != nil && stderrTail != "" {
		app.Logger().Warn(
			"[PG restore] pg_restore reported errors (continuing; verifying result)",
			slog.String("error", runErr.Error()),
			slog.String("stderr", stderrTail),
		)
	}

	// real success gate: the restored database must contain collections
	var collectionsCount int
	if err := app.ConcurrentDB().NewQuery(`SELECT count(*) FROM "_collections"`).Row(&collectionsCount); err != nil {
		return fmt.Errorf("pg_restore verification failed (could not read _collections): %w; pg_restore stderr: %s", err, stderrTail)
	}
	if collectionsCount == 0 {
		return fmt.Errorf("pg_restore produced an empty database (no collections); pg_restore stderr: %s", stderrTail)
	}

	if err := app.importBackupStorage(ctx, extractedDir); err != nil {
		return fmt.Errorf("failed to import the storage files: %w", err)
	}

	app.Logger().Info("[PG restore] Restore completed successfully", slog.Int("collections", collectionsCount))

	return nil
}

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
	"time"
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

	// pre-restore archive validation (W-08): parse the archive TOC BEFORE the
	// destructive pg_restore wipes the current database — a corrupt or
	// truncated dump is rejected while the live data is still intact, and the
	// parsed object set becomes the post-restore completeness expectation.
	// The connection env goes with it (W-14) so the --list invocation resolves
	// the same pg_restore client as the export and the restore below.
	toc, err := pgRestoreListTables(ctx, bin, dumpPath, conn.envList())
	if err != nil {
		return fmt.Errorf("invalid backup archive: %w", err)
	}

	// --clean --if-exists drops existing objects before recreating them;
	// --no-owner/--no-privileges avoid role mismatches.
	//
	// NB: we intentionally do NOT pass --exit-on-error. A full-database restore
	// emits benign noise (see pgRestoreBenignStderrError) that sets a non-zero
	// exit code; success is gated by the stderr classification below plus the
	// post-restore verification, not by the process exit code.
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

	// real errors first (W-08): any pg_restore error that is not a known
	// benign conflict is fatal — a partial restore must fail loudly.
	if fatal := pgRestoreFatalStderrErrors(stderrTail); len(fatal) > 0 {
		return fmt.Errorf("pg_restore failed with real errors:\n%s", strings.Join(fatal, "\n"))
	}

	if runErr != nil && stderrTail != "" {
		// only benign noise is left (fatal lines returned above); keep going —
		// the post-restore verification below decides success
		app.Logger().Warn(
			"[PG restore] pg_restore reported benign errors (continuing; verifying result)",
			slog.String("error", runErr.Error()),
			slog.String("stderr", stderrTail),
		)
	}

	// real success gate (W-08): the restored database must match the archive —
	// every TOC table readable, every TOC index present (post-data completion
	// marker), plus the semantic invariants of a bootable database.
	if err := app.verifyRestoredDatabase(ctx, toc); err != nil {
		return fmt.Errorf("pg_restore verification failed: %w; pg_restore stderr: %s", err, stderrTail)
	}

	if err := app.importBackupStorage(ctx, extractedDir); err != nil {
		return fmt.Errorf("failed to import the storage files: %w", err)
	}

	// G-REL-04: this restore passed the verification gate — persist the
	// verified-restore timestamp (best-effort: observability metadata must
	// not flip a successful restore into a failure).
	if err := app.markBackupVerified(ctx, time.Now()); err != nil {
		app.Logger().Warn(
			"[PG restore] Failed to record the verified-restore timestamp",
			slog.String("error", err.Error()),
		)
	}

	app.Logger().Info(
		"[PG restore] Restore completed successfully",
		slog.Int("tables", len(toc.Tables)),
		slog.Int("indexes", len(toc.Indexes)),
	)

	return nil
}

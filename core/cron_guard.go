package core

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"fmt"
	"log/slog"
)

// cronAdvisoryLockBase is a fixed offset added to a per-job derived key so
// advisory lock keys for cron jobs never collide with the migration-runner
// lock key (migrationsAdvisoryLockKey).
const cronAdvisoryLockBase int64 = 900000000000

// cronLockableDB is implemented by the concrete *dbx.DB returned from the app
// pools, exposing the underlying *sql.DB so a dedicated session can hold a
// session-scoped advisory lock across the whole cron run.
type cronLockableDB interface {
	DB() *sql.DB
}

// cronLockKey derives a stable, collision-avoiding advisory lock key from a
// cron job id.
func cronLockKey(jobID string) int64 {
	sum := sha256.Sum256([]byte(jobID))
	// take 8 bytes from the hash; mask to keep the int64 in a safe positive range
	n := int64(binary.BigEndian.Uint64(sum[:8]) & 0x7fffffffffffffff)
	return cronAdvisoryLockBase + (n % 1_000_000_000)
}

// runWithCronLock runs fn only if this instance holds the (non-blocking)
// session-scoped advisory lock for the given cron job. In a multi-instance
// deployment exactly one instance executes the job per firing; the others skip
// it (NF-5). On a single instance the lock is always free, so behavior is
// unchanged.
//
// Unlike the migration runner (which blocks with pg_advisory_xact_lock), this
// uses pg_try_advisory_lock so a cron tick never blocks on another instance.
// The session lock is held on a dedicated connection for the full fn duration
// and released afterwards, so it also works for jobs that cannot run inside a
// transaction block (eg. VACUUM).
func (app *BaseApp) runWithCronLock(jobID string, logger *slog.Logger, fn func() error) error {
	lockable, ok := app.NonconcurrentDB().(cronLockableDB)
	if !ok {
		// e.g. a mock or non-standard builder in tests: run directly without guard
		return fn()
	}

	conn, err := lockable.DB().Conn(context.Background())
	if err != nil {
		logger.Warn("Failed to open a dedicated connection for cron advisory lock (running without guard)", "jobId", jobID, "error", err)
		return fn()
	}
	defer conn.Close()

	key := cronLockKey(jobID)

	var acquired bool
	err = conn.QueryRowContext(context.Background(),
		fmt.Sprintf("SELECT pg_try_advisory_lock(%d)", key),
	).Scan(&acquired)
	if err != nil {
		logger.Warn("Failed to acquire cron advisory lock (running without guard)", "jobId", jobID, "error", err)
		return fn()
	}
	if !acquired {
		// another instance is already running this job -> skip
		logger.Debug("Skipping cron job, another instance holds its advisory lock", "jobId", jobID)
		return nil
	}

	defer func() {
		conn.ExecContext(context.Background(),
			fmt.Sprintf("SELECT pg_advisory_unlock(%d)", key),
		)
	}()

	return fn()
}

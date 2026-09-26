package core

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Diagnostic counter indices (Prometheus label values: db="data"|"aux").
const (
	dbEventIdxData = 0
	dbEventIdxAux  = 1
)

// dbEventIdx maps a data/aux flag to the counter index.
func dbEventIdx(isForAuxDB bool) int {
	if isForAuxDB {
		return dbEventIdxAux
	}
	return dbEventIdxData
}

// dbEventsState is the diagnostic event counter set kept on the app and read
// at scrape time by the metrics collector (apis.metrics). It must live behind
// a POINTER field on BaseApp: createTxApp shallow-clones the app struct, so
// a value field would give every transaction clone its own invisible
// counters. Nil-safe: every increment method tolerates a nil receiver state
// (apps constructed without NewBaseApp, e.g. zero-value test stubs).
type dbEventsState struct {
	// per-pool event counters (index: dbEventIdxData / dbEventIdxAux)
	queryTimeouts [2]atomic.Int64 // client deadline OR server statement_timeout (57014)
	lockTimeouts  [2]atomic.Int64 // server lock_timeout (55P03)
	txRollbacks   [2]atomic.Int64 // top-level transactions that rolled back

	// backup lifecycle counters (pool-agnostic)
	backupAttempts      atomic.Int64
	backupSuccesses     atomic.Int64
	backupFailures      atomic.Int64
	backupDurationNanos atomic.Int64 // sum; average = / attempts
	backupLastSizeBytes atomic.Int64 // size of the last completed backup
}

// DBEventStats is a point-in-time snapshot of the diagnostic event counters,
// scraped by the /metrics collector. It pairs with the guard rails G-DB-03/04
// (bounded execution/lock waits must be observable) and G-REL-01 (timeout and
// rollback counters).
type DBEventStats struct {
	DataQueryTimeouts int64
	AuxQueryTimeouts  int64
	DataLockTimeouts  int64
	AuxLockTimeouts   int64
	DataTxRollbacks   int64
	AuxTxRollbacks    int64

	BackupAttempts  int64
	BackupSuccesses int64
	BackupFailures  int64
	BackupDuration  time.Duration // sum of completed (success or failure) backups
	BackupLastSize  int64         // bytes of the last successful backup
}

// snapshot returns a copy of the current counters.
func (s *dbEventsState) snapshot() DBEventStats {
	if s == nil {
		return DBEventStats{}
	}
	return DBEventStats{
		DataQueryTimeouts: s.queryTimeouts[dbEventIdxData].Load(),
		AuxQueryTimeouts:  s.queryTimeouts[dbEventIdxAux].Load(),
		DataLockTimeouts:  s.lockTimeouts[dbEventIdxData].Load(),
		AuxLockTimeouts:   s.lockTimeouts[dbEventIdxAux].Load(),
		DataTxRollbacks:   s.txRollbacks[dbEventIdxData].Load(),
		AuxTxRollbacks:    s.txRollbacks[dbEventIdxAux].Load(),
		BackupAttempts:    s.backupAttempts.Load(),
		BackupSuccesses:   s.backupSuccesses.Load(),
		BackupFailures:    s.backupFailures.Load(),
		BackupDuration:    time.Duration(s.backupDurationNanos.Load()),
		BackupLastSize:    s.backupLastSizeBytes.Load(),
	}
}

// recordDBQueryError classifies a data-path query/execution error into the
// diagnostic timeout counters:
//
//   - client-side deadline exceeded (queryTimeoutHook / withWriteDeadline);
//   - server-side statement_timeout (SQLSTATE 57014);
//   - server-side lock_timeout (SQLSTATE 55P03).
//
// Anything else is deliberately not counted: e.g. deadlocks (40P01) and
// connection failures are outside the diagnostic set tied to G-DB-03/04 and
// G-REL-01 (bounded operation must be observable, not every failure class).
// Call it where the bounded statement's error surfaces.
//
// Coverage today: the record query path (RecordQuery, data side) and the
// model write execute paths (create/update/delete, data + aux). Raw
// builder queries (app.DB().NewQuery) and aux reads are not instrumented.
func (app *BaseApp) recordDBQueryError(err error, isForAuxDB bool) {
	if app == nil || app.dbEvents == nil || err == nil {
		return
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "57014": // query_canceled — statement_timeout
			app.dbEvents.queryTimeouts[dbEventIdx(isForAuxDB)].Add(1)
		case "55P03": // lock_not_available — lock_timeout
			app.dbEvents.lockTimeouts[dbEventIdx(isForAuxDB)].Add(1)
		}
		return
	}

	if errors.Is(err, context.DeadlineExceeded) {
		app.dbEvents.queryTimeouts[dbEventIdx(isForAuxDB)].Add(1)
	}
}

// recordTxRollback counts a top-level transaction that rolled back (its
// callback failed). Nested RunInTransaction calls inside an existing
// transaction reuse the outer transaction and are NOT counted.
func (app *BaseApp) recordTxRollback(isForAuxDB bool) {
	if app == nil || app.dbEvents == nil {
		return
	}
	app.dbEvents.txRollbacks[dbEventIdx(isForAuxDB)].Add(1)
}

// recordBackupAttempt counts a started backup creation.
func (app *BaseApp) recordBackupAttempt() {
	if app == nil || app.dbEvents == nil {
		return
	}
	app.dbEvents.backupAttempts.Add(1)
}

// recordBackupOutcome counts a finished backup (success or failure) together
// with its duration.
func (app *BaseApp) recordBackupOutcome(err error, duration time.Duration) {
	if app == nil || app.dbEvents == nil {
		return
	}
	app.dbEvents.backupDurationNanos.Add(int64(duration))
	if err != nil {
		app.dbEvents.backupFailures.Add(1)
		return
	}
	app.dbEvents.backupSuccesses.Add(1)
}

// recordBackupSize records the byte size of the last successfully created
// backup archive.
func (app *BaseApp) recordBackupSize(size int64) {
	if app == nil || app.dbEvents == nil {
		return
	}
	app.dbEvents.backupLastSizeBytes.Store(size)
}

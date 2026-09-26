package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/dbx"
)

// queryTimeoutHook bounds a query with a client-side timeout (when the query
// carries no context yet) and classifies the resulting error into the
// diagnostic counters (see recordDBQueryError): a deadline-exceeded error
// counts as a query timeout. It is installed on the record query path
// (RecordQuery — the data side).
func queryTimeoutHook(app *BaseApp, timeout time.Duration) dbx.ExecHookFunc {
	return func(q *dbx.Query, op func() error) error {
		if q.Context() == nil {
			cancelCtx, cancel := context.WithTimeout(context.Background(), timeout)
			defer func() {
				cancel()
				// SA1012: intentionally pass nil to reset the dbx query
				// context after use, so the pooled query never carries a
				// canceled context into a later execution.
				q.WithContext(nil) //nolint:staticcheck
			}()
			q.WithContext(cancelCtx)
		}

		execErr := op()
		if execErr != nil && !errors.Is(execErr, sql.ErrNoRows) {
			app.recordDBQueryError(execErr, false)
			execErr = fmt.Errorf("%w; failed query: %s", execErr, q.SQL())
		}

		return execErr
	}
}

// withWriteDeadline is the write-path analogue of queryTimeoutHook: it bounds a
// write statement with a client-side deadline so it cannot hang indefinitely on
// a silently dropped connection (network partition with no TCP RST/FIN), which
// would otherwise hold a pooled connection until the OS TCP retransmit gives up
// (~minutes). The server-side statement_timeout/lock_timeout cannot rescue such
// a case because the abort message cannot cross a dead socket.
//
// A timeout is applied ONLY when ctx has no deadline yet, so an explicit
// caller-provided deadline (e.g. a request context, or an outer bound) always
// wins. The returned CancelFunc must be deferred by the caller.
func withWriteDeadline(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/dbx"
)

func queryTimeoutHook(timeout time.Duration) dbx.ExecHookFunc {
	return func(q *dbx.Query, op func() error) error {
		if q.Context() == nil {
			cancelCtx, cancel := context.WithTimeout(context.Background(), timeout)
			defer func() {
				cancel()
				q.WithContext(nil)
			}()
			q.WithContext(cancelCtx)
		}

		execErr := op()
		if execErr != nil && !errors.Is(execErr, sql.ErrNoRows) {
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

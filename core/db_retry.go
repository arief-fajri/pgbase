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

package migrations

import (
	"github.com/arief-fajri/pgbase/core"
)

// D-4 hardening: the outbox reader advances with ORDER BY created, id and the
// cleanup cron filters on created/processed_at, but the initial migration only
// created a partial index on (id) WHERE processed_at IS NULL, which cannot serve
// either query. Add a (created, id) index so reads and cleanup stay fast as the
// table grows under multi-instance write load (PGB-N02).
func init() {
	core.SystemMigrations.Add(&core.Migration{
		Up: func(txApp core.App) error {
			_, execErr := txApp.DB().NewQuery(`
				CREATE INDEX IF NOT EXISTS idx_realtime_outbox_created_id
					ON _realtime_outbox (created, id)
			`).Execute()

			return execErr
		},
		Down: func(txApp core.App) error {
			_, err := txApp.DB().NewQuery(`
				DROP INDEX IF EXISTS idx_realtime_outbox_created_id
			`).Execute()
			return err
		},
	})
}

package migrations

import (
	"github.com/arief-fajri/pgbase/core"
)

// D-4 (step 3): stamp each outbox row with the publishing instance's identity
// so a listener can skip its OWN events. Without it the publisher double-
// broadcasts to its own local clients: once synchronously at write time and
// again when its own listener reads the row back off the outbox.
//
// Nullable + backfilled to NULL: legacy rows (and any row from an instance that
// predates this column) have origin=NULL and are still delivered, because the
// reader excludes only rows whose origin equals its own id (NULL never does).
func init() {
	core.SystemMigrations.Add(&core.Migration{
		Up: func(txApp core.App) error {
			_, execErr := txApp.DB().NewQuery(`
				ALTER TABLE "_realtime_outbox"
					ADD COLUMN IF NOT EXISTS "origin" TEXT DEFAULT NULL
			`).Execute()

			return execErr
		},
		Down: func(txApp core.App) error {
			_, err := txApp.DB().NewQuery(`
				ALTER TABLE "_realtime_outbox" DROP COLUMN IF EXISTS "origin"
			`).Execute()
			return err
		},
	})
}

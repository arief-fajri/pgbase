package migrations

import (
	"github.com/arief-fajri/pgbase/core"
)

// instanceHeartbeatsTableName is the aux-DB table used by the multi-instance
// loud-warning guard (R-1b). Each app instance periodically upserts its
// presence so other instances can detect a multi-instance deployment and warn
// (realtime broadcasts are per-instance until the Batch-3 Broadcaster lands).
func init() {
	core.SystemMigrations.Add(&core.Migration{
		Up: func(txApp core.App) error {
			_, execErr := txApp.AuxDB().NewQuery(`
				CREATE TABLE IF NOT EXISTS _instance_heartbeats (
					instance_id TEXT PRIMARY KEY NOT NULL,
					seen_at    TIMESTAMPTZ DEFAULT NOW() NOT NULL
				);
			`).Execute()

			return execErr
		},
		Down: func(txApp core.App) error {
			_, err := txApp.AuxDB().DropTable("_instance_heartbeats").Execute()
			return err
		},
		ReapplyCondition: func(txApp core.App, runner *core.MigrationsRunner, fileName string) (bool, error) {
			// reapply only if the table doesn't exist
			exists := txApp.AuxHasTable("_instance_heartbeats")
			return !exists, nil
		},
	})
}
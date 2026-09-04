package migrations

import (
	"github.com/arief-fajri/pgbase/core"
)

// D-4 (step 1): the DB-backed outbox table for the cross-instance realtime
// broadcaster. Events are rows (create/update carry identifiers, delete carries
// a full snapshot); NOTIFY is only a wake-up signal, so it is not bounded by the
// 8KB NOTIFY payload limit and delete snapshots can be full records.
func init() {
	core.SystemMigrations.Add(&core.Migration{
		Up: func(txApp core.App) error {
			_, execErr := txApp.DB().NewQuery(`
				CREATE TABLE IF NOT EXISTS _realtime_outbox (
					id           TEXT PRIMARY KEY DEFAULT ('r'||lower(encode(gen_random_bytes(7), 'hex'))) NOT NULL,
					action       TEXT NOT NULL,
					collection   TEXT NOT NULL,
					record_id    TEXT NOT NULL,
					snapshot     JSONB DEFAULT NULL,
					created      TIMESTAMPTZ DEFAULT NOW() NOT NULL,
					processed_at TIMESTAMPTZ DEFAULT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_realtime_outbox_pending
					ON _realtime_outbox (id)
					WHERE processed_at IS NULL;
			`).Execute()

			return execErr
		},
		Down: func(txApp core.App) error {
			_, err := txApp.DB().DropTable("_realtime_outbox").Execute()
			return err
		},
	})
}
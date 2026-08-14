package migrations

import (
	"github.com/arief-fajri/pgbase/core"
)

func init() {
	core.SystemMigrations.Add(&core.Migration{
		Up: func(txApp core.App) error {
			_, execErr := txApp.AuxDB().NewQuery(`
				CREATE TABLE IF NOT EXISTS _logs (
					id      TEXT PRIMARY KEY DEFAULT ('r'||lower(encode(gen_random_bytes(7), 'hex'))) NOT NULL,
					level   INTEGER DEFAULT 0 NOT NULL,
					message TEXT DEFAULT '' NOT NULL,
					data    JSONB DEFAULT '{}'::jsonb NOT NULL,
					created TIMESTAMPTZ DEFAULT NOW() NOT NULL,
					updated TIMESTAMPTZ DEFAULT NOW() NOT NULL
				);

				CREATE INDEX IF NOT EXISTS idx_logs_level ON _logs (level);
				CREATE INDEX IF NOT EXISTS idx_logs_created ON _logs (created);

				ALTER TABLE _logs ADD COLUMN IF NOT EXISTS updated TIMESTAMPTZ DEFAULT NOW() NOT NULL;
			`).Execute()

			return execErr
		},
		Down: func(txApp core.App) error {
			_, err := txApp.AuxDB().DropTable("_logs").Execute()
			return err
		},
		ReapplyCondition: func(txApp core.App, runner *core.MigrationsRunner, fileName string) (bool, error) {
			// reapply only if the _logs table doesn't exist
			exists := txApp.AuxHasTable("_logs")
			return !exists, nil
		},
	})
}

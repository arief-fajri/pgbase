package migrations

import (
	"github.com/arief-fajri/pgbase/core"
)

// IDX-3 (lighter alternative): tune the per-table autovacuum of "_logs" so
// dead tuples left by the hourly old-logs DELETE get reclaimed aggressively
// instead of accumulating up to the default scale_factor (~20% of the table).
//
// Full RANGE partitioning with DROP-partition retention (the "_audits" design)
// remains the high-scale end-state but is a high-risk live-table restructure;
// this per-table tuning delivers most of the bloat benefit cheaply and safely.
func init() {
	core.SystemMigrations.Add(&core.Migration{
		Up: func(txApp core.App) error {
			// autovacuum only keeps tables healthy on its own when the default
			// thresholds are low enough for a table that is bulk-deleted in
			// batches; the defaults (scale_factor 0.2) are too high.
			_, execErr := txApp.AuxDB().NewQuery(`
				ALTER TABLE "_logs" SET (
					autovacuum_vacuum_scale_factor = 0.02,
					autovacuum_vacuum_threshold    = 10000,
					autovacuum_analyze_scale_factor = 0.02
				)
			`).Execute()

			return execErr
		},
		Down: func(txApp core.App) error {
			_, err := txApp.AuxDB().NewQuery(`
				ALTER TABLE "_logs" RESET (
					autovacuum_vacuum_scale_factor,
					autovacuum_vacuum_threshold,
					autovacuum_analyze_scale_factor
				)
			`).Execute()
			return err
		},
	})
}
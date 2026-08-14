package tests

import (
	"github.com/arief-fajri/pgbase/core"
	"github.com/pocketbase/dbx"
)

type collectionInfo struct {
	Name string `db:"name"`
	Id   string `db:"id"`
}

// cleanupTestDB clears test data while preserving system state.
func cleanupTestDB(app core.App) {
	// Auth record tables - truncate
	systemTables := []string{
		"_superusers", "_mfas", "_otps",
		"_externalAuths", "_authOrigins", "_logs",
	}
	for _, table := range systemTables {
		execSafe(app, "TRUNCATE TABLE \""+table+"\" CASCADE")
	}

	// Users and clients (seed auth tables)
	execSafe(app, "TRUNCATE TABLE \"users\" CASCADE")
	execSafe(app, "TRUNCATE TABLE \"clients\" CASCADE")

	// Clear settings params
	execSafe(app, "TRUNCATE TABLE \"_params\" CASCADE")

	// Find and remove any leftover test-created collections
	// (e.g. "test123", "new_name" from migratecmd tests).
	// Also re-remove seed collections from _collections so that
	// SeedTestData can recreate them (including their data tables).
	// Keep only system collections by name.
	keepNames := map[string]bool{
		"_superusers": true, "_mfas": true, "_otps": true,
		"_externalAuths": true, "_authOrigins": true,
	}

	var collections []collectionInfo
	if err := app.DB().
		NewQuery("SELECT id, name FROM _collections").
		All(&collections); err != nil {
		return
	}

	for _, col := range collections {
		if keepNames[col.Name] {
			continue
		}
		app.DB().NewQuery("DELETE FROM _collections WHERE id = {:id}").
			Bind(dbx.Params{"id": col.Id}).
			Execute()
		execSafe(app, "DROP TABLE IF EXISTS \""+col.Name+"\" CASCADE")
	}
}

func execSafe(app core.App, query string) {
	app.DB().NewQuery(query).Execute()
}

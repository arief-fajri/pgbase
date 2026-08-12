package tests

import (
	"os"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/arief-fajri/pgbase/core"
)

func SetupTestDB(t *testing.T) *core.BaseApp {
	t.Helper()

	config := core.BaseAppConfig{
		DBConnect: func(config core.DBConfig) (*dbx.DB, error) {
			return core.DefaultDBConnect(core.DBConfig{
				Host:         getEnvOrDefault("PGTEST_HOST", "localhost"),
				Port:         5433,
				User:         getEnvOrDefault("PGTEST_USER", "test"),
				Password:     getEnvOrDefault("PGTEST_PASSWORD", "test"),
				DBName:       getEnvOrDefault("PGTEST_DBNAME", "pgbase_test"),
				SSLMode:      "disable",
				MaxOpenConns: 10,
				MaxIdleConns: 5,
			})
		},
		DataDir: t.TempDir(),
	}

	app := core.NewBaseApp(config)
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Failed to bootstrap: %v", err)
	}

	if err := app.RunAllMigrations(); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return app
}

func CleanupTestDB(t *testing.T, app core.App) {
	t.Helper()

	tables := []string{
		"users", "_superusers", "_authOrigins",
		"_otps", "_mfas", "_externalAuths",
		"_params", "_collections", "_migrations",
	}

	for _, table := range tables {
		app.DB().NewQuery("TRUNCATE " + table + " CASCADE").Execute()
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

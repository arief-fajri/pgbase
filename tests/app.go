// Package tests provides common helpers and mocks used in PG-BASE application tests.
package tests

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tools/hook"
	"github.com/pocketbase/dbx"

	_ "github.com/arief-fajri/pgbase/migrations"
)

// schemaCounter provides unique schema names for test isolation.
var schemaCounter atomic.Int64

// schemaInitDB is a shared connection used for creating test schemas.
var schemaInitDB *dbx.DB
var schemaInitOnce sync.Once

// templateSchemaName is the name of the pre-built migration template schema.
var templateSchemaName string
var templateInitOnce sync.Once
var templateInitErr error

// orphanCleanupOnce ensures we only drop orphaned schemas once per process.
var orphanCleanupOnce sync.Once

func getSchemaInitDB() (*dbx.DB, error) {
	var err error
	schemaInitOnce.Do(func() {
		schemaInitDB, err = core.DefaultDBConnect(core.DBConfig{
			Host:         getEnvOrDefault("PGTEST_HOST", "localhost"),
			Port:         5433,
			User:         getEnvOrDefault("PGTEST_USER", "test"),
			Password:     getEnvOrDefault("PGTEST_PASSWORD", "test"),
			DBName:       getEnvOrDefault("PGTEST_DBNAME", "pgbase_test"),
			SSLMode:      "disable",
			MaxOpenConns: 50,
			MaxIdleConns: 10,
		})
	})
	return schemaInitDB, err
}

// dropOrphanSchemas drops any leftover pb_test_* and pb_template_* schemas
// from previous crashed runs to keep the catalog clean.
func dropOrphanSchemas() {
	orphanCleanupOnce.Do(func() {
		db, err := getSchemaInitDB()
		if err != nil {
			return
		}
		pid := os.Getpid()
		rows, qErr := db.NewQuery(
			fmt.Sprintf(`SELECT schema_name FROM information_schema.schemata WHERE (schema_name LIKE '%s') OR (schema_name LIKE '%s' AND schema_name != 'pb_template_%d')`, "pb\\_test\\_%", "pb\\_template\\_%", pid),
		).Rows()
		if qErr != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var schema string
			if rows.Scan(&schema) != nil {
				continue
			}
			db.NewQuery("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Execute()
		}
	})
}

// connectToSchema returns a DBConnect callback that connects to the specified schema
// with the standard test environment settings.
func connectToSchema(schemaName string) func(cfg core.DBConfig) (*dbx.DB, error) {
	return func(cfg core.DBConfig) (*dbx.DB, error) {
		if cfg.Host == "" {
			cfg.Host = getEnvOrDefault("PGTEST_HOST", "localhost")
		}
		cfg.Port = 5433
		if cfg.User == "" {
			cfg.User = getEnvOrDefault("PGTEST_USER", "test")
		}
		if cfg.Password == "" {
			cfg.Password = getEnvOrDefault("PGTEST_PASSWORD", "test")
		}
		if cfg.DBName == "" {
			cfg.DBName = getEnvOrDefault("PGTEST_DBNAME", "pgbase_test")
		}
		if cfg.SSLMode == "" {
			cfg.SSLMode = "disable"
		}
		cfg.Schema = schemaName
		return core.DefaultDBConnect(cfg)
	}
}

// initTemplateSchema builds a fully-migrated and seeded template schema ONCE
// per test process. All subsequent test schemas clone from this template,
// avoiding the per-app cost of re-running all migrations + seed.
func initTemplateSchema() error {
	templateInitOnce.Do(func() {
		// clean orphan schemas first
		dropOrphanSchemas()

		pid := os.Getpid()
		templateSchemaName = fmt.Sprintf("pb_template_%d", pid)

		db, err := getSchemaInitDB()
		if err != nil {
			templateInitErr = fmt.Errorf("getSchemaInitDB: %w", err)
			return
		}

		if _, qErr := db.NewQuery("CREATE SCHEMA IF NOT EXISTS " + templateSchemaName).Execute(); qErr != nil {
			templateInitErr = fmt.Errorf("create template schema: %w", qErr)
			return
		}

		templateDataDir, dErr := os.MkdirTemp("", "pb_template_data")
		if dErr != nil {
			templateInitErr = dErr
			return
		}

		templateApp := core.NewBaseApp(core.BaseAppConfig{
			DataDir:          templateDataDir,
			EncryptionEnv:    "pb_test_env",
			DataMaxOpenConns: 2,
			AuxMaxOpenConns:  1,
			DBConnect:        connectToSchema(templateSchemaName),
		})

		if bErr := templateApp.Bootstrap(); bErr != nil {
			templateInitErr = fmt.Errorf("template bootstrap: %w", bErr)
			return
		}
		if _, qErr := templateApp.DB().NewQuery("Select 1").Execute(); qErr != nil {
			templateInitErr = qErr
			return
		}
		if _, qErr := templateApp.AuxDB().NewQuery("Select 1").Execute(); qErr != nil {
			templateInitErr = qErr
			return
		}
		if mErr := templateApp.RunAllMigrations(); mErr != nil {
			templateInitErr = fmt.Errorf("template RunAllMigrations: %w", mErr)
			return
		}
		if sErr := SeedTestData(templateApp); sErr != nil {
			templateInitErr = fmt.Errorf("template SeedTestData: %w", sErr)
			return
		}

		templateApp.ResetBootstrapState()
	})
	return templateInitErr
}

// cloneSchemaFromTemplate copies all tables and data from the template schema
// into the specified target schema. This avoids re-running migrations and
// re-seeding data for each test.
func cloneSchemaFromTemplate(targetSchema string) error {
	db, err := getSchemaInitDB()
	if err != nil {
		return err
	}

	rows, qErr := db.NewQuery(
		fmt.Sprintf("SELECT tablename FROM pg_tables WHERE schemaname = '%s' ORDER BY tablename", templateSchemaName),
	).Rows()
	if qErr != nil {
		return qErr
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		if sErr := rows.Scan(&tableName); sErr != nil {
			return sErr
		}

		createSQL := fmt.Sprintf(
			`CREATE TABLE "%s"."%s" (LIKE "%s"."%s" INCLUDING ALL)`,
			targetSchema, tableName, templateSchemaName, tableName,
		)
		if _, cErr := db.NewQuery(createSQL).Execute(); cErr != nil {
			return fmt.Errorf("clone table %s: %w", tableName, cErr)
		}

		insertSQL := fmt.Sprintf(
			`INSERT INTO "%s"."%s" SELECT * FROM "%s"."%s"`,
			targetSchema, tableName, templateSchemaName, tableName,
		)
		if _, iErr := db.NewQuery(insertSQL).Execute(); iErr != nil {
			return fmt.Errorf("clone data %s: %w", tableName, iErr)
		}
	}

	return rows.Err()
}

// TestApp is a wrapper app instance used for testing.
type TestApp struct {
	*core.BaseApp

	mux sync.Mutex

	// EventCalls defines a map to inspect which app events
	// (and how many times) were triggered.
	EventCalls map[string]int

	TestMailer *TestMailer

	// schemaName is the isolated PostgreSQL schema for this test instance.
	schemaName string
}

// Cleanup resets the test application state and removes the test
// app's dataDir from the filesystem.
//
// After this call, the app instance shouldn't be used anymore.
func (t *TestApp) Cleanup() {
	if t == nil || t.BaseApp == nil {
		return
	}

	event := new(core.TerminateEvent)
	event.App = t

	t.OnTerminate().Trigger(event, func(e *core.TerminateEvent) error {
		// Drop the isolated schema, removing all tables and data.
		if t.schemaName != "" {
			t.DB().NewQuery("DROP SCHEMA IF EXISTS " + t.schemaName + " CASCADE").Execute()
		}

		t.TestMailer.Reset()
		t.ResetEventCalls()
		t.ResetBootstrapState()

		return e.Next()
	})

	if t.DataDir() != "" {
		os.RemoveAll(t.DataDir())
	}
}

// ResetEventCalls resets the EventCalls counter.
func (t *TestApp) ResetEventCalls() {
	t.mux.Lock()
	defer t.mux.Unlock()

	t.EventCalls = make(map[string]int)
}

func (t *TestApp) registerEventCall(name string) {
	t.mux.Lock()
	defer t.mux.Unlock()

	if t.EventCalls == nil {
		t.EventCalls = make(map[string]int)
	}

	t.EventCalls[name]++
}

// NewTestApp creates and initializes a test application instance.
//
// It is the caller's responsibility to call app.Cleanup() when the app is no longer needed.
func NewTestApp(optTestDataDir ...string) (*TestApp, error) {
	var testDataDir string
	if len(optTestDataDir) > 0 {
		testDataDir = optTestDataDir[0]
	}

	return NewTestAppWithConfig(core.BaseAppConfig{
		DataDir:         testDataDir,
		EncryptionEnv:   "pb_test_env",
		DataMaxOpenConns: 2,
		AuxMaxOpenConns:  1,
	})
}

// NewTestAppWithConfig creates and initializes a test application instance
// from the provided config.
//
// If config.DataDir is not set it fallbacks to the default internal test data directory.
//
// config.DataDir is cloned for each new test application instance.
//
// It is the caller's responsibility to call app.Cleanup() when the app is no longer needed.
func NewTestAppWithConfig(config core.BaseAppConfig) (*TestApp, error) {
	if config.DataDir == "" {
		// fallback to the default test data directory
		_, currentFile, _, _ := runtime.Caller(0)
		config.DataDir = filepath.Join(path.Dir(currentFile), "data")
	}

	tempDir, err := TempDirClone(config.DataDir)
	if err != nil {
		return nil, err
	}

	// replace with the clone
	config.DataDir = tempDir

	if config.DataMaxOpenConns <= 0 {
		config.DataMaxOpenConns = 2
	}
	if config.AuxMaxOpenConns <= 0 {
		config.AuxMaxOpenConns = 1
	}

	// Generate a unique schema name for this test app.
	// Each test gets its own schema, providing full database isolation
	// and allowing safe parallel execution.
	schemaName := generateSchemaName()

	// Ensure the template schema is built once per process.
	if err := initTemplateSchema(); err != nil {
		return nil, fmt.Errorf("template init failed: %w", err)
	}

	// Create a new empty schema for this test app.
	if err := createTestSchema(schemaName); err != nil {
		return nil, fmt.Errorf("failed to create schema %s: %w", schemaName, err)
	}

	// Clone all tables (structure + data) from the template schema,
	// avoiding re-running migrations and re-seeding data.
	if err := cloneSchemaFromTemplate(schemaName); err != nil {
		return nil, fmt.Errorf("failed to clone template to schema %s: %w", schemaName, err)
	}

	config.DBConnect = connectToSchema(schemaName)

	app := core.NewBaseApp(config)

	// load data dir and db connections
	// (Bootstrap still runs, but system migrations are skipped
	//  because the cloned _migrations table marks them as applied)
	if err := app.Bootstrap(); err != nil {
		return nil, err
	}

	// ensure that the Dao and DB configurations are properly loaded
	if _, err := app.DB().NewQuery("Select 1").Execute(); err != nil {
		return nil, err
	}
	if _, err := app.AuxDB().NewQuery("Select 1").Execute(); err != nil {
		return nil, err
	}

	// RunAllMigrations is skipped because the cloned _migrations table
	// already has all app migrations marked as applied.
	//
	// SeedTestData is skipped because the template schema already
	// contains all test data (collections, records, auth users).

	// force disable request logs because the logs db call execute in a separate
	// go routine and it is possible to panic due to earlier api test completion.
	app.Settings().Logs.MaxDays = 0

	t := &TestApp{
		BaseApp:    app,
		EventCalls: make(map[string]int),
		TestMailer: &TestMailer{},
		schemaName: schemaName,
	}

	t.OnBootstrap().Bind(&hook.Handler[*core.BootstrapEvent]{
		Func: func(e *core.BootstrapEvent) error {
			t.registerEventCall("OnBootstrap")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Func: func(e *core.ServeEvent) error {
			t.registerEventCall("OnServe")
			e.InstallerFunc = nil // https://github.com/arief-fajri/pgbase/discussions/7202
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
		Func: func(e *core.TerminateEvent) error {
			t.registerEventCall("OnTerminate")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnBackupCreate().Bind(&hook.Handler[*core.BackupEvent]{
		Func: func(e *core.BackupEvent) error {
			t.registerEventCall("OnBackupCreate")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnBackupRestore().Bind(&hook.Handler[*core.BackupEvent]{
		Func: func(e *core.BackupEvent) error {
			t.registerEventCall("OnBackupRestore")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelCreate().Bind(&hook.Handler[*core.ModelEvent]{
		Func: func(e *core.ModelEvent) error {
			t.registerEventCall("OnModelCreate")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelCreateExecute().Bind(&hook.Handler[*core.ModelEvent]{
		Func: func(e *core.ModelEvent) error {
			t.registerEventCall("OnModelCreateExecute")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelAfterCreateSuccess().Bind(&hook.Handler[*core.ModelEvent]{
		Func: func(e *core.ModelEvent) error {
			t.registerEventCall("OnModelAfterCreateSuccess")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelAfterCreateError().Bind(&hook.Handler[*core.ModelErrorEvent]{
		Func: func(e *core.ModelErrorEvent) error {
			t.registerEventCall("OnModelAfterCreateError")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelUpdate().Bind(&hook.Handler[*core.ModelEvent]{
		Func: func(e *core.ModelEvent) error {
			t.registerEventCall("OnModelUpdate")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelUpdateExecute().Bind(&hook.Handler[*core.ModelEvent]{
		Func: func(e *core.ModelEvent) error {
			t.registerEventCall("OnModelUpdateExecute")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelAfterUpdateSuccess().Bind(&hook.Handler[*core.ModelEvent]{
		Func: func(e *core.ModelEvent) error {
			t.registerEventCall("OnModelAfterUpdateSuccess")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelAfterUpdateError().Bind(&hook.Handler[*core.ModelErrorEvent]{
		Func: func(e *core.ModelErrorEvent) error {
			t.registerEventCall("OnModelAfterUpdateError")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelValidate().Bind(&hook.Handler[*core.ModelEvent]{
		Func: func(e *core.ModelEvent) error {
			t.registerEventCall("OnModelValidate")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelDelete().Bind(&hook.Handler[*core.ModelEvent]{
		Func: func(e *core.ModelEvent) error {
			t.registerEventCall("OnModelDelete")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelDeleteExecute().Bind(&hook.Handler[*core.ModelEvent]{
		Func: func(e *core.ModelEvent) error {
			t.registerEventCall("OnModelDeleteExecute")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelAfterDeleteSuccess().Bind(&hook.Handler[*core.ModelEvent]{
		Func: func(e *core.ModelEvent) error {
			t.registerEventCall("OnModelAfterDeleteSuccess")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnModelAfterDeleteError().Bind(&hook.Handler[*core.ModelErrorEvent]{
		Func: func(e *core.ModelErrorEvent) error {
			t.registerEventCall("OnModelAfterDeleteError")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordEnrich().Bind(&hook.Handler[*core.RecordEnrichEvent]{
		Func: func(e *core.RecordEnrichEvent) error {
			t.registerEventCall("OnRecordEnrich")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordValidate().Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			t.registerEventCall("OnRecordValidate")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordCreate().Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			t.registerEventCall("OnRecordCreate")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordCreateExecute().Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			t.registerEventCall("OnRecordCreateExecute")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordAfterCreateSuccess().Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			t.registerEventCall("OnRecordAfterCreateSuccess")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordAfterCreateError().Bind(&hook.Handler[*core.RecordErrorEvent]{
		Func: func(e *core.RecordErrorEvent) error {
			t.registerEventCall("OnRecordAfterCreateError")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordUpdate().Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			t.registerEventCall("OnRecordUpdate")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordUpdateExecute().Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			t.registerEventCall("OnRecordUpdateExecute")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordAfterUpdateSuccess().Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			t.registerEventCall("OnRecordAfterUpdateSuccess")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordAfterUpdateError().Bind(&hook.Handler[*core.RecordErrorEvent]{
		Func: func(e *core.RecordErrorEvent) error {
			t.registerEventCall("OnRecordAfterUpdateError")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordDelete().Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			t.registerEventCall("OnRecordDelete")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordDeleteExecute().Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			t.registerEventCall("OnRecordDeleteExecute")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordAfterDeleteSuccess().Bind(&hook.Handler[*core.RecordEvent]{
		Func: func(e *core.RecordEvent) error {
			t.registerEventCall("OnRecordAfterDeleteSuccess")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordAfterDeleteError().Bind(&hook.Handler[*core.RecordErrorEvent]{
		Func: func(e *core.RecordErrorEvent) error {
			t.registerEventCall("OnRecordAfterDeleteError")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionValidate().Bind(&hook.Handler[*core.CollectionEvent]{
		Func: func(e *core.CollectionEvent) error {
			t.registerEventCall("OnCollectionValidate")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionCreate().Bind(&hook.Handler[*core.CollectionEvent]{
		Func: func(e *core.CollectionEvent) error {
			t.registerEventCall("OnCollectionCreate")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionCreateExecute().Bind(&hook.Handler[*core.CollectionEvent]{
		Func: func(e *core.CollectionEvent) error {
			t.registerEventCall("OnCollectionCreateExecute")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionAfterCreateSuccess().Bind(&hook.Handler[*core.CollectionEvent]{
		Func: func(e *core.CollectionEvent) error {
			t.registerEventCall("OnCollectionAfterCreateSuccess")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionAfterCreateError().Bind(&hook.Handler[*core.CollectionErrorEvent]{
		Func: func(e *core.CollectionErrorEvent) error {
			t.registerEventCall("OnCollectionAfterCreateError")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionUpdate().Bind(&hook.Handler[*core.CollectionEvent]{
		Func: func(e *core.CollectionEvent) error {
			t.registerEventCall("OnCollectionUpdate")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionUpdateExecute().Bind(&hook.Handler[*core.CollectionEvent]{
		Func: func(e *core.CollectionEvent) error {
			t.registerEventCall("OnCollectionUpdateExecute")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionAfterUpdateSuccess().Bind(&hook.Handler[*core.CollectionEvent]{
		Func: func(e *core.CollectionEvent) error {
			t.registerEventCall("OnCollectionAfterUpdateSuccess")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionAfterUpdateError().Bind(&hook.Handler[*core.CollectionErrorEvent]{
		Func: func(e *core.CollectionErrorEvent) error {
			t.registerEventCall("OnCollectionAfterUpdateError")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionDelete().Bind(&hook.Handler[*core.CollectionEvent]{
		Func: func(e *core.CollectionEvent) error {
			t.registerEventCall("OnCollectionDelete")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionDeleteExecute().Bind(&hook.Handler[*core.CollectionEvent]{
		Func: func(e *core.CollectionEvent) error {
			t.registerEventCall("OnCollectionDeleteExecute")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionAfterDeleteSuccess().Bind(&hook.Handler[*core.CollectionEvent]{
		Func: func(e *core.CollectionEvent) error {
			t.registerEventCall("OnCollectionAfterDeleteSuccess")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionAfterDeleteError().Bind(&hook.Handler[*core.CollectionErrorEvent]{
		Func: func(e *core.CollectionErrorEvent) error {
			t.registerEventCall("OnCollectionAfterDeleteError")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnMailerSend().Bind(&hook.Handler[*core.MailerEvent]{
		Func: func(e *core.MailerEvent) error {
			if t.TestMailer == nil {
				t.TestMailer = &TestMailer{}
			}
			e.Mailer = t.TestMailer
			t.registerEventCall("OnMailerSend")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnMailerRecordAuthAlertSend().Bind(&hook.Handler[*core.MailerRecordEvent]{
		Func: func(e *core.MailerRecordEvent) error {
			t.registerEventCall("OnMailerRecordAuthAlertSend")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnMailerRecordPasswordResetSend().Bind(&hook.Handler[*core.MailerRecordEvent]{
		Func: func(e *core.MailerRecordEvent) error {
			t.registerEventCall("OnMailerRecordPasswordResetSend")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnMailerRecordVerificationSend().Bind(&hook.Handler[*core.MailerRecordEvent]{
		Func: func(e *core.MailerRecordEvent) error {
			t.registerEventCall("OnMailerRecordVerificationSend")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnMailerRecordEmailChangeSend().Bind(&hook.Handler[*core.MailerRecordEvent]{
		Func: func(e *core.MailerRecordEvent) error {
			t.registerEventCall("OnMailerRecordEmailChangeSend")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnMailerRecordOTPSend().Bind(&hook.Handler[*core.MailerRecordEvent]{
		Func: func(e *core.MailerRecordEvent) error {
			t.registerEventCall("OnMailerRecordOTPSend")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRealtimeConnectRequest().Bind(&hook.Handler[*core.RealtimeConnectRequestEvent]{
		Func: func(e *core.RealtimeConnectRequestEvent) error {
			t.registerEventCall("OnRealtimeConnectRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRealtimeMessageSend().Bind(&hook.Handler[*core.RealtimeMessageEvent]{
		Func: func(e *core.RealtimeMessageEvent) error {
			t.registerEventCall("OnRealtimeMessageSend")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRealtimeSubscribeRequest().Bind(&hook.Handler[*core.RealtimeSubscribeRequestEvent]{
		Func: func(e *core.RealtimeSubscribeRequestEvent) error {
			t.registerEventCall("OnRealtimeSubscribeRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnSettingsListRequest().Bind(&hook.Handler[*core.SettingsListRequestEvent]{
		Func: func(e *core.SettingsListRequestEvent) error {
			t.registerEventCall("OnSettingsListRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnSettingsUpdateRequest().Bind(&hook.Handler[*core.SettingsUpdateRequestEvent]{
		Func: func(e *core.SettingsUpdateRequestEvent) error {
			t.registerEventCall("OnSettingsUpdateRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnSettingsReload().Bind(&hook.Handler[*core.SettingsReloadEvent]{
		Func: func(e *core.SettingsReloadEvent) error {
			t.registerEventCall("OnSettingsReload")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnFileDownloadRequest().Bind(&hook.Handler[*core.FileDownloadRequestEvent]{
		Func: func(e *core.FileDownloadRequestEvent) error {
			t.registerEventCall("OnFileDownloadRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnFileTokenRequest().Bind(&hook.Handler[*core.FileTokenRequestEvent]{
		Func: func(e *core.FileTokenRequestEvent) error {
			t.registerEventCall("OnFileTokenRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordAuthRequest().Bind(&hook.Handler[*core.RecordAuthRequestEvent]{
		Func: func(e *core.RecordAuthRequestEvent) error {
			t.registerEventCall("OnRecordAuthRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordAuthWithPasswordRequest().Bind(&hook.Handler[*core.RecordAuthWithPasswordRequestEvent]{
		Func: func(e *core.RecordAuthWithPasswordRequestEvent) error {
			t.registerEventCall("OnRecordAuthWithPasswordRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordAuthWithOAuth2Request().Bind(&hook.Handler[*core.RecordAuthWithOAuth2RequestEvent]{
		Func: func(e *core.RecordAuthWithOAuth2RequestEvent) error {
			t.registerEventCall("OnRecordAuthWithOAuth2Request")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordAuthRefreshRequest().Bind(&hook.Handler[*core.RecordAuthRefreshRequestEvent]{
		Func: func(e *core.RecordAuthRefreshRequestEvent) error {
			t.registerEventCall("OnRecordAuthRefreshRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordRequestPasswordResetRequest().Bind(&hook.Handler[*core.RecordRequestPasswordResetRequestEvent]{
		Func: func(e *core.RecordRequestPasswordResetRequestEvent) error {
			t.registerEventCall("OnRecordRequestPasswordResetRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordConfirmPasswordResetRequest().Bind(&hook.Handler[*core.RecordConfirmPasswordResetRequestEvent]{
		Func: func(e *core.RecordConfirmPasswordResetRequestEvent) error {
			t.registerEventCall("OnRecordConfirmPasswordResetRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordRequestVerificationRequest().Bind(&hook.Handler[*core.RecordRequestVerificationRequestEvent]{
		Func: func(e *core.RecordRequestVerificationRequestEvent) error {
			t.registerEventCall("OnRecordRequestVerificationRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordConfirmVerificationRequest().Bind(&hook.Handler[*core.RecordConfirmVerificationRequestEvent]{
		Func: func(e *core.RecordConfirmVerificationRequestEvent) error {
			t.registerEventCall("OnRecordConfirmVerificationRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordRequestEmailChangeRequest().Bind(&hook.Handler[*core.RecordRequestEmailChangeRequestEvent]{
		Func: func(e *core.RecordRequestEmailChangeRequestEvent) error {
			t.registerEventCall("OnRecordRequestEmailChangeRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordConfirmEmailChangeRequest().Bind(&hook.Handler[*core.RecordConfirmEmailChangeRequestEvent]{
		Func: func(e *core.RecordConfirmEmailChangeRequestEvent) error {
			t.registerEventCall("OnRecordConfirmEmailChangeRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordRequestOTPRequest().Bind(&hook.Handler[*core.RecordCreateOTPRequestEvent]{
		Func: func(e *core.RecordCreateOTPRequestEvent) error {
			t.registerEventCall("OnRecordRequestOTPRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordAuthWithOTPRequest().Bind(&hook.Handler[*core.RecordAuthWithOTPRequestEvent]{
		Func: func(e *core.RecordAuthWithOTPRequestEvent) error {
			t.registerEventCall("OnRecordAuthWithOTPRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordsListRequest().Bind(&hook.Handler[*core.RecordsListRequestEvent]{
		Func: func(e *core.RecordsListRequestEvent) error {
			t.registerEventCall("OnRecordsListRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordViewRequest().Bind(&hook.Handler[*core.RecordRequestEvent]{
		Func: func(e *core.RecordRequestEvent) error {
			t.registerEventCall("OnRecordViewRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordCreateRequest().Bind(&hook.Handler[*core.RecordRequestEvent]{
		Func: func(e *core.RecordRequestEvent) error {
			t.registerEventCall("OnRecordCreateRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordUpdateRequest().Bind(&hook.Handler[*core.RecordRequestEvent]{
		Func: func(e *core.RecordRequestEvent) error {
			t.registerEventCall("OnRecordUpdateRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnRecordDeleteRequest().Bind(&hook.Handler[*core.RecordRequestEvent]{
		Func: func(e *core.RecordRequestEvent) error {
			t.registerEventCall("OnRecordDeleteRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionsListRequest().Bind(&hook.Handler[*core.CollectionsListRequestEvent]{
		Func: func(e *core.CollectionsListRequestEvent) error {
			t.registerEventCall("OnCollectionsListRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionViewRequest().Bind(&hook.Handler[*core.CollectionRequestEvent]{
		Func: func(e *core.CollectionRequestEvent) error {
			t.registerEventCall("OnCollectionViewRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionCreateRequest().Bind(&hook.Handler[*core.CollectionRequestEvent]{
		Func: func(e *core.CollectionRequestEvent) error {
			t.registerEventCall("OnCollectionCreateRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionUpdateRequest().Bind(&hook.Handler[*core.CollectionRequestEvent]{
		Func: func(e *core.CollectionRequestEvent) error {
			t.registerEventCall("OnCollectionUpdateRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionDeleteRequest().Bind(&hook.Handler[*core.CollectionRequestEvent]{
		Func: func(e *core.CollectionRequestEvent) error {
			t.registerEventCall("OnCollectionDeleteRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnCollectionsImportRequest().Bind(&hook.Handler[*core.CollectionsImportRequestEvent]{
		Func: func(e *core.CollectionsImportRequestEvent) error {
			t.registerEventCall("OnCollectionsImportRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	t.OnBatchRequest().Bind(&hook.Handler[*core.BatchRequestEvent]{
		Func: func(e *core.BatchRequestEvent) error {
			t.registerEventCall("OnBatchRequest")
			return e.Next()
		},
		Priority: -99999,
	})

	return t, nil
}

// TempDirClone creates a new temporary directory copy from the
// provided directory path.
//
// It is the caller's responsibility to call `os.RemoveAll(tempDir)`
// when the directory is no longer needed!
func TempDirClone(dirToClone string) (string, error) {
	tempDir, err := os.MkdirTemp("", "pb_test_*")
	if err != nil {
		return "", err
	}

	// copy everything from testDataDir to tempDir
	if err := copyDir(dirToClone, tempDir); err != nil {
		return "", err
	}

	return tempDir, nil
}

// -------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------

func copyDir(src string, dest string) error {
	if err := os.MkdirAll(dest, os.ModePerm); err != nil {
		return err
	}

	sourceDir, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceDir.Close()

	items, err := sourceDir.Readdir(-1)
	if err != nil {
		return err
	}

	for _, item := range items {
		fullSrcPath := filepath.Join(src, item.Name())
		fullDestPath := filepath.Join(dest, item.Name())

		var copyErr error
		if item.IsDir() {
			copyErr = copyDir(fullSrcPath, fullDestPath)
		} else {
			copyErr = copyFile(fullSrcPath, fullDestPath)
		}

		if copyErr != nil {
			return copyErr
		}
	}

	return nil
}

func copyFile(src string, dest string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return err
	}

	return nil
}

// generateSchemaName creates a unique PostgreSQL schema name for test isolation.
func generateSchemaName() string {
	id := schemaCounter.Add(1)
	return fmt.Sprintf("pb_test_%d_%d", os.Getpid(), id)
}

// createTestSchema creates a PostgreSQL schema for isolated test execution.
func createTestSchema(schemaName string) error {
	db, err := getSchemaInitDB()
	if err != nil {
		return err
	}

	_, err = db.NewQuery("CREATE SCHEMA IF NOT EXISTS " + schemaName).Execute()
	return err
}

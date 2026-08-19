package core

import (
	"fmt"
	"strings"
	"time"

	"github.com/arief-fajri/pgbase/tools/osutils"
	"github.com/fatih/color"
	"github.com/pocketbase/dbx"
	"github.com/spf13/cast"
)

var AppMigrations MigrationsList
var SystemMigrations MigrationsList

const DefaultMigrationsTable = "_migrations"

// migrationsAdvisoryLockKey is a fixed advisory lock key used by every
// migration runner so that concurrent processes applying migrations against
// the same Postgres database (eg. multiple app instances starting at once or
// the "-p 4" parallel test binaries sharing a test database) serialize their
// changes.
//
// PostgreSQL does not serialize concurrent CREATE/ALTER statements, so without
// this lock two processes can both pass the "_migrations" applied-check and
// then crash into a "duplicate key value violates unique constraint
// 'pg_type_typname_nsp_index'" error while creating the same table/type.
const migrationsAdvisoryLockKey = 11924226342963

// MigrationsRunner defines a simple struct for managing the execution of db migrations.
type MigrationsRunner struct {
	app            App
	tableName      string
	migrationsList MigrationsList
	inited         bool
}

// NewMigrationsRunner creates and initializes a new db migrations MigrationsRunner instance.
func NewMigrationsRunner(app App, migrationsList MigrationsList) *MigrationsRunner {
	return &MigrationsRunner{
		app:            app,
		migrationsList: migrationsList,
		tableName:      DefaultMigrationsTable,
	}
}

// Run interactively executes the current runner with the provided args.
//
// The following commands are supported:
// - up           - applies all migrations
// - down [n]     - reverts the last n (default 1) applied migrations
// - history-sync - syncs the migrations table with the runner's migrations list
func (r *MigrationsRunner) Run(args ...string) error {
	if err := r.initMigrationsTable(); err != nil {
		return err
	}

	cmd := "up"
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "up":
		applied, err := r.Up()
		if err != nil {
			return err
		}

		if len(applied) == 0 {
			color.Green("No new migrations to apply.")
		} else {
			for _, file := range applied {
				color.Green("Applied %s", file)
			}
		}

		return nil
	case "down":
		toRevertCount := 1
		if len(args) > 1 {
			toRevertCount = cast.ToInt(args[1])
			if toRevertCount < 0 {
				// revert all applied migrations
				toRevertCount = len(r.migrationsList.Items())
			}
		}

		names, err := r.lastAppliedMigrations(r.app.DB(), toRevertCount)
		if err != nil {
			return err
		}

		confirm := osutils.YesNoPrompt(fmt.Sprintf(
			"\n%v\nDo you really want to revert the last %d applied migration(s)?",
			strings.Join(names, "\n"),
			toRevertCount,
		), false)
		if !confirm {
			fmt.Println("The command has been cancelled")
			return nil
		}

		reverted, err := r.Down(toRevertCount)
		if err != nil {
			return err
		}

		if len(reverted) == 0 {
			color.Green("No migrations to revert.")
		} else {
			for _, file := range reverted {
				color.Green("Reverted %s", file)
			}
		}

		return nil
	case "history-sync":
		if err := r.RemoveMissingAppliedMigrations(); err != nil {
			return err
		}

		color.Green("The %s table was synced with the available migrations.", r.tableName)
		return nil
	default:
		return fmt.Errorf("unsupported command: %q", cmd)
	}
}

// Up executes all unapplied migrations for the provided runner.
//
// On success returns list with the applied migrations file names.
func (r *MigrationsRunner) Up() ([]string, error) {
	applied := []string{}

	err := r.runMigrationTx(func(txApp App) error {
		for _, m := range r.migrationsList.Items() {
			// applied migrations check
			if r.isMigrationApplied(txApp, m.File) {
				if m.ReapplyCondition == nil {
					continue // no need to reapply
				}

				shouldReapply, err := m.ReapplyCondition(txApp, r, m.File)
				if err != nil {
					return err
				}
				if !shouldReapply {
					continue
				}

				// clear previous history stored entry
				// (it will be recreated after successful execution)
				r.saveRevertedMigration(txApp, m.File)
			}

			// ignore empty Up action
			if m.Up != nil {
				if err := m.Up(txApp); err != nil {
					return fmt.Errorf("failed to apply migration %s: %w", m.File, err)
				}
			}

			if err := r.saveAppliedMigration(txApp, m.File); err != nil {
				return fmt.Errorf("failed to save applied migration info for %s: %w", m.File, err)
			}

			applied = append(applied, m.File)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return applied, nil
}

// Down reverts the last `toRevertCount` applied migrations
// (in the order they were applied).
//
// On success returns list with the reverted migrations file names.
func (r *MigrationsRunner) Down(toRevertCount int) ([]string, error) {
	reverted := make([]string, 0, toRevertCount)

	err := r.runMigrationTx(func(txApp App) error {
		names, appliedErr := r.lastAppliedMigrations(txApp.DB(), toRevertCount)
		if appliedErr != nil {
			return appliedErr
		}

		for _, name := range names {
			for _, m := range r.migrationsList.Items() {
				if m.File != name {
					continue
				}

				// revert limit reached
				if toRevertCount-len(reverted) <= 0 {
					return nil
				}

				// ignore empty Down action
				if m.Down != nil {
					if err := m.Down(txApp); err != nil {
						return fmt.Errorf("failed to revert migration %s: %w", m.File, err)
					}
				}

				if err := r.saveRevertedMigration(txApp, m.File); err != nil {
					return fmt.Errorf("failed to save reverted migration info for %s: %w", m.File, err)
				}

				reverted = append(reverted, m.File)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	return reverted, nil
}

// RemoveMissingAppliedMigrations removes the db entries of all applied migrations
// that are not listed in the runner's migrations list.
func (r *MigrationsRunner) RemoveMissingAppliedMigrations() error {
	loadedMigrations := r.migrationsList.Items()

	names := make([]any, len(loadedMigrations))
	for i, migration := range loadedMigrations {
		names[i] = migration.File
	}

	_, err := r.app.DB().Delete(r.tableName, dbx.Not(dbx.HashExp{
		"file": names,
	})).Execute()

	return err
}

func (r *MigrationsRunner) initMigrationsTable() error {
	if r.inited {
		return nil // already inited
	}

	if err := r.ensureMigrationsTable(r.app.DB()); err != nil {
		return err
	}

	r.inited = true

	return nil
}

// ensureMigrationsTable creates the migrations history table (if missing).
func (r *MigrationsRunner) ensureMigrationsTable(db dbx.Builder) error {
	rawQuery := fmt.Sprintf(
		`CREATE TABLE IF NOT EXISTS "%s" (file VARCHAR(255) PRIMARY KEY NOT NULL, applied BIGINT NOT NULL)`,
		r.tableName,
	)

	_, err := db.NewQuery(rawQuery).Execute()

	return err
}

// lockMigrations acquires a session-scoped advisory lock bound to the current
// (aux) transaction. It is automatically released when the transaction
// commits/rollbacks, and because the outer transaction spans the entire
// migration run (including the nested data transaction and any auxiliary DB
// DDL), it guarantees that only one process applies migrations to this
// database at a time.
func (r *MigrationsRunner) lockMigrations(txApp App) error {
	_, err := txApp.AuxDB().NewQuery(fmt.Sprintf(
		"SELECT pg_advisory_xact_lock(%d)",
		migrationsAdvisoryLockKey,
	)).Execute()

	return err
}

// runMigrationTx wraps a migration operation (apply/revert) in the standard
// nested aux+data transaction and serializes it against concurrent migrators
// via a PostgreSQL advisory lock.
func (r *MigrationsRunner) runMigrationTx(fn func(txApp App) error) error {
	return r.app.AuxRunInTransaction(func(txApp App) error {
		if err := r.lockMigrations(txApp); err != nil {
			return err
		}

		return txApp.RunInTransaction(func(txApp App) error {
			if err := r.ensureMigrationsTable(txApp.DB()); err != nil {
				return err
			}

			return fn(txApp)
		})
	})
}

func (r *MigrationsRunner) isMigrationApplied(txApp App, file string) bool {
	var exists int

	err := txApp.DB().Select("(1)").
		From(r.tableName).
		Where(dbx.HashExp{"file": file}).
		Limit(1).
		Row(&exists)

	return err == nil && exists > 0
}

func (r *MigrationsRunner) saveAppliedMigration(txApp App, file string) error {
	// Use microsecond precision to keep consistent ordering with any other
	// (microsecond) timestamps written into the same column.
	_, err := txApp.DB().Insert(r.tableName, dbx.Params{
		"file":    file,
		"applied": time.Now().UnixMicro(),
	}).Execute()

	return err
}

func (r *MigrationsRunner) saveRevertedMigration(txApp App, file string) error {
	_, err := txApp.DB().Delete(r.tableName, dbx.HashExp{"file": file}).Execute()

	return err
}

func (r *MigrationsRunner) lastAppliedMigrations(db dbx.Builder, limit int) ([]string, error) {
	var files = make([]string, 0, limit)

	loadedMigrations := r.migrationsList.Items()

	names := make([]any, len(loadedMigrations))
	for i, migration := range loadedMigrations {
		names[i] = migration.File
	}

	err := db.Select("file").
		From(r.tableName).
		Where(dbx.Not(dbx.HashExp{"applied": nil})).
		AndWhere(dbx.HashExp{"file": names}).
		OrderBy("applied DESC").
		AndOrderBy("file DESC").
		Limit(int64(limit)).
		Column(&files)

	if err != nil {
		return nil, err
	}

	return files, nil
}

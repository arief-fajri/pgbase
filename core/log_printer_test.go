package core

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/arief-fajri/pgbase/tools/list"
	"github.com/arief-fajri/pgbase/tools/logger"
	"github.com/arief-fajri/pgbase/tools/security"
	"github.com/pocketbase/dbx"
)

func TestBaseAppLoggerLevelDevPrint(t *testing.T) {
	testLogLevel := 4

	scenarios := []struct {
		name            string
		isDev           bool
		levels          []int
		printedLevels   []int
		persistedLevels []int
	}{
		{
			"dev mode",
			true,
			[]int{testLogLevel - 1, testLogLevel, testLogLevel + 1},
			[]int{testLogLevel - 1, testLogLevel, testLogLevel + 1},
			[]int{testLogLevel, testLogLevel + 1},
		},
		{
			"nondev mode",
			false,
			[]int{testLogLevel - 1, testLogLevel, testLogLevel + 1},
			[]int{},
			[]int{testLogLevel, testLogLevel + 1},
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			const testDataDir = "./pb_base_app_test_data_dir/"
			defer os.RemoveAll(testDataDir)

			app := NewBaseApp(BaseAppConfig{
				DataDir: testDataDir,
				IsDev:   s.isDev,
			})
			defer app.ResetBootstrapState()

			if err := app.Bootstrap(); err != nil {
				t.Fatal(err)
			}

			// silence query logs no longer needed (PostgreSQL connection handled by pool)

			app.Settings().Logs.MinLevel = testLogLevel
			if err := app.Save(app.Settings()); err != nil {
				t.Fatal(err)
			}

			var printedLevels []int

			ctx := context.Background()

			// track printed logs
			originalPrintLog := printLog
			defer func() {
				printLog = originalPrintLog
			}()
			printLog = func(log *logger.Log) {
				printedLevels = append(printedLevels, int(log.Level))
			}

			// use a unique message so the persisted rows can be isolated from
			// any other logs already present in the shared aux _logs table
			// (the batched multi-row insert writes raw rows and intentionally
			// no longer triggers the per-model create hooks)
			logMessage := "logtest_" + security.PseudorandomString(10)

			// write and persist logs
			for _, l := range s.levels {
				app.Logger().Log(ctx, slog.Level(l), logMessage)
			}
			handler, ok := app.Logger().Handler().(*logger.BatchHandler)
			if !ok {
				t.Fatalf("Expected BatchHandler, got %v", app.Logger().Handler())
			}
			if err := handler.WriteAll(ctx); err != nil {
				t.Fatalf("Failed to write all logs: %v", err)
			}

			// check persisted log levels (queried directly from the aux _logs
			// table since the batched insert bypasses the model create hooks)
			var persistedLevels []int
			err := app.AuxDB().
				NewQuery("SELECT [[level]] FROM " + LogsTableName + " WHERE [[message]]={:msg} ORDER BY [[level]] ASC").
				Bind(dbx.Params{"msg": logMessage}).
				Column(&persistedLevels)
			if err != nil {
				t.Fatalf("Failed to fetch persisted logs: %v", err)
			}

			// check persisted log levels
			if len(s.persistedLevels) != len(persistedLevels) {
				t.Fatalf("Expected persisted levels \n%v\ngot\n%v", s.persistedLevels, persistedLevels)
			}
			for _, l := range persistedLevels {
				if !list.ExistInSlice(l, s.persistedLevels) {
					t.Fatalf("Missing expected persisted level %v in %v", l, persistedLevels)
				}
			}

			// check printed log levels
			if len(s.printedLevels) != len(printedLevels) {
				t.Fatalf("Expected printed levels \n%v\ngot\n%v", s.printedLevels, printedLevels)
			}
			for _, l := range printedLevels {
				if !list.ExistInSlice(l, s.printedLevels) {
					t.Fatalf("Missing expected printed level %v in %v", l, printedLevels)
				}
			}
		})
	}
}

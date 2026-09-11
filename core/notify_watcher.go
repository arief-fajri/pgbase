package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/arief-fajri/pgbase/tools/hook"
	"github.com/arief-fajri/pgbase/tools/routine"
	"github.com/arief-fajri/pgbase/tools/security"
	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
)

const systemHookIdNotifyWatcher = "__pbNotifyWatcherSystemHook__"

func (app *BaseApp) registerNotifyWatcherHooks() {
	var notifyWatcher *fsnotify.Watcher

	instanceId := "@" + security.PseudorandomString(10)

	localNotifyDirPath := filepath.Join(app.DataDir(), LocalNotifyDirName)
	settingsFile := filepath.Join(localNotifyDirPath, "settings"+instanceId)
	collectionsFile := filepath.Join(localNotifyDirPath, "collections"+instanceId)

	// init
	app.OnBootstrap().Bind(&hook.Handler[*BootstrapEvent]{
		Id: systemHookIdNotifyWatcher,
		Func: func(e *BootstrapEvent) error {
			err := e.Next()
			if err != nil {
				return err
			}

			if notifyWatcher != nil {
				_ = notifyWatcher.Close()
			}

			notifyWatcher, err = createNotifyDirWatcher(e.App, instanceId, localNotifyDirPath)
			if err != nil {
				e.App.Logger().Warn("Notify dir watcher failure.", "error", err)
			}

			return nil
		},
		Priority: -998,
	})

	// cleanup
	app.OnTerminate().Bind(&hook.Handler[*TerminateEvent]{
		Id: systemHookIdNotifyWatcher,
		Func: func(e *TerminateEvent) error {
			if notifyWatcher != nil {
				_ = notifyWatcher.Close()
			}

			_ = os.Remove(settingsFile)
			_ = os.Remove(collectionsFile)

			return e.Next()
		},
		Priority: -998,
	})

	// ---------------------------------------------------------------

	settingsNotify := func(e *ModelEvent) error {
		err := e.Next()
		if err != nil || e.Model.PK() != paramsKeySettings {
			return err
		}

		if notifyWatcher != nil {
			if err := os.WriteFile(settingsFile, nil, 0644); err != nil {
				e.App.Logger().Warn("Failed to write watcher file", "error", err, "file", settingsFile)
			}
			// Keep the sentinel around briefly. macOS kqueue can otherwise
			// coalesce the create and remove events before other watchers see it.
			time.AfterFunc(250*time.Millisecond, func() {
				_ = os.Remove(settingsFile)
			})
		}

		return nil
	}
	app.OnModelAfterCreateSuccess(paramsTable).Bind(&hook.Handler[*ModelEvent]{
		Id:       systemHookIdNotifyWatcher,
		Func:     settingsNotify,
		Priority: 999,
	})
	app.OnModelAfterUpdateSuccess(paramsTable).Bind(&hook.Handler[*ModelEvent]{
		Id:       systemHookIdNotifyWatcher,
		Func:     settingsNotify,
		Priority: 999,
	})

	// ---------------------------------------------------------------

	collectionsNotify := func(e *CollectionEvent) error {
		if err := e.Next(); err != nil {
			return err
		}

		if notifyWatcher != nil {
			if err := os.WriteFile(collectionsFile, nil, 0644); err != nil {
				e.App.Logger().Warn("Failed to write watcher file", "error", err, "file", collectionsFile)
			}
			// Keep the sentinel around briefly. macOS kqueue can otherwise
			// coalesce the create and remove events into a single Remove
			// (which is ignored for collections) before other watchers see
			// the write, causing the reload to be missed entirely.
			time.AfterFunc(250*time.Millisecond, func() {
				_ = os.Remove(collectionsFile)
			})
		}

		return nil
	}
	app.OnCollectionAfterCreateSuccess().Bind(&hook.Handler[*CollectionEvent]{
		Id:       systemHookIdNotifyWatcher,
		Func:     collectionsNotify,
		Priority: 999,
	})
	app.OnCollectionAfterUpdateSuccess().Bind(&hook.Handler[*CollectionEvent]{
		Id:       systemHookIdNotifyWatcher,
		Func:     collectionsNotify,
		Priority: 999,
	})
	app.OnCollectionAfterDeleteSuccess().Bind(&hook.Handler[*CollectionEvent]{
		Id:       systemHookIdNotifyWatcher,
		Func:     collectionsNotify,
		Priority: 999,
	})
}

func createNotifyDirWatcher(app App, instanceId string, localNotifyDirPath string) (*fsnotify.Watcher, error) {
	// create the notify dir (if not already)
	err := os.MkdirAll(localNotifyDirPath, os.ModePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to create a notify dir: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to init notify dir watcher: %w", err)
	}

	err = watcher.Add(localNotifyDirPath)
	if err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("unable to watch notify dir: %w", err)
	}

	var debounceTimer *time.Timer

	stopDebounceTimer := func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
			debounceTimer = nil
		}
	}

	// watch
	routine.FireAndForget(func() {
		defer stopDebounceTimer()

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// modified from within the current app instance or cleanup event
				if strings.HasSuffix(event.Name, instanceId) || !app.IsBootstrapped() {
					continue
				}

				// On macOS (kqueue), a rapid Write+Remove can be coalesced into
				// a single Remove event. Allow settings Remove events through so
				// the settings reload still fires while preserving collection
				// notification debouncing.
				filename := filepath.Base(event.Name)
				isSettingsSentinel := strings.HasPrefix(filename, "settings@")
				if event.Has(fsnotify.Remove) && !isSettingsSentinel {
					continue
				}

				stopDebounceTimer()

				// The debounce window must comfortably span a burst of rapid
				// schema changes so they coalesce into a single reload. Unlike
				// SQLite, PostgreSQL DDL (CREATE/ALTER/DROP TABLE) can take tens
				// of milliseconds each, spacing consecutive notify events wider
				// apart, so a short window would fire mid-burst and reload twice.
				debounceTimer = time.AfterFunc(200*time.Millisecond, func() {
					filename := filepath.Base(event.Name)

					// settings changed
					if strings.HasPrefix(filename, "settings@") {
						app.Logger().Debug("Reloading settings after notify event")

						err := app.ReloadSettings()
						if err != nil {
							app.Logger().Warn("Failed to reload app settings after notify", "error", err)
						}
						return
					}

					// collections changed
					if strings.HasPrefix(filename, "collections@") {
						app.Logger().Debug("Reloading cached collections after notify event")

						err := app.ReloadCachedCollections()
						if err != nil {
							app.Logger().Warn("Failed to reload cached collections after notify", "error", err)
						}
						return
					}
				})
			case err, ok := <-watcher.Errors:
				if app.IsDev() && err != nil {
					color.Red("Notify dir watch error:", err)
				}

				if !ok {
					return
				}
			}
		}
	})

	return watcher, err
}

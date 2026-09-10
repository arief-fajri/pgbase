package core

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/arief-fajri/pgbase/tools/hook"
	"github.com/arief-fajri/pgbase/tools/routine"
	"github.com/arief-fajri/pgbase/tools/security"
	"github.com/pocketbase/dbx"
)

const instanceHeartbeatsTableName = "_instance_heartbeats"

const systemHookIdInstanceHeartbeat = "__pbInstanceHeartbeatSystemHook__"

// defaultInstanceHeartbeatInterval is how often each instance upserts its
// presence (see addendum #5.2a). The per-guard interval defaults to this and
// can be overridden in tests without sharing a mutable global.
const defaultInstanceHeartbeatInterval = 10 * time.Second

// instanceHeartbeatPeerWindow is how stale a peer's seen_at must be to be
// considered gone; 3x the heartbeat interval.
const instanceHeartbeatPeerWindow = 30 * time.Second

// instanceHeartbeatStaleRowAge is when an old row is considered garbage and
// cleaned up by the hourly cron (rows of dead instances).
const instanceHeartbeatStaleRowAge = time.Hour

const instanceHeartbeatsCleanupCronKey = "__pbInstanceHeartbeatsCleanup__"

// instanceHeartbeatGuard tracks multi-instance presence via a DB-plane
// heartbeat table and warns on the single -> multi transition (R-1b, spec
// #5.2a).
//
// It is read-only with respect to app behavior: it only detects peers and
// logs/warns. It never takes locks or elects leaders (that is the advisory
// lock split of #5.1).
type instanceHeartbeatGuard struct {
	app        *BaseApp
	instanceID string
	interval   time.Duration

	mu        sync.Mutex
	lastMulti bool
	stopCh    chan struct{}
	done      chan struct{} // closed when the background goroutine exits
}

func newInstanceHeartbeatGuard(app *BaseApp) *instanceHeartbeatGuard {
	return &instanceHeartbeatGuard{
		app:        app,
		instanceID: "@" + security.PseudorandomString(10),
		interval:   defaultInstanceHeartbeatInterval,
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// registerInstanceHeartbeatGuard wires the guard into the app lifecycle:
// heartbeat+detection ticker during bootstrap, stop on terminate, and an
// hourly stale-row cleanup cron job.
func (app *BaseApp) registerInstanceHeartbeatGuard() {
	guard := newInstanceHeartbeatGuard(app)
	app.instanceHeartbeatGuard = guard

	app.OnBootstrap().Bind(&hook.Handler[*BootstrapEvent]{
		Id:       systemHookIdInstanceHeartbeat,
		Func:     guard.init,
		Priority: -998,
	})

	app.OnTerminate().Bind(&hook.Handler[*TerminateEvent]{
		Id:       systemHookIdInstanceHeartbeat,
		Func:     guard.cleanup,
		Priority: -998,
	})

	app.Cron().MustAdd(instanceHeartbeatsCleanupCronKey, "2 * * * *", func() {
		err := guard.app.runWithCronLock(instanceHeartbeatsCleanupCronKey, app.Logger(), func() error {
			return guard.purgeStaleRows()
		})
		if err != nil {
			app.Logger().Warn("Failed to purge stale instance heartbeats", slog.String("error", err.Error()))
		}
	})
}

func (guard *instanceHeartbeatGuard) init(e *BootstrapEvent) error {
	if err := e.Next(); err != nil {
		return err
	}

	guard.mu.Lock()
	// allow re-bootstrap (the previous stopCh was closed by cleanup)
	if guard.stopCh == nil {
		guard.stopCh = make(chan struct{})
	}
	if guard.done == nil {
		guard.done = make(chan struct{})
	}
	stopCh := guard.stopCh
	done := guard.done
	guard.mu.Unlock()

	// write an initial presence so peers see us promptly, then tick
	if err := guard.writeHeartbeat(); err != nil {
		guard.app.Logger().Warn("Failed to write initial instance heartbeat", slog.String("error", err.Error()))
	}

	routine.FireAndForget(func() {
		defer close(done)

		ticker := time.NewTicker(guard.interval)
		defer ticker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-ticker.C:
				guard.runCycle()
			}
		}
	})

	return nil
}

func (guard *instanceHeartbeatGuard) cleanup(e *TerminateEvent) error {
	guard.mu.Lock()
	if guard.stopCh != nil {
		close(guard.stopCh)
		guard.stopCh = nil
	}
	done := guard.done
	guard.done = nil
	guard.mu.Unlock()

	// Wait for the background goroutine to exit before continuing the
	// terminate chain, so DB connections can be safely closed afterwards.
	if done != nil {
		<-done
	}

	return e.Next()
}

// runCycle writes this instance's heartbeat and detects multi-instance peers,
// warning (once per single->multi transition) when found.
func (guard *instanceHeartbeatGuard) runCycle() {
	// best-effort: a failing heartbeat must not crash the ticker
	if err := guard.writeHeartbeat(); err != nil {
		guard.app.Logger().Warn("Failed to write instance heartbeat", slog.String("error", err.Error()))
		return
	}

	multi, err := guard.hasPeer()
	if err != nil {
		guard.app.Logger().Warn("Failed to read instance heartbeat peers", slog.String("error", err.Error()))
		return
	}

	guard.mu.Lock()
	wasMulti := guard.lastMulti
	guard.lastMulti = multi
	guard.mu.Unlock()

	if multi && !wasMulti {
		guard.warnMultiInstance()
	}
}

// writeHeartbeat upserts this instance's presence (single cheap statement, works
// behind PgBouncer transaction pooling).
func (guard *instanceHeartbeatGuard) writeHeartbeat() error {
	_, err := guard.app.AuxNonconcurrentDB().
		NewQuery(fmt.Sprintf(
			`INSERT INTO "%s" ("instance_id", "seen_at") VALUES ({:id}, NOW())
			 ON CONFLICT ("instance_id") DO UPDATE SET "seen_at" = NOW()`,
			instanceHeartbeatsTableName,
		)).
		Bind(dbx.Params{"id": guard.instanceID}).
		Execute()

	return err
}

// hasPeer reports whether at least one OTHER live instance was seen within the
// peer window.
func (guard *instanceHeartbeatGuard) hasPeer() (bool, error) {
	var count int64

	err := guard.app.AuxNonconcurrentDB().
		NewQuery(fmt.Sprintf(
			`SELECT COUNT(*) FROM "%s"
			 WHERE "instance_id" <> {:self}
			   AND "seen_at" > NOW() - ({:window}::text)::interval`,
			instanceHeartbeatsTableName,
		)).
		Bind(dbx.Params{
			"self":   guard.instanceID,
			"window": instanceHeartbeatPeerWindow.String(),
		}).
		Row(&count)

	return count > 0, err
}

// warnMultiInstance surfaces a clear log warning once per single->multi
// transition, plus a best-effort superuser alert (requires SMTP).
func (guard *instanceHeartbeatGuard) warnMultiInstance() {
	guard.app.Logger().Warn(
		"Multiple app instances detected sharing the same database. Realtime broadcasts are per-instance until the cross-instance broadcaster is enabled; subscriptions on one instance will not receive record events written by another.",
		"instanceId", guard.instanceID,
	)

	// best-effort email to superusers; a missing/misconfigured SMTP must not
	// break the guard.
	if err := sendSystemAlertToAllSuperusers(
		guard.app,
		"Multi-instance deployment detected",
		"Multiple pgbase app instances are sharing the same database. Realtime record broadcasts are currently per-instance: subscriptions on one instance will not receive record events written by another instance until the cross-instance broadcaster is enabled.",
	); err != nil {
		guard.app.Logger().Debug("Failed to send multi-instance system alert", slog.String("error", err.Error()))
	}
}

// purgeStaleRows removes heartbeat rows of instances that have not reported
// within the stale age (ie. dead instances) to keep the table tiny.
func (guard *instanceHeartbeatGuard) purgeStaleRows() error {
	_, err := guard.app.AuxNonconcurrentDB().
		NewQuery(fmt.Sprintf(
			`DELETE FROM "%s" WHERE "seen_at" < NOW() - ({:age}::text)::interval`,
			instanceHeartbeatsTableName,
		)).
		Bind(dbx.Params{"age": instanceHeartbeatStaleRowAge.String()}).
		Execute()

	return err
}

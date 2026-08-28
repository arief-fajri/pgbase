package apis

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tools/hook"
)

// realtimeOutboxPollInterval bounds how long the listener waits between NOTIFY
// wake-ups before re-polling the outbox as a safety net (in case a NOTIFY was
// missed).
const realtimeOutboxPollInterval = 5 * time.Second

// realtimeOutboxBatchSize is how many pending events are processed per pass.
const realtimeOutboxBatchSize = 100

// realtimeOutboxReconnectDelay is the backoff before reconnecting a dropped
// LISTEN connection.
const realtimeOutboxReconnectDelay = 2 * time.Second

// realtimeOutboxListener consumes the cross-instance realtime outbox (D-4 step
// 2): it LISTENs for the NOTIFY wake-up, reads pending events, and re-broadcasts
// them to THIS instance's local subscribers (create/update re-fetch the record;
// delete uses the stored snapshot). It is the BROADCAST consumer side: it never
// ack-marks rows (broadcast queue - see core realtimeOutboxAckColumnNote), it
// only dedups in memory per event id.
type realtimeOutboxListener struct {
	app core.App

	stopOnce sync.Once
	stopCh   chan struct{}

	mu    sync.Mutex
	dedup map[string]struct{}
}

func newRealtimeOutboxListener(app core.App) *realtimeOutboxListener {
	return &realtimeOutboxListener{
		app:    app,
		stopCh: make(chan struct{}),
		dedup:  make(map[string]struct{}),
	}
}

// registerRealtimeOutboxListener wires the cross-instance listener into the
// app. It starts immediately (bindRealtimeApi runs after the app is
// bootstrapped in both the serve flow and the test scenario), and stops on
// terminate. When the outbox is disabled the goroutine parks until stop.
func registerRealtimeOutboxListener(app core.App) {
	listener := newRealtimeOutboxListener(app)

	go listener.run(app)

	app.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
		Id:       "__pbRealtimeOutboxListener__",
		Priority: -99,
		Func: func(e *core.TerminateEvent) error {
			listener.stop()
			return e.Next()
		},
	})
}

func (l *realtimeOutboxListener) stop() {
	l.stopOnce.Do(func() {
		close(l.stopCh)
	})
}

func (l *realtimeOutboxListener) waitOrStop(d time.Duration) bool {
	select {
	case <-l.stopCh:
		return false
	case <-time.After(d):
		return true
	}
}

// run owns the LISTEN connection for the process lifetime, reconnecting with a
// backoff on failure and re-polling pending events after reconnect (catchup).
func (l *realtimeOutboxListener) run(app core.App) {
	// the listener is a no-op when the outbox is disabled
	if !app.RealtimeOutboxEnabled() {
		<-l.stopCh
		return
	}

	// initial pass covers events published before we started listening
	l.processPending(app)

	// safety poll ticker (also serves as reconnect catchup re-armed below)
	pollTicker := time.NewTicker(realtimeOutboxPollInterval)
	defer pollTicker.Stop()

	for {
		conn, err := app.OpenRealtimeOutboxListener(context.Background())
		if err != nil {
			app.Logger().Warn("Failed to open realtime outbox listener", slog.String("error", err.Error()))
			if !l.waitOrStop(realtimeOutboxReconnectDelay) {
				return
			}
			continue
		}

		if _, err := conn.Exec(context.Background(), "LISTEN "+core.RealtimeOutboxNotifyChannel()); err != nil {
			conn.Close(context.Background())
			app.Logger().Warn("Failed to LISTEN on realtime outbox", slog.String("error", err.Error()))
			if !l.waitOrStop(realtimeOutboxReconnectDelay) {
				return
			}
			continue
		}

		// catchup: reprocess pending rows that may have been missed while down
		l.processPending(app)

		// wake-up / poll loop on this connection
	reconnect:
		for {
			select {
			case <-l.stopCh:
				conn.Close(context.Background())
				return
			case <-pollTicker.C:
				l.processPending(app)
			default:
			}

			ctx, cancel := context.WithTimeout(context.Background(), realtimeOutboxPollInterval)
			n, err := conn.WaitForNotification(ctx)
			cancel()

			if err == nil {
				_ = n // channel name only; all data is read from the table
				l.processPending(app)
				continue
			}

			// timeout -> poll as a safety net; other errors -> reconnect
			if ctx.Err() == context.DeadlineExceeded {
				l.processPending(app)
				continue
			}

			break reconnect
		}

		conn.Close(context.Background())

		if !l.waitOrStop(realtimeOutboxReconnectDelay) {
			return
		}
	}
}

// processPending reads pending events and re-broadcasts them locally. Each
// event is remembered only in the in-memory dedup (broadcast queue semantics).
func (l *realtimeOutboxListener) processPending(app core.App) {
	events, err := app.RealtimeOutboxPendingEvents(realtimeOutboxBatchSize)
	if err != nil {
		app.Logger().Warn("Failed to read realtime outbox events", slog.String("error", err.Error()))
		return
	}

	for _, e := range events {
		if !l.dedupSeen(e.Id) {
			l.processEvent(app, e)
		}
	}
}

func (l *realtimeOutboxListener) dedupSeen(id string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if _, ok := l.dedup[id]; ok {
		return true
	}
	l.dedup[id] = struct{}{}
	return false
}

// processEvent re-broadcasts a single event to this instance's subscribers.
func (l *realtimeOutboxListener) processEvent(app core.App, e core.RealtimeOutboxEvent) {
	switch e.Action {
	case core.RealtimeActionDelete:
		l.processDelete(app, e)
	default: // create / update
		l.processCreateUpdate(app, e)
	}
}

// processCreateUpdate re-broadcasts by re-fetching the current record state
// (LWW is automatic: the DB holds the latest committed state).
func (l *realtimeOutboxListener) processCreateUpdate(app core.App, e core.RealtimeOutboxEvent) {
	record, err := app.FindRecordById(e.Collection, e.RecordId)
	if err != nil {
		// record gone (a later delete overtook) or collection removed -> skip
		app.Logger().Debug("Failed to re-fetch realtime outbox record; skipping", "collection", e.Collection, "recordId", e.RecordId, "error", err.Error())
		return
	}

	if err := realtimeBroadcastRecord(app, e.Action, record, false); err != nil {
		app.Logger().Debug("Failed to re-broadcast realtime create/update", "recordId", e.RecordId, "error", err.Error())
	}
}

// processDelete re-broadcasts a delete event from its pre-delete snapshot
// (re-fetch is impossible after the delete commits). It dry-caches the delete
// messages then flushes them, mirroring the single-instance delete path.
func (l *realtimeOutboxListener) processDelete(app core.App, e core.RealtimeOutboxEvent) {
	collection, err := app.FindCollectionByNameOrId(e.Collection)
	if err != nil {
		app.Logger().Debug("Failed to resolve collection for realtime delete", "collection", e.Collection, "error", err.Error())
		return
	}

	// the snapshot JSON lacks the collection model; wire an empty record to the
	// collection so UnmarshalJSON can set fields
	record := core.NewRecord(collection)

	if err := json.Unmarshal(e.Snapshot, record); err != nil {
		app.Logger().Debug("Failed to decode realtime delete snapshot", "recordId", e.RecordId, "error", err.Error())
		return
	}

	if err := realtimeBroadcastRecord(app, core.RealtimeActionDelete, record, true); err != nil {
		app.Logger().Debug("Failed to dry-cache realtime delete", "recordId", e.RecordId, "error", err.Error())
		return
	}

	if err := realtimeBroadcastDryCacheKey(app, getDryCacheKey(core.RealtimeActionDelete, record)); err != nil {
		app.Logger().Debug("Failed to flush realtime delete cache", "recordId", e.RecordId, "error", err.Error())
	}
}

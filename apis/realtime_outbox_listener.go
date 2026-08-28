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
// delete uses the stored snapshot).
//
// It tracks a per-instance forward (created,id) cursor rather than an unbounded
// in-memory "seen" set: each pass reads only events after the cursor and
// advances it, so the read window always moves forward (bounded memory, and it
// never stalls once the backlog exceeds one batch).
type realtimeOutboxListener struct {
	app core.App

	stopOnce sync.Once
	stopCh   chan struct{}

	mu            sync.Mutex
	cursorCreated time.Time
	cursorId      string
	cursorReady   bool
}

func newRealtimeOutboxListener(app core.App) *realtimeOutboxListener {
	return &realtimeOutboxListener{
		app:    app,
		stopCh: make(chan struct{}),
	}
}

// registerRealtimeOutboxListener wires the cross-instance listener into the
// app. It starts immediately (bindRealtimeApi runs after the app is
// bootstrapped in both the serve flow and the test scenario), and stops on
// terminate. When the outbox is disabled the goroutine parks until stop.
func registerRealtimeOutboxListener(app core.App) {
	listener := newRealtimeOutboxListener(app)

	// Seed the forward cursor to the current tail synchronously here (before
	// the listener goroutine starts) so a freshly started instance forwards
	// only events published AFTER start. Doing it at registration rather than
	// inside run() closes a startup race: an event published right after boot
	// could otherwise land at/behind a not-yet-seeded cursor and be skipped.
	// run() still calls seedCursorIfNeeded as a fallback (guarded by
	// cursorReady) in case this initial seed failed on a transient DB error.
	if app.RealtimeOutboxEnabled() {
		listener.seedCursorIfNeeded(app)
	}

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

		// Seed the cursor to the current tail on the FIRST successful LISTEN so
		// a freshly started instance forwards only NEW events (not up to the
		// whole retention window of history to clients that just connected). On
		// later reconnects the cursor is kept, so we catch up on anything
		// published while the connection was down.
		l.seedCursorIfNeeded(app)

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

// seedCursorIfNeeded initializes the forward cursor to the current outbox tail
// the first time it is called, so a freshly started instance skips history.
func (l *realtimeOutboxListener) seedCursorIfNeeded(app core.App) {
	l.mu.Lock()
	already := l.cursorReady
	l.mu.Unlock()
	if already {
		return
	}

	created, id, err := app.RealtimeOutboxTailCursor()
	if err != nil {
		app.Logger().Warn("Failed to seed realtime outbox cursor", slog.String("error", err.Error()))
		return
	}

	l.mu.Lock()
	// guard against a racing reconnect having seeded already
	if !l.cursorReady {
		l.cursorCreated = created
		l.cursorId = id
		l.cursorReady = true
	}
	l.mu.Unlock()
}

// processPending pages through all settled events after the current cursor and
// re-broadcasts them locally, advancing the cursor as it goes. It drains the
// backlog in batches so it never stalls once more than one batch is pending.
func (l *realtimeOutboxListener) processPending(app core.App) {
	for {
		l.mu.Lock()
		afterCreated, afterId := l.cursorCreated, l.cursorId
		l.mu.Unlock()

		events, err := app.RealtimeOutboxEventsAfter(afterCreated, afterId, realtimeOutboxBatchSize)
		if err != nil {
			app.Logger().Warn("Failed to read realtime outbox events", slog.String("error", err.Error()))
			return
		}
		if len(events) == 0 {
			return
		}

		for _, e := range events {
			l.processEvent(app, e)
		}

		last := events[len(events)-1]
		l.mu.Lock()
		l.cursorCreated = last.Created
		l.cursorId = last.Id
		l.mu.Unlock()

		if len(events) < realtimeOutboxBatchSize {
			return // drained
		}
	}
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

package core

import (
	"sync"
	"time"

	"github.com/arief-fajri/pgbase/tools/routine"
	"github.com/pocketbase/dbx"
)

// auditReadWriterStoreKey is the app-store key under which each app's
// read-audit writer is stored (see registerAuditHooks). Keying by the app
// store (which is stable across ResetBootstrapState) keeps the writer
// per-app/per-DB so parallel tests never cross-write into another app's DB.
const auditReadWriterStoreKey = "__pbAuditReadWriter__"

const (
	// auditReadBufferSize is the capacity of the buffered enqueue channel.
	// Beyond this, events are dropped (best-effort — a read must never block).
	auditReadBufferSize = 500

	// auditReadFlushSize is the max number of rows written per transaction.
	auditReadFlushSize = 200

	// auditReadFlushInterval is the periodic ticker flush interval (prod only;
	// the ticker is started on OnServe, so tests flush explicitly).
	auditReadFlushInterval = 3 * time.Second
)

// auditReadWriter is a per-app batched writer for the read (view/list) audit
// trail. Events are enqueued on a buffered channel and flushed into one MAIN-db
// transaction either periodically (ticker) or on demand via Flush. Mirrors the
// log BatchHandler but targets the MAIN pool via RunInTransaction.
type auditReadWriter struct {
	app  App
	buf  chan *AuditRead
	done chan struct{}

	flushMu   sync.Mutex
	startOnce sync.Once
	stopOnce  sync.Once
}

func newAuditReadWriter(app App) *auditReadWriter {
	return &auditReadWriter{
		app:  app,
		buf:  make(chan *AuditRead, auditReadBufferSize),
		done: make(chan struct{}),
	}
}

// enqueue adds a read-audit event to the buffer without blocking. If the buffer
// is full the event is dropped (best-effort — a read must never block or fail).
func (w *auditReadWriter) enqueue(r *AuditRead) {
	select {
	case w.buf <- r:
	default:
		w.app.Logger().Warn("audit_read buffer full, dropping event",
			"collection", r.CollectionName, "event", r.Event)
	}
}

// start launches the periodic flush ticker goroutine (idempotent). It is bound
// to OnServe so that tests (which never Serve) do not spawn the goroutine and
// instead drain deterministically via Flush.
func (w *auditReadWriter) start() {
	w.startOnce.Do(func() {
		routine.FireAndForget(func() {
			ticker := time.NewTicker(auditReadFlushInterval)
			defer ticker.Stop()

			for {
				select {
				case <-w.done:
					return
				case <-ticker.C:
					w.Flush()
				}
			}
		})
	})
}

// stop drains buffered events and signals the ticker goroutine to exit
// (idempotent).
func (w *auditReadWriter) stop() {
	w.Flush()
	w.stopOnce.Do(func() {
		close(w.done)
	})
}

// Flush drains all currently buffered events and writes them in batched MAIN-db
// transactions. Errors are logged, never propagated (best-effort). Exported for
// deterministic draining in tests (do not poll/sleep).
func (w *auditReadWriter) Flush() {
	w.flushMu.Lock()
	defer w.flushMu.Unlock()

	batch := make([]*AuditRead, 0, auditReadFlushSize)

	for {
		select {
		case r := <-w.buf:
			batch = append(batch, r)
			if len(batch) >= auditReadFlushSize {
				w.writeBatch(batch)
				batch = batch[:0]
			}
		default:
			if len(batch) > 0 {
				w.writeBatch(batch)
			}
			return
		}
	}
}

func (w *auditReadWriter) writeBatch(batch []*AuditRead) {
	// nothing to write or the app is not (yet) usable
	if len(batch) == 0 || !w.app.IsBootstrapped() {
		return
	}

	err := w.app.RunInTransaction(func(txApp App) error {
		for _, r := range batch {
			params := dbx.Params{
				"collection_name": r.CollectionName,
				"record_id":       r.RecordId,
				"event":           r.Event,
				"auth_id":         r.AuthId,
				"auth_collection": r.AuthCollection,
				"source":          r.Source,
				"filter":          r.Filter,
				"sort":            r.Sort,
				"page":            r.Page,
				"per_page":        r.PerPage,
				"total_items":     r.TotalItems,
				"user_ip":         r.UserIP,
				"user_agent":      r.UserAgent,
				// "created" omitted → SQL DEFAULT NOW()
			}

			if _, err := txApp.NonconcurrentDB().Insert(AuditReadsTableName, params).Execute(); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		w.app.Logger().Error("failed to flush audit_read batch", "error", err, "count", len(batch))
	}
}

// auditReadWriterFor returns the read-audit writer stored on the given app (or
// nil if audit hooks were never registered for it).
func auditReadWriterFor(app App) *auditReadWriter {
	w, _ := app.Store().Get(auditReadWriterStoreKey).(*auditReadWriter)
	return w
}

// FlushAuditReads synchronously drains and persists any pending read-audit
// events for the given app. Intended for deterministic draining in tests
// (production relies on the OnServe ticker). No-op if audit hooks are not
// registered for the app.
func FlushAuditReads(app App) {
	if w := auditReadWriterFor(app); w != nil {
		w.Flush()
	}
}

package core

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
)

func newRealtimeOutboxTestApp(t *testing.T) *BaseApp {
	t.Helper()

	dataDir, err := os.MkdirTemp("", "pb_outbox*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	app := NewBaseApp(BaseAppConfig{
		DataDir:          dataDir,
		IsDev:            true,
		DataMaxOpenConns: 2,
		AuxMaxOpenConns:  2,
	})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("failed to bootstrap test app: %v", err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })

	return app
}

func outboxCount(t *testing.T, app *BaseApp) int64 {
	t.Helper()
	var count int64
	err := app.DB().NewQuery(`SELECT COUNT(*) FROM "_realtime_outbox"`).Row(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

// TestRealtimeOutboxDisabledIsNoop verifies the default (off) writes nothing.
func TestRealtimeOutboxDisabledIsNoop(t *testing.T) {
	t.Setenv("PB_REALTIME_OUTBOX", "")

	app := newRealtimeOutboxTestApp(t)

	// clean table first (raw shared-DB path, rows may linger across runs)
	if _, err := app.DB().NewQuery(`DELETE FROM "_realtime_outbox"`).Execute(); err != nil {
		t.Fatal(err)
	}

	if app.realtimeOutboxEnabled() {
		t.Fatal("expected the realtime outbox to be disabled by default")
	}

	if err := app.PublishRealtimeEvent(RealtimeActionCreate, "demo1", "rec1", nil); err != nil {
		t.Fatalf("expected no-op publish to succeed: %v", err)
	}
	if got := outboxCount(t, app); got != 0 {
		t.Fatalf("expected no outbox rows when disabled, got %d", got)
	}
}

// TestRealtimeOutboxPublishCreateAndDelete verifies the publisher writes
// identifiers for create and a full snapshot for delete.
func TestRealtimeOutboxPublishCreateAndDelete(t *testing.T) {
	t.Setenv("PB_REALTIME_OUTBOX", "1")

	app := newRealtimeOutboxTestApp(t)

	// clean table first (raw shared-DB path)
	if _, err := app.DB().NewQuery(`DELETE FROM "_realtime_outbox"`).Execute(); err != nil {
		t.Fatal(err)
	}

	if !app.realtimeOutboxEnabled() {
		t.Fatal("expected the realtime outbox to be enabled")
	}

	// create/update: identifiers only, no snapshot
	if err := app.PublishRealtimeEvent(RealtimeActionCreate, "demo1", "rec-create", nil); err != nil {
		t.Fatal(err)
	}
	if err := app.PublishRealtimeEvent(RealtimeActionUpdate, "demo1", "rec-update", nil); err != nil {
		t.Fatal(err)
	}

	// delete: full snapshot serialized (collection with a title field)
	coll := NewBaseCollection("demo1")
	coll.Id = "pbc_demo1"
	coll.Fields.Add(&TextField{Name: "title"})
	rec := NewRecord(coll)
	rec.Id = "rec-delete"
	rec.SetRaw("title", "to be deleted")
	if err := app.PublishRealtimeEvent(RealtimeActionDelete, "demo1", "rec-delete", rec); err != nil {
		t.Fatal(err)
	}

	if got := outboxCount(t, app); got != 3 {
		t.Fatalf("expected 3 outbox rows, got %d", got)
	}

	rows := []struct {
		Action     string `db:"action"`
		Collection string `db:"collection"`
		RecordId   string `db:"record_id"`
		Snapshot   string `db:"snapshot"`
	}{}
	if err := app.DB().NewQuery(`SELECT action, collection, record_id, COALESCE(snapshot::text,'') AS snapshot FROM "_realtime_outbox" ORDER BY "created"`).All(&rows); err != nil {
		t.Fatal(err)
	}

	// create has no snapshot
	if rows[0].Action != RealtimeActionCreate || rows[0].RecordId != "rec-create" || rows[0].Snapshot != "" {
		t.Fatalf("unexpected create row: %+v", rows[0])
	}
	// update has no snapshot
	if rows[1].Action != RealtimeActionUpdate || rows[1].RecordId != "rec-update" || rows[1].Snapshot != "" {
		t.Fatalf("unexpected update row: %+v", rows[1])
	}
	// delete carries the snapshot JSON including the record id/title
	if rows[2].Action != RealtimeActionDelete || rows[2].RecordId != "rec-delete" {
		t.Fatalf("unexpected delete row: %+v", rows[2])
	}
	if !strings.Contains(rows[2].Snapshot, `"id": "rec-delete"`) || !strings.Contains(rows[2].Snapshot, `"title": "to be deleted"`) {
		t.Fatalf("expected the delete snapshot to carry the full record, got %q", rows[2].Snapshot)
	}
}

// TestRealtimeOutboxDeleteSnapshotScrubsAuthSecrets verifies a delete snapshot
// for an auth collection never persists password or tokenKey (PGB-N02): those
// secrets are stored at rest in the outbox table and are included in backups.
func TestRealtimeOutboxDeleteSnapshotScrubsAuthSecrets(t *testing.T) {
	t.Setenv("PB_REALTIME_OUTBOX", "1")

	app := newRealtimeOutboxTestApp(t)

	// clean table first (raw shared-DB path)
	if _, err := app.DB().NewQuery(`DELETE FROM "_realtime_outbox"`).Execute(); err != nil {
		t.Fatal(err)
	}

	coll := NewBaseCollection("users")
	coll.Id = "pbc_users"
	coll.Type = CollectionTypeAuth
	coll.Fields.Add(&TextField{Name: "title"})
	rec := NewRecord(coll)
	rec.Id = "rec-auth"
	rec.SetRaw("title", "public title")
	rec.SetRaw(FieldNamePassword, "$2a$10$abcdefghijklmnopqrstuv")
	rec.SetRaw(FieldNameTokenKey, "super-secret-token-key")

	raw := sanitizeOutboxSnapshot(rec)

	for _, secret := range []string{FieldNamePassword, FieldNameTokenKey, "$2a$10$", "super-secret-token-key"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("expected outbox snapshot to scrub %q, got: %s", secret, raw)
		}
	}

	if !strings.Contains(string(raw), "public title") {
		t.Fatalf("expected non-sensitive fields to remain in the snapshot, got: %s", raw)
	}
}

// TestRealtimeOutboxCleanup verifies processed and stale rows are removed.
func TestRealtimeOutboxCleanup(t *testing.T) {
	t.Setenv("PB_REALTIME_OUTBOX", "1")

	app := newRealtimeOutboxTestApp(t)

	// clean table first (raw shared-DB path)
	if _, err := app.DB().NewQuery(`DELETE FROM "_realtime_outbox"`).Execute(); err != nil {
		t.Fatal(err)
	}

	// one processed row, one stale row, one fresh pending row
	_, err := app.DB().NewQuery(`
		INSERT INTO "_realtime_outbox" ("id", "action", "collection", "record_id", "snapshot", "created", "processed_at") VALUES
			('1', 'create', 'c', 'r1', NULL, NOW(), NOW() - interval '1 hour'),
			('2', 'create', 'c', 'r2', NULL, NOW() - interval '2 days', NULL),
			('3', 'create', 'c', 'r3', NULL, NOW(), NULL)
	`).Execute()
	if err != nil {
		t.Fatal(err)
	}

	if err := app.CleanupStaleRealtimeEvents(24 * time.Hour); err != nil {
		t.Fatal(err)
	}

	rows := []struct {
		Id string `db:"id"`
	}{}
	if err := app.DB().NewQuery(`SELECT "id" FROM "_realtime_outbox"`).All(&rows); err != nil {
		t.Fatal(err)
	}

	if len(rows) != 1 || rows[0].Id != "3" {
		t.Fatalf("expected only the fresh pending row to survive cleanup, got %+v", rows)
	}
}

// TestRealtimeOutboxEventsAfterCursorPaging inserts more than one batch of
// settled rows and pages through them with the forward cursor, proving the old
// stuck-100-window bug is fixed: every row is returned exactly once in
// (created, id) order, even past the first batch.
func TestRealtimeOutboxEventsAfterCursorPaging(t *testing.T) {
	t.Setenv("PB_REALTIME_OUTBOX", "1")

	app := newRealtimeOutboxTestApp(t)

	if _, err := app.DB().NewQuery(`DELETE FROM "_realtime_outbox"`).Execute(); err != nil {
		t.Fatal(err)
	}

	// 250 settled rows (created well in the past so the stability lag passes),
	// with monotonic created + zero-padded ids so (created, id) order is stable.
	_, err := app.DB().NewQuery(`
		INSERT INTO "_realtime_outbox" ("id", "action", "collection", "record_id", "snapshot", "created")
		SELECT lpad(g::text, 6, '0'), 'create', 'c', 'r' || g, NULL,
			NOW() - interval '1 hour' + (g * interval '1 millisecond')
		FROM generate_series(1, 250) g
	`).Execute()
	if err != nil {
		t.Fatal(err)
	}

	const batch = 100
	var afterCreated time.Time
	var afterId string
	seen := make(map[string]struct{})
	var prevCreated time.Time
	prevId := ""

	for pass := 0; pass < 10; pass++ {
		events, err := app.RealtimeOutboxEventsAfter(afterCreated, afterId, batch)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) == 0 {
			break
		}
		for _, e := range events {
			if _, dup := seen[e.Id]; dup {
				t.Fatalf("event %q returned twice", e.Id)
			}
			seen[e.Id] = struct{}{}

			// strictly ascending (created, id)
			if !prevCreated.IsZero() {
				if e.Created.Before(prevCreated) || (e.Created.Equal(prevCreated) && e.Id <= prevId) {
					t.Fatalf("events out of order: (%s,%s) after (%s,%s)", e.Created, e.Id, prevCreated, prevId)
				}
			}
			prevCreated, prevId = e.Created, e.Id
		}
		last := events[len(events)-1]
		afterCreated, afterId = last.Created, last.Id
	}

	if len(seen) != 250 {
		t.Fatalf("expected to page through all 250 settled rows, saw %d", len(seen))
	}
}

// TestRealtimeOutboxEventsAfterStabilityLag verifies rows younger than the
// stability lag are withheld so a slightly-later commit can't be skipped by the
// forward cursor.
func TestRealtimeOutboxEventsAfterStabilityLag(t *testing.T) {
	t.Setenv("PB_REALTIME_OUTBOX", "1")

	app := newRealtimeOutboxTestApp(t)

	if _, err := app.DB().NewQuery(`DELETE FROM "_realtime_outbox"`).Execute(); err != nil {
		t.Fatal(err)
	}

	// one settled row (older than the lag) and one fresh row (within the lag)
	_, err := app.DB().NewQuery(`
		INSERT INTO "_realtime_outbox" ("id", "action", "collection", "record_id", "snapshot", "created") VALUES
			('old', 'create', 'c', 'r-old', NULL, NOW() - interval '10 seconds'),
			('new', 'create', 'c', 'r-new', NULL, NOW())
	`).Execute()
	if err != nil {
		t.Fatal(err)
	}

	events, err := app.RealtimeOutboxEventsAfter(time.Time{}, "", 100)
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 1 || events[0].Id != "old" {
		t.Fatalf("expected only the settled 'old' row (fresh row within lag withheld), got %+v", events)
	}
}

// TestRealtimeOutboxTailCursor verifies the tail cursor is empty on an empty
// table and points at the newest (created, id) once rows exist.
func TestRealtimeOutboxTailCursor(t *testing.T) {
	t.Setenv("PB_REALTIME_OUTBOX", "1")

	app := newRealtimeOutboxTestApp(t)

	if _, err := app.DB().NewQuery(`DELETE FROM "_realtime_outbox"`).Execute(); err != nil {
		t.Fatal(err)
	}

	// empty table -> zero cursor
	created, id, err := app.RealtimeOutboxTailCursor()
	if err != nil {
		t.Fatal(err)
	}
	if !created.IsZero() || id != "" {
		t.Fatalf("expected a zero cursor on empty table, got (%s, %q)", created, id)
	}

	// newest row is 'c' (latest created)
	_, err = app.DB().NewQuery(`
		INSERT INTO "_realtime_outbox" ("id", "action", "collection", "record_id", "snapshot", "created") VALUES
			('a', 'create', 'c', 'r-a', NULL, NOW() - interval '2 seconds'),
			('b', 'create', 'c', 'r-b', NULL, NOW() - interval '1 second'),
			('c', 'create', 'c', 'r-c', NULL, NOW())
	`).Execute()
	if err != nil {
		t.Fatal(err)
	}

	created, id, err = app.RealtimeOutboxTailCursor()
	if err != nil {
		t.Fatal(err)
	}
	if id != "c" {
		t.Fatalf("expected the tail cursor to point at the newest row 'c', got %q", id)
	}
	if created.IsZero() {
		t.Fatal("expected a non-zero created for the tail cursor")
	}
}

// TestRealtimeOutboxEventsAfterExcludesOwnOrigin proves the double-broadcast fix:
// rows stamped with THIS instance's origin are withheld from the reader (the
// publisher already broadcast them to its local clients synchronously), while
// foreign-origin and legacy NULL-origin rows are still delivered.
func TestRealtimeOutboxEventsAfterExcludesOwnOrigin(t *testing.T) {
	t.Setenv("PB_REALTIME_OUTBOX", "1")

	app := newRealtimeOutboxTestApp(t)

	if _, err := app.DB().NewQuery(`DELETE FROM "_realtime_outbox"`).Execute(); err != nil {
		t.Fatal(err)
	}

	// three settled rows: our own origin, a foreign instance, and a legacy
	// NULL origin. Only the foreign + legacy rows should come back.
	_, err := app.DB().NewQuery(`
		INSERT INTO "_realtime_outbox" ("id", "action", "collection", "record_id", "snapshot", "created", "origin") VALUES
			('self',    'create', 'c', 'r-self',    NULL, NOW() - interval '10 seconds', {:self}),
			('foreign', 'create', 'c', 'r-foreign', NULL, NOW() - interval '9 seconds',  '@other-instance'),
			('legacy',  'create', 'c', 'r-legacy',  NULL, NOW() - interval '8 seconds',  NULL)
	`).Bind(dbx.Params{"self": app.realtimeOutboxOrigin}).Execute()
	if err != nil {
		t.Fatal(err)
	}

	events, err := app.RealtimeOutboxEventsAfter(time.Time{}, "", 100)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, e := range events {
		seen[e.Id] = true
	}
	if seen["self"] {
		t.Fatal("expected the reader to skip its own-origin row, but it was returned")
	}
	if !seen["foreign"] || !seen["legacy"] {
		t.Fatalf("expected foreign + legacy(NULL)-origin rows to be delivered, got %+v", events)
	}
}

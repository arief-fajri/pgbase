package core

import (
	"os"
	"strings"
	"testing"
	"time"
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

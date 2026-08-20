package core_test

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
)

// auditTestApp returns a test app with the write audit trail enabled for the
// provided collections.
func auditTestApp(t *testing.T, collections ...string) *tests.TestApp {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}

	s := app.Settings()
	s.Audit.Enabled = true
	s.Audit.Collections = collections

	return app
}

func auditsForRecord(t *testing.T, app core.App, recordId string) []*core.Audit {
	t.Helper()

	audits := []*core.Audit{}
	err := app.AuditQuery().
		AndWhere(dbx.HashExp{"record_id": recordId}).
		OrderBy("created ASC").
		All(&audits)
	if err != nil {
		t.Fatalf("failed to query audits: %v", err)
	}

	return audits
}

func auditsCount(t *testing.T, app core.App, where dbx.Expression) int {
	t.Helper()

	var n int
	q := app.AuditQuery().Select("COUNT(*)")
	if where != nil {
		q.AndWhere(where)
	}
	if err := q.Row(&n); err != nil {
		t.Fatalf("failed to count audits: %v", err)
	}

	return n
}

func TestAuditWriteCreate(t *testing.T) {
	t.Parallel()

	app := auditTestApp(t, "demo1")
	defer app.Cleanup()

	col, err := app.FindCollectionByNameOrId("demo1")
	if err != nil {
		t.Fatal(err)
	}

	rec := core.NewRecord(col)
	rec.Set("text", "created-value")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	audits := auditsForRecord(t, app, rec.Id)
	if len(audits) != 1 {
		t.Fatalf("expected 1 audit, got %d", len(audits))
	}

	a := audits[0]
	if a.Event != "create" {
		t.Fatalf("expected event=create, got %q", a.Event)
	}
	if a.Source != "system" {
		t.Fatalf("expected source=system (no request), got %q", a.Source)
	}
	if a.CollectionName != "demo1" {
		t.Fatalf("expected collection_name=demo1, got %q", a.CollectionName)
	}
	if got := a.Snapshot["text"]; got != "created-value" {
		t.Fatalf("expected snapshot.text=created-value, got %v", got)
	}
	if len(a.Changes) != 0 {
		t.Fatalf("expected empty changes for create, got %v", a.Changes)
	}
}

func TestAuditWriteUpdate(t *testing.T) {
	t.Parallel()

	app := auditTestApp(t, "demo1")
	defer app.Cleanup()

	rec, err := app.FindRecordById("demo1", "imy661ixudk5izi")
	if err != nil {
		t.Fatal(err)
	}

	rec.Set("text", "updated-value")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	audits := auditsForRecord(t, app, rec.Id)
	if len(audits) != 1 {
		t.Fatalf("expected 1 audit, got %d", len(audits))
	}

	a := audits[0]
	if a.Event != "update" {
		t.Fatalf("expected event=update, got %q", a.Event)
	}
	if len(a.Snapshot) != 0 {
		t.Fatalf("expected empty snapshot for update, got %v", a.Snapshot)
	}

	change, ok := a.Changes["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected changes.text object, got %v", a.Changes["text"])
	}
	if change["old"] != "lorem ipsum" || change["new"] != "updated-value" {
		t.Fatalf("unexpected changes.text diff: %v", change)
	}
}

func TestAuditWriteDelete(t *testing.T) {
	t.Parallel()

	app := auditTestApp(t, "demo1")
	defer app.Cleanup()

	col, err := app.FindCollectionByNameOrId("demo1")
	if err != nil {
		t.Fatal(err)
	}

	rec := core.NewRecord(col)
	rec.Set("text", "to-be-deleted")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	if err := app.Delete(rec); err != nil {
		t.Fatal(err)
	}

	audits := auditsForRecord(t, app, rec.Id)
	if len(audits) != 2 {
		t.Fatalf("expected 2 audits (create+delete), got %d", len(audits))
	}

	var del *core.Audit
	for _, a := range audits {
		if a.Event == "delete" {
			del = a
		}
	}
	if del == nil {
		t.Fatal("expected a delete audit")
	}
	if got := del.Snapshot["text"]; got != "to-be-deleted" {
		t.Fatalf("expected delete snapshot.text=to-be-deleted, got %v", got)
	}
}

func TestAuditWriteSkipNonAllowlisted(t *testing.T) {
	t.Parallel()

	// only demo1 is audited
	app := auditTestApp(t, "demo1")
	defer app.Cleanup()

	col, err := app.FindCollectionByNameOrId("demo2")
	if err != nil {
		t.Fatal(err)
	}

	rec := core.NewRecord(col)
	rec.Set("title", "not audited")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	if got := auditsCount(t, app, dbx.HashExp{"collection_name": "demo2"}); got != 0 {
		t.Fatalf("expected 0 audits for non-allowlisted demo2, got %d", got)
	}
}

func TestAuditWriteDisabled(t *testing.T) {
	t.Parallel()

	// audit fully disabled (Enabled defaults to false)
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	col, err := app.FindCollectionByNameOrId("demo1")
	if err != nil {
		t.Fatal(err)
	}

	rec := core.NewRecord(col)
	rec.Set("text", "disabled")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	if got := auditsCount(t, app, nil); got != 0 {
		t.Fatalf("expected 0 audits when disabled, got %d", got)
	}
}

func TestAuditRedactionUpdateChanges(t *testing.T) {
	t.Parallel()

	app := auditTestApp(t, "clients")
	defer app.Cleanup()

	rec, err := app.FindRecordById("clients", "gk390qegs4y47wn")
	if err != nil {
		t.Fatal(err)
	}

	rec.Set("name", "Audited Name")
	rec.SetPassword("newSecret12345")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	audits := auditsForRecord(t, app, rec.Id)
	if len(audits) != 1 {
		t.Fatalf("expected 1 audit, got %d", len(audits))
	}

	changes := audits[0].Changes
	if _, ok := changes["password"]; ok {
		t.Fatalf("password must be redacted from changes, got %v", changes["password"])
	}
	if _, ok := changes["tokenKey"]; ok {
		t.Fatalf("tokenKey must be redacted from changes, got %v", changes["tokenKey"])
	}
	if _, ok := changes["name"].(map[string]any); !ok {
		t.Fatalf("expected non-secret field 'name' in changes, got %v", changes)
	}
}

func TestAuditRedactionCreateSnapshot(t *testing.T) {
	t.Parallel()

	app := auditTestApp(t, "clients")
	defer app.Cleanup()

	col, err := app.FindCollectionByNameOrId("clients")
	if err != nil {
		t.Fatal(err)
	}

	rec := core.NewRecord(col)
	rec.Set("email", "audit-redact@example.com")
	rec.Set("username", "audituser9")
	rec.SetPassword("secret12345")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	audits := auditsForRecord(t, app, rec.Id)
	if len(audits) != 1 {
		t.Fatalf("expected 1 audit, got %d", len(audits))
	}

	snapshot := audits[0].Snapshot
	if _, ok := snapshot["password"]; ok {
		t.Fatalf("password must be redacted from snapshot")
	}
	if _, ok := snapshot["tokenKey"]; ok {
		t.Fatalf("tokenKey must be redacted from snapshot")
	}
	if snapshot["email"] != "audit-redact@example.com" {
		t.Fatalf("expected non-secret field 'email' in snapshot, got %v", snapshot["email"])
	}
}

// TestAuditBestEffortNonTx verifies that a failing audit INSERT on the
// top-level (autocommit) write path does NOT roll back the data change.
func TestAuditBestEffortNonTx(t *testing.T) {
	t.Parallel()

	app := auditTestApp(t, "demo1")
	defer app.Cleanup()

	// force every audit INSERT to fail (isolated per-test DB)
	if _, err := app.DB().NewQuery(`DROP TABLE IF EXISTS "_audits" CASCADE`).Execute(); err != nil {
		t.Fatal(err)
	}

	col, err := app.FindCollectionByNameOrId("demo1")
	if err != nil {
		t.Fatal(err)
	}

	rec := core.NewRecord(col)
	rec.Set("text", "still-persisted")
	if err := app.Save(rec); err != nil {
		t.Fatalf("record save must succeed despite audit failure, got %v", err)
	}

	if _, err := app.FindRecordById("demo1", rec.Id); err != nil {
		t.Fatalf("record must exist despite audit failure, got %v", err)
	}
}

// TestAuditBestEffortInTx verifies the SAVEPOINT guard: a failing audit INSERT
// inside a transaction must NOT poison the surrounding tx (the data write still
// commits).
func TestAuditBestEffortInTx(t *testing.T) {
	t.Parallel()

	app := auditTestApp(t, "demo1")
	defer app.Cleanup()

	if _, err := app.DB().NewQuery(`DROP TABLE IF EXISTS "_audits" CASCADE`).Execute(); err != nil {
		t.Fatal(err)
	}

	col, err := app.FindCollectionByNameOrId("demo1")
	if err != nil {
		t.Fatal(err)
	}

	var recId string
	err = app.RunInTransaction(func(txApp core.App) error {
		rec := core.NewRecord(col)
		rec.Set("text", "tx-persisted")
		if err := txApp.Save(rec); err != nil {
			return err
		}
		recId = rec.Id
		return nil
	})
	if err != nil {
		t.Fatalf("transaction must commit despite audit failure (savepoint guard), got %v", err)
	}

	if _, err := app.FindRecordById("demo1", recId); err != nil {
		t.Fatalf("record must exist despite in-tx audit failure, got %v", err)
	}
}

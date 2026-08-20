package core_test

import (
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
)

func TestFindAuditById(t *testing.T) {
	t.Parallel()

	app := auditTestApp(t, "demo1")
	defer app.Cleanup()

	col, err := app.FindCollectionByNameOrId("demo1")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("text", "find-by-id")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	// resolve the generated audit id
	created := &core.Audit{}
	if err := app.AuditQuery().AndWhere(dbx.HashExp{"record_id": rec.Id}).One(created); err != nil {
		t.Fatal(err)
	}

	found, err := app.FindAuditById(created.Id)
	if err != nil {
		t.Fatalf("expected to find audit, got %v", err)
	}
	if found.Id != created.Id || found.CollectionName != "demo1" || found.Event != "create" {
		t.Fatalf("unexpected audit: %+v", found)
	}

	if _, err := app.FindAuditById("missing0000000000"); err == nil {
		t.Fatal("expected an error for a missing audit id")
	}
}

// TestFlushAuditReadsNoop verifies the exported test-drain helper is safe when
// there is nothing buffered (and never writes spurious rows).
func TestFlushAuditReadsNoop(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	core.FlushAuditReads(app)

	var n int
	if err := app.ModelQuery(&core.AuditRead{}).Select("COUNT(*)").Row(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 read audits, got %d", n)
	}
}

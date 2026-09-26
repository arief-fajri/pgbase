package core

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
)

// newVerifyTestApp boots a dedicated-database app for the W-08 restore
// verification gate tests (coldboot pattern): every mutation happens inside
// the dedicated database, which is dropped at cleanup, so the shared test
// schema is never touched.
func newVerifyTestApp(t *testing.T) *BaseApp {
	t.Helper()

	dbName := fmt.Sprintf("pb_verify_%d_%d", os.Getpid(), time.Now().UnixNano())

	maint, err := DefaultDBConnect(ResolveDBConfig(DBConfig{
		SSLMode:      "disable",
		MaxOpenConns: 2,
		MaxIdleConns: 1,
	}))
	if err != nil {
		t.Fatalf("maintenance connect: %v", err)
	}
	t.Cleanup(func() { maint.Close() })

	if _, err := maint.NewQuery("DROP DATABASE IF EXISTS " + dbName + " WITH (FORCE)").Execute(); err != nil {
		t.Fatalf("failed to drop leftover db: %v", err)
	}
	if _, err := maint.NewQuery("CREATE DATABASE " + dbName).Execute(); err != nil {
		t.Fatalf("failed to create database %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		maint.NewQuery(
			"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='" + dbName + "' AND pid <> pg_backend_pid()",
		).Execute()
		maint.NewQuery("DROP DATABASE IF EXISTS " + dbName + " WITH (FORCE)").Execute()
	})

	dataDir, err := os.MkdirTemp("", "pb_verify_data_")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dataDir) })

	app := NewBaseApp(BaseAppConfig{
		DataDir:       dataDir,
		EncryptionEnv: "pb_verify_test_env",
		DBConnect: func(c DBConfig) (*dbx.DB, error) {
			c.DBName = dbName
			return DefaultDBConnect(c)
		},
	})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	t.Cleanup(func() { app.ResetBootstrapState() })

	// a real backup always carries at least one superuser (the last one cannot
	// be deleted), so seed one for the verification gate baseline
	coll, err := app.FindCollectionByNameOrId("_superusers")
	if err != nil {
		t.Fatalf("find _superusers collection: %v", err)
	}
	superuser := NewRecord(coll)
	superuser.Set("email", "verify@example.com")
	superuser.SetPassword("verifytestpass123")
	if err := app.Save(superuser); err != nil {
		t.Fatalf("seed superuser: %v", err)
	}

	return app
}

// TestParsePGRestoreToc verifies the TOC parser against the observed
// `pg_restore --list` entry shapes (pg_dump 16/17/18 custom format), including
// the two-word types (TABLE DATA, TABLE ATTACH, INDEX ATTACH) and the
// schema-less entries (SCHEMA -, EXTENSION -).
func TestParsePGRestoreToc(t *testing.T) {
	out := `
; Archive created at 2026-09-26 ...
;     dbname: pgbase_test
;
; Selected TOC Entries:
;
232; 1259 17656 TABLE public _collections test
3820; 0 17656 TABLE DATA public _collections test
3656; 1259 17832 INDEX public idx_audit_reads_coll test
3463; 0 0 TABLE ATTACH public _audits_default test
3671; 0 0 INDEX ATTACH public idx_audit_reads_default_created_idx test
19; 3079 16385 EXTENSION - pgcrypto
3655; 2606 17811 CONSTRAINT public _audit_reads _audit_reads_pkey test
3828; 0 17812 TABLE DATA public _audit_reads_default test
`

	toc, err := parsePGRestoreToc(out)
	if err != nil {
		t.Fatalf("parsePGRestoreToc: %v", err)
	}

	// TABLE + TABLE DATA dedupe to one table each; ATTACH variants are steps,
	// not objects; EXTENSION/CONSTRAINT/SCHEMA are not collected
	wantTables := "public._collections,public._audit_reads_default"
	gotTables := make([]string, 0, len(toc.Tables))
	for _, tbl := range toc.Tables {
		gotTables = append(gotTables, tbl.Schema+"."+tbl.Name)
	}
	if got := strings.Join(gotTables, ","); got != wantTables {
		t.Fatalf("expected tables [%s], got [%s]", wantTables, got)
	}

	if len(toc.Indexes) != 1 || toc.Indexes[0].Schema != "public" || toc.Indexes[0].Name != "idx_audit_reads_coll" {
		t.Fatalf("expected only the regular INDEX entry, got %+v", toc.Indexes)
	}

	// an archive without tables is not a full pg_dump — reject loudly
	if _, err := parsePGRestoreToc("; no entries here\n19; 3079 16385 EXTENSION - pgcrypto"); err == nil {
		t.Fatal("expected an error for a TOC without tables")
	}
}

// TestPGRestoreFatalStderrErrors verifies the stderr classification: every
// observed benign family must be tolerated, every other error line must stay
// fatal (W-08 — the gate prefers failing loudly over passing a partial
// restore).
func TestPGRestoreFatalStderrErrors(t *testing.T) {
	stderr := `
pg_restore: connecting to database for restore
pg_restore: error: could not execute query: ERROR:  unrecognized configuration parameter "transaction_timeout"
Command was: SET transaction_timeout = 0;
pg_restore: error: could not execute query: ERROR:  cannot drop inherited constraint "_audits_default_pkey" of relation "_audits_default"
Command was: ALTER TABLE IF EXISTS ONLY public._audits_default DROP CONSTRAINT IF EXISTS _audits_default_pkey;
pg_restore: error: could not execute query: ERROR:  cannot drop extension "pgcrypto" because other objects depend on it
pg_restore: error: could not execute query: ERROR:  extension "pgcrypto" already exists
pg_restore: warning: errors ignored on restore: 6
pg_restore: error: could not execute query: ERROR:  relation "demo1" does not exist
pg_restore: error: COPY failed for table "public.demo1": invalid byte sequence
pg_restore: error: connection to server at "127.0.0.1", port 5432 failed
`

	fatal := pgRestoreFatalStderrErrors(stderr)

	if len(fatal) != 3 {
		t.Fatalf("expected exactly 3 fatal lines, got %d: %q", len(fatal), fatal)
	}
	for _, line := range fatal {
		if !strings.Contains(line, "relation \"demo1\" does not exist") &&
			!strings.Contains(line, "COPY failed") &&
			!strings.Contains(line, "connection to server") {
			t.Fatalf("unexpected line classified as fatal: %q", line)
		}
	}

	if benign := pgRestoreFatalStderrErrors("pg_restore: warning: errors ignored on restore: 1"); len(benign) != 0 {
		t.Fatalf("warnings must never be fatal, got %q", benign)
	}
}

// TestVerifyRestoredDatabase verifies the W-08 post-restore gate failure
// paths: a missing TOC table, a missing TOC index (post-data completion
// marker), a missing settings row, and a missing superuser password must each
// fail loudly, while a matching expectation set on a healthy database passes.
func TestVerifyRestoredDatabase(t *testing.T) {
	app := newVerifyTestApp(t)
	ctx := context.Background()

	healthy := pgRestoreToc{
		Tables: []pgTocObject{
			{Schema: "public", Name: "_collections"},
			{Schema: "public", Name: "_params"},
			{Schema: "public", Name: "_superusers"},
		},
		Indexes: []pgTocObject{{Schema: "public", Name: "_collections_pkey"}},
	}
	if err := app.verifyRestoredDatabase(ctx, healthy); err != nil {
		t.Fatalf("baseline: expected a healthy database to verify, got: %v", err)
	}

	// 1. a table the archive contains is missing from the restored catalog
	missingTable := healthy
	missingTable.Tables = append([]pgTocObject{{Schema: "public", Name: "_archive_table_missing"}}, healthy.Tables...)
	err := app.verifyRestoredDatabase(ctx, missingTable)
	if err == nil || !strings.Contains(err.Error(), `"public"."_archive_table_missing" is missing after the restore`) {
		t.Fatalf("expected a missing-table failure, got: %v", err)
	}

	// 2. an index the archive contains is missing — the post-data section did
	// not finish (the deterministic fingerprint of a partial restore)
	missingIndex := healthy
	missingIndex.Indexes = append([]pgTocObject{{Schema: "public", Name: "idx_archive_index_missing"}}, healthy.Indexes...)
	err = app.verifyRestoredDatabase(ctx, missingIndex)
	if err == nil || !strings.Contains(err.Error(), `idx_archive_index_missing" is missing after the restore`) {
		t.Fatalf("expected a missing-index failure, got: %v", err)
	}

	// 3. no settings row in _params
	if _, err := app.ConcurrentDB().NewQuery(`DELETE FROM "_params" WHERE id = 'settings'`).Execute(); err != nil {
		t.Fatalf("delete settings row: %v", err)
	}
	err = app.verifyRestoredDatabase(ctx, healthy)
	if err == nil || !strings.Contains(err.Error(), "no settings row in _params") {
		t.Fatalf("expected a missing-settings failure, got: %v", err)
	}
	// re-insert a minimal settings row so the remaining checks run past it
	if _, err := app.ConcurrentDB().NewQuery(
		`INSERT INTO "_params" (id, value) VALUES ('settings', '{}')`,
	).Execute(); err != nil {
		t.Fatalf("re-insert settings row: %v", err)
	}

	// 4. no superuser with a non-empty password (auth not preserved)
	if _, err := app.ConcurrentDB().NewQuery(`UPDATE "_superusers" SET password = ''`).Execute(); err != nil {
		t.Fatalf("clear superuser passwords: %v", err)
	}
	err = app.verifyRestoredDatabase(ctx, healthy)
	if err == nil || !strings.Contains(err.Error(), "no superuser with a non-empty password") {
		t.Fatalf("expected a missing-superuser failure, got: %v", err)
	}
}

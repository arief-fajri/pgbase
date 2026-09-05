package core_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/arief-fajri/pgbase/tools/archive"
)

// legacyV22Fixture is a committed, inspectable pre-v0.23 SQLite backup sample
// (see TestGenerateLegacyV22Fixture, which regenerates it via buildLegacyV22DB).
const legacyV22Fixture = "testdata/legacy_v22_backup/data.db"

const legacyV22Timestamp = "2023-01-01 10:00:00.000Z"

// TestGenerateLegacyV22Fixture (re)generates the committed pre-v0.23 SQLite
// sample at legacyV22Fixture. It is skipped during normal runs and only writes
// the file when PB_GEN_SAMPLES=1, so the committed sample stays reproducible
// and reviewable without being rewritten on every test run:
//
//	PB_GEN_SAMPLES=1 go test ./core/ -run TestGenerateLegacyV22Fixture
func TestGenerateLegacyV22Fixture(t *testing.T) {
	if os.Getenv("PB_GEN_SAMPLES") == "" {
		t.Skip("set PB_GEN_SAMPLES=1 to regenerate the committed legacy v22 sample")
	}

	if err := os.MkdirAll(filepath.Dir(legacyV22Fixture), os.ModePerm); err != nil {
		t.Fatal(err)
	}
	if err := buildLegacyV22DB(legacyV22Fixture); err != nil {
		t.Fatalf("buildLegacyV22DB: %v", err)
	}

	t.Logf("wrote legacy v22 sample to %s", legacyV22Fixture)
}

// TestBackupRestoreScenario walks the full backup/import/restore lifecycle the
// fork must support after the SQLite->PostgreSQL migration, covering all three
// source "versions" of a data.db:
//
//	below v0.23 (legacy SQLite) -> above v0.23 shape (this fork's own dump) -> re-import.
//
// The 8 steps mirror the acceptance scenario:
//
//  1. import a pre-v0.23 SQLite backup (backward compatibility);
//  2. inspect the imported state (the import IS the restore for this fork);
//  3. mutate data (create a new collection + records, update existing ones);
//  4. back up -> the archive now bundles a PostgreSQL dump as data.db (Gap B);
//  5. "download" the backup (extract it) and confirm the bundled data.db;
//  6. mutate again so the live state diverges from step 3;
//  7. import the backup's data.db (the exporter -> importer round-trip);
//  8. assert the DB is reset to the step-3 snapshot (proves export/import fidelity).
func TestBackupRestoreScenario(t *testing.T) {
	ctx := context.Background()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	// -----------------------------------------------------------------
	// step 1 - import a pre-v0.23 (legacy SQLite) backup
	// -----------------------------------------------------------------
	legacyDir := t.TempDir()
	legacyDB := filepath.Join(legacyDir, "data.db")
	if err := buildLegacyV22DB(legacyDB); err != nil {
		t.Fatalf("buildLegacyV22DB: %v", err)
	}

	// simulate a fresh restore target (see clearNonSystemCollections)
	clearNonSystemCollections(t, app)

	if err := app.ImportFromSQLiteDir(ctx, legacyDir); err != nil {
		t.Fatalf("step 1 ImportFromSQLiteDir (pre-v0.23): %v", err)
	}

	// collections created from the legacy "schema" column
	for _, name := range []string{"authors", "articles"} {
		if _, err := app.FindCollectionByNameOrId(name); err != nil {
			t.Fatalf("step 1: missing collection %q after legacy import: %v", name, err)
		}
	}

	if got := countRows(t, app, "authors"); got != 2 {
		t.Fatalf("step 1: expected 2 authors, got %d", got)
	}
	if got := countRows(t, app, "articles"); got != 2 {
		t.Fatalf("step 1: expected 2 articles, got %d", got)
	}

	// the legacy "_admins" row was migrated into the "_superusers" collection
	var adminEmail string
	if err := app.DB().NewQuery(`SELECT email FROM "_superusers" WHERE id = 'admin_lifecycle01'`).Row(&adminEmail); err != nil {
		t.Fatalf("step 1: legacy admin not migrated to superusers: %v", err)
	}
	if adminEmail != "admin@lifecycle.test" {
		t.Fatalf("step 1: migrated admin email mismatch: got %q", adminEmail)
	}

	// the legacy nested field options were converted to the flat v0.23 shape
	articles, err := app.FindCollectionByNameOrId("articles")
	if err != nil {
		t.Fatalf("step 1: find articles: %v", err)
	}
	if f, ok := articles.Fields.GetByName("views").(*core.NumberField); !ok {
		t.Fatalf("step 1: expected articles.views to be a NumberField")
	} else if !f.OnlyInt {
		t.Fatalf("step 1: expected number.noDecimal to convert to onlyInt=true")
	}
	if f, ok := articles.Fields.GetByName("body").(*core.EditorField); !ok {
		t.Fatalf("step 1: expected articles.body to be an EditorField")
	} else if f.ConvertURLs {
		t.Fatalf("step 1: expected editor.convertUrls=false to convert to convertURLs=false")
	}

	// -----------------------------------------------------------------
	// step 2 - inspect the imported (restored) state
	// -----------------------------------------------------------------
	// verify a record value round-tripped verbatim through the coercion
	var r1Title string
	if err := app.DB().NewQuery(`SELECT title FROM "articles" WHERE id = 'art_legacy_0001'`).Row(&r1Title); err != nil {
		t.Fatalf("step 2: read legacy article: %v", err)
	}
	if r1Title != "First Post" {
		t.Fatalf("step 2: expected legacy article title %q, got %q", "First Post", r1Title)
	}

	// -----------------------------------------------------------------
	// step 3 - mutate: create a new collection + records, update a record.
	// This is the "target snapshot" the step-8 restore must reproduce.
	// -----------------------------------------------------------------
	tags := core.NewBaseCollection("tags")
	tags.Fields.Add(&core.TextField{Name: "label"})
	if err := app.Save(tags); err != nil {
		t.Fatalf("step 3: create tags collection: %v", err)
	}

	for _, label := range []string{"go", "postgres"} {
		rec := core.NewRecord(tags)
		rec.Set("label", label)
		if err := app.Save(rec); err != nil {
			t.Fatalf("step 3: create tag %q: %v", label, err)
		}
	}

	// add a third article
	art3 := core.NewRecord(articles)
	art3.Set("title", "Third Post")
	art3.Set("views", 7)
	if err := app.Save(art3); err != nil {
		t.Fatalf("step 3: create article: %v", err)
	}

	// update the first legacy article
	art1, err := app.FindRecordById("articles", "art_legacy_0001")
	if err != nil {
		t.Fatalf("step 3: find legacy article: %v", err)
	}
	art1.Set("title", "First Post (edited)")
	if err := app.Save(art1); err != nil {
		t.Fatalf("step 3: update legacy article: %v", err)
	}

	// snapshot of the step-3 state that step 8 must restore
	wantAuthors := countRows(t, app, "authors")   // 2
	wantArticles := countRows(t, app, "articles") // 3
	wantTags := countRows(t, app, "tags")         // 2
	const wantArt1Title = "First Post (edited)"

	if wantArticles != 3 || wantTags != 2 {
		t.Fatalf("step 3: unexpected baseline (articles=%d tags=%d)", wantArticles, wantTags)
	}

	// -----------------------------------------------------------------
	// step 4 - back up. The archive must now bundle a PostgreSQL dump as
	// "data.db" (the fix for Gap B: native backups used to be storage-only).
	// -----------------------------------------------------------------
	const backupName = "scenario_snapshot.zip"
	// opt into the portable SQLite dump so this hermetic scenario test does
	// not require an external pg_dump client (the native "pg" default is
	// covered by TestPGDumpExportImportRoundTrip).
	app.Settings().Backups.Format = core.BackupFormatSQLite
	if err := app.CreateBackup(ctx, backupName); err != nil {
		t.Fatalf("step 4 CreateBackup: %v", err)
	}

	backupPath := filepath.Join(app.DataDir(), core.LocalBackupsDirName, backupName)
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("step 4: backup file not created: %v", err)
	}

	// -----------------------------------------------------------------
	// step 5 - "download" the backup: extract it and confirm the bundled dump
	// -----------------------------------------------------------------
	downloadDir := t.TempDir()
	if err := archive.Extract(backupPath, downloadDir); err != nil {
		t.Fatalf("step 5 extract backup: %v", err)
	}

	dumpPath := filepath.Join(downloadDir, sqliteBackupDBNameForTest)
	if _, err := os.Stat(dumpPath); err != nil {
		t.Fatalf("step 5: the native backup must now bundle a %q dump: %v", sqliteBackupDBNameForTest, err)
	}

	// the dump is a valid v0.23-shaped SQLite DB carrying the step-3 state
	assertDumpSnapshot(t, dumpPath, wantArticles, wantTags)

	// -----------------------------------------------------------------
	// step 6 - mutate again so the live DB diverges from the step-3 snapshot
	// -----------------------------------------------------------------
	tagsColl, err := app.FindCollectionByNameOrId("tags")
	if err != nil {
		t.Fatalf("step 6: find tags: %v", err)
	}
	if err := app.Delete(tagsColl); err != nil {
		t.Fatalf("step 6: delete tags collection: %v", err)
	}

	art4 := core.NewRecord(articles)
	art4.Set("title", "Fourth Post (post-snapshot)")
	if err := app.Save(art4); err != nil {
		t.Fatalf("step 6: create post-snapshot article: %v", err)
	}

	art1.Set("title", "First Post (diverged)")
	if err := app.Save(art1); err != nil {
		t.Fatalf("step 6: update article: %v", err)
	}

	// sanity: the live state now differs from the snapshot
	if countRows(t, app, "articles") == wantArticles {
		t.Fatalf("step 6: expected articles count to diverge from the snapshot")
	}
	if app.HasTable("tags") {
		t.Fatalf("step 6: expected tags table to be gone before restore")
	}

	// -----------------------------------------------------------------
	// step 7 - import the downloaded backup's data.db (exporter -> importer
	// round-trip). This is the full DB+storage restore the feature performs.
	// -----------------------------------------------------------------
	if err := app.ImportFromSQLiteDir(ctx, downloadDir); err != nil {
		t.Fatalf("step 7 ImportFromSQLiteDir (PostgreSQL dump): %v", err)
	}

	// -----------------------------------------------------------------
	// step 8 - the DB is reset to the step-3 snapshot
	// -----------------------------------------------------------------
	if got := countRows(t, app, "authors"); got != wantAuthors {
		t.Fatalf("step 8: authors not restored: want %d, got %d", wantAuthors, got)
	}
	if got := countRows(t, app, "articles"); got != wantArticles {
		t.Fatalf("step 8: articles not restored: want %d, got %d (step-6 changes should be gone)", wantArticles, got)
	}

	// the step-3 collection was recreated ...
	if _, err := app.FindCollectionByNameOrId("tags"); err != nil {
		t.Fatalf("step 8: tags collection not restored: %v", err)
	}
	if got := countRows(t, app, "tags"); got != wantTags {
		t.Fatalf("step 8: tags not restored: want %d, got %d", wantTags, got)
	}

	// ... and the record edits were rolled back to the snapshot value
	var gotArt1Title string
	if err := app.DB().NewQuery(`SELECT title FROM "articles" WHERE id = 'art_legacy_0001'`).Row(&gotArt1Title); err != nil {
		t.Fatalf("step 8: read restored article: %v", err)
	}
	if gotArt1Title != wantArt1Title {
		t.Fatalf("step 8: article title not restored to snapshot: want %q, got %q", wantArt1Title, gotArt1Title)
	}
}

// assertDumpSnapshot opens the exported SQLite dump directly and verifies it is
// a v0.23-shaped legacy SQLite database carrying the expected step-3 snapshot.
func assertDumpSnapshot(t *testing.T, dumpPath string, wantArticles, wantTags int) {
	t.Helper()

	db, err := sql.Open("sqlite", "file:"+dumpPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open dump: %v", err)
	}
	defer db.Close()

	// v0.23 markers: a "_superusers" table and a "fields" column on "_collections"
	var superusers int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='_superusers'`).Scan(&superusers); err != nil {
		t.Fatalf("probe _superusers: %v", err)
	}
	if superusers != 1 {
		t.Fatalf("exported dump is not v0.23-shaped: missing _superusers table")
	}
	var hasFields int
	if err := db.QueryRow(`SELECT count(*) FROM pragma_table_info('_collections') WHERE name='fields'`).Scan(&hasFields); err != nil {
		t.Fatalf("probe _collections.fields: %v", err)
	}
	if hasFields != 1 {
		t.Fatalf("exported dump is not v0.23-shaped: missing _collections.fields column")
	}

	// the dump carries the step-3 data
	var gotArticles, gotTags int
	if err := db.QueryRow(`SELECT count(*) FROM "articles"`).Scan(&gotArticles); err != nil {
		t.Fatalf("count articles in dump: %v", err)
	}
	if gotArticles != wantArticles {
		t.Fatalf("dump articles: want %d, got %d", wantArticles, gotArticles)
	}
	if err := db.QueryRow(`SELECT count(*) FROM "tags"`).Scan(&gotTags); err != nil {
		t.Fatalf("count tags in dump: %v", err)
	}
	if gotTags != wantTags {
		t.Fatalf("dump tags: want %d, got %d", wantTags, gotTags)
	}
}

// -------------------------------------------------------------------
// pre-v0.23 (v0.22) SQLite fixture builder
// -------------------------------------------------------------------

// buildLegacyV22DB writes a minimal but representative pre-v0.23 SQLite
// backup at path. It uses the legacy "_collections.schema" column (with
// nested per-type field options), a separate "_admins" table, and base
// collections whose records span the tricky field types the importer must
// convert (text with null min/max, number noDecimal, relation displayFields,
// json, select, bool, date, editor convertUrls, email, url, file).
//
// It is the reproducible source of the committed testdata sample and is also
// invoked directly by TestBackupRestoreScenario to keep the test hermetic.
func buildLegacyV22DB(path string) error {
	_ = os.Remove(path)

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()

	schemaStmts := []string{
		`CREATE TABLE _collections (
			id TEXT PRIMARY KEY NOT NULL,
			system BOOLEAN NOT NULL DEFAULT FALSE,
			type TEXT NOT NULL DEFAULT 'base',
			name TEXT UNIQUE NOT NULL,
			schema JSON NOT NULL DEFAULT '[]',
			indexes JSON NOT NULL DEFAULT '[]',
			listRule TEXT DEFAULT NULL,
			viewRule TEXT DEFAULT NULL,
			createRule TEXT DEFAULT NULL,
			updateRule TEXT DEFAULT NULL,
			deleteRule TEXT DEFAULT NULL,
			options JSON NOT NULL DEFAULT '{}',
			created TEXT NOT NULL DEFAULT '',
			updated TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE _admins (
			id TEXT PRIMARY KEY NOT NULL,
			avatar INTEGER NOT NULL DEFAULT 0,
			email TEXT UNIQUE NOT NULL,
			tokenKey TEXT UNIQUE NOT NULL,
			passwordHash TEXT NOT NULL,
			lastResetSentAt TEXT NOT NULL DEFAULT '',
			created TEXT NOT NULL DEFAULT '',
			updated TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE _params (
			id TEXT PRIMARY KEY NOT NULL,
			key TEXT,
			value JSON,
			created TEXT DEFAULT '',
			updated TEXT DEFAULT ''
		)`,
		`CREATE TABLE authors (
			id TEXT PRIMARY KEY NOT NULL,
			created TEXT DEFAULT '',
			updated TEXT DEFAULT '',
			name TEXT DEFAULT '',
			age NUMERIC DEFAULT 0
		)`,
		`CREATE TABLE articles (
			id TEXT PRIMARY KEY NOT NULL,
			created TEXT DEFAULT '',
			updated TEXT DEFAULT '',
			title TEXT DEFAULT '',
			views NUMERIC DEFAULT 0,
			published BOOLEAN DEFAULT FALSE,
			contact TEXT DEFAULT '',
			homepage TEXT DEFAULT '',
			publishDate TEXT DEFAULT '',
			category TEXT DEFAULT '',
			meta JSON DEFAULT NULL,
			body TEXT DEFAULT '',
			author TEXT DEFAULT '',
			attachment TEXT DEFAULT ''
		)`,
	}
	for _, s := range schemaStmts {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}

	authorsSchema := mustJSON([]map[string]any{
		legacyField("authors_name_01", "name", "text", true, map[string]any{"min": nil, "max": nil, "pattern": ""}),
		legacyField("authors_age_01", "age", "number", false, map[string]any{"min": nil, "max": nil, "noDecimal": false}),
	})

	articlesSchema := mustJSON([]map[string]any{
		legacyField("articles_title_01", "title", "text", true, map[string]any{"min": nil, "max": nil, "pattern": ""}),
		legacyField("articles_views_01", "views", "number", false, map[string]any{"min": nil, "max": nil, "noDecimal": true}),
		legacyField("articles_pub_01", "published", "bool", false, map[string]any{}),
		legacyField("articles_contact_01", "contact", "email", false, map[string]any{"exceptDomains": nil, "onlyDomains": nil}),
		legacyField("articles_home_01", "homepage", "url", false, map[string]any{"exceptDomains": nil, "onlyDomains": nil}),
		legacyField("articles_date_01", "publishDate", "date", false, map[string]any{"min": "", "max": ""}),
		legacyField("articles_cat_01", "category", "select", false, map[string]any{"maxSelect": 1, "values": []string{"news", "sports", "tech"}}),
		legacyField("articles_meta_01", "meta", "json", false, map[string]any{"maxSize": 2000000}),
		legacyField("articles_body_01", "body", "editor", false, map[string]any{"convertUrls": false}),
		legacyField("articles_author_01", "author", "relation", false, map[string]any{
			"collectionId":  "authors_col_0001",
			"cascadeDelete": false,
			"minSelect":     nil,
			"maxSelect":     1,
			"displayFields": []string{"name"},
		}),
		legacyField("articles_att_01", "attachment", "file", false, map[string]any{
			"maxSelect": 1, "maxSize": 5242880, "mimeTypes": []string{}, "thumbs": []string{}, "protected": false,
		}),
	})

	collInsert := `INSERT INTO _collections
		(id, system, type, name, schema, indexes, listRule, viewRule, createRule, updateRule, deleteRule, options, created, updated)
		VALUES (?, 0, 'base', ?, ?, '[]', '', '', NULL, NULL, NULL, '{}', ?, ?)`
	if _, err := db.Exec(collInsert, "authors_col_0001", "authors", authorsSchema, legacyV22Timestamp, legacyV22Timestamp); err != nil {
		return err
	}
	if _, err := db.Exec(collInsert, "articles_col_001", "articles", articlesSchema, legacyV22Timestamp, legacyV22Timestamp); err != nil {
		return err
	}

	// legacy admin (migrated into _superusers on import)
	if _, err := db.Exec(
		`INSERT INTO _admins (id, avatar, email, tokenKey, passwordHash, lastResetSentAt, created, updated)
			VALUES (?, 0, ?, ?, ?, '', ?, ?)`,
		"admin_lifecycle01", "admin@lifecycle.test", "tok_admin_lifecycle01",
		"$2a$10$3Q3vJ8n7mQhVv0m3s0m3s.uJ8n7mQhVv0m3s0m3s0m3s0m3s0m3s", legacyV22Timestamp, legacyV22Timestamp,
	); err != nil {
		return err
	}

	// pre-v0.23 settings row: a random id + a "key" column (NOT id='settings'),
	// so the importer's v0.23 lookup skips it cleanly (as documented).
	if _, err := db.Exec(
		`INSERT INTO _params (id, key, value, created, updated) VALUES (?, 'settings', ?, ?, ?)`,
		"prm_legacy_00001", `{"meta":{"appName":"legacy-sample"}}`, legacyV22Timestamp, legacyV22Timestamp,
	); err != nil {
		return err
	}

	// authors records
	authorRows := [][]any{
		{"aut_legacy_0001", legacyV22Timestamp, legacyV22Timestamp, "Ada Lovelace", 36},
		{"aut_legacy_0002", legacyV22Timestamp, legacyV22Timestamp, "Alan Turing", 41},
	}
	for _, r := range authorRows {
		if _, err := db.Exec(
			`INSERT INTO authors (id, created, updated, name, age) VALUES (?, ?, ?, ?, ?)`, r...,
		); err != nil {
			return err
		}
	}

	// articles records (span every tricky field type)
	articleRows := [][]any{
		{
			"art_legacy_0001", legacyV22Timestamp, legacyV22Timestamp, "First Post", 100, 1,
			"ada@example.com", "https://example.com", "2024-05-01 12:00:00.000Z", "news",
			`{"tags":["intro"]}`, "<p>hello</p>", "aut_legacy_0001", "",
		},
		{
			"art_legacy_0002", legacyV22Timestamp, legacyV22Timestamp, "Second Post", 250, 0,
			"alan@example.com", "https://turing.dev", "2024-06-15 09:30:00.000Z", "tech",
			`{"tags":["ml","ai"]}`, "<p>world</p>", "aut_legacy_0002", "",
		},
	}
	for _, r := range articleRows {
		if _, err := db.Exec(
			`INSERT INTO articles
				(id, created, updated, title, views, published, contact, homepage, publishDate, category, meta, body, author, attachment)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, r...,
		); err != nil {
			return err
		}
	}

	return nil
}

// legacyField builds a single pre-v0.23 field definition (nested per-type
// options) as stored in the legacy "_collections.schema" JSON array.
func legacyField(id, name, ftype string, required bool, options map[string]any) map[string]any {
	return map[string]any{
		"system":      false,
		"id":          id,
		"name":        name,
		"type":        ftype,
		"required":    required,
		"presentable": false,
		"unique":      false,
		"options":     options,
	}
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

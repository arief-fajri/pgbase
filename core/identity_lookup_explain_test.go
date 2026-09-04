package core_test

import (
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/arief-fajri/pgbase/tests"
)

// functionalIndexesOnTable returns the index definitions (pg_indexes.indexdef)
// for the provided table that index a single column and use LOWER(...).
func functionalIndexesOnTable(t testing.TB, app *tests.TestApp, tableName string) map[string]string {
	t.Helper()

	rows := []struct {
		IndexName string `db:"indexname"`
		IndexDef  string `db:"indexdef"`
	}{}

	err := app.DB().NewQuery(`
		SELECT indexname, indexdef
		FROM pg_indexes
		WHERE tablename = {:tableName}
	`).Bind(dbx.Params{"tableName": tableName}).All(&rows)
	if err != nil {
		t.Fatal(err)
	}

	result := make(map[string]string, len(rows))
	for _, r := range rows {
		result[r.IndexName] = r.IndexDef
	}

	return result
}

// explainWithSeqScanDisabled returns the EXPLAIN output for the exact auth
// identity lookup shape (LOWER(field) = LOWER($1) AND field <> '') with the
// planner forced to consider index scans (enable_seqscan = off). This proves
// the functional index is USABLE for the lookup, independent of table size.
func explainWithSeqScanDisabled(t testing.TB, app *tests.TestApp, tableName string, fieldName string) string {
	t.Helper()

	if _, err := app.DB().NewQuery(`SET enable_seqscan = off`).Execute(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		app.DB().NewQuery(`SET enable_seqscan = on`).Execute()
	})

	query := `SELECT value FROM (
		SELECT 'id' AS value
		FROM "` + strings.ToLower(tableName) + `"
		WHERE LOWER("` + strings.ToLower(fieldName) + `") = LOWER('testuser')
		  AND "` + strings.ToLower(fieldName) + `" <> ''
	) t LIMIT 0`

	rows, err := app.DB().NewQuery(`EXPLAIN (COSTS OFF) ` + query).Rows()
	if err != nil {
		t.Fatalf("EXPLAIN failed for %s.%s: %v", tableName, fieldName, err)
	}
	defer rows.Close()

	var sb strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("failed to scan EXPLAIN row: %v", err)
		}
		sb.WriteString(line)
		sb.WriteString("\n")
	}

	return sb.String()
}

// TestIdentityLoginLookupsUseIndexScan is the EXPLAIN-based regression guard
// for D-1 / IDX-1. It asserts two things per identity lookup:
//
//  1. a functional (LOWER(...)) single-column index exists for the field, and
//  2. the auth lookup shape can be served by an Index Scan (not a Seq Scan)
//     when the planner is allowed to consider indexes.
func TestIdentityLoginLookupsUseIndexScan(t *testing.T) {
	t.Parallel()

	app, _ := tests.NewTestApp()
	defer app.Cleanup()

	scenarios := []struct {
		tableName string
		fieldName string
	}{
		{"users", "email"},
		{"users", "username"},
		{"clients", "username"},
	}

	indexes := functionalIndexesOnTable(t, app, "users")
	clientsIndexes := functionalIndexesOnTable(t, app, "clients")

	for _, s := range scenarios {
		t.Run(s.tableName+"-"+s.fieldName, func(t *testing.T) {
			// 1. a functional index must cover the identity field
			all := indexes
			if s.tableName == "clients" {
				all = clientsIndexes
			}
			var functionalFound bool
			for _, def := range all {
				lowerDef := strings.ToLower(def)
				if strings.Contains(lowerDef, "lower("+strings.ToLower(s.fieldName)) && strings.Contains(lowerDef, "unique") {
					functionalFound = true
					break
				}
			}
			if !functionalFound {
				t.Fatalf("expected a functional unique index on %s.%s, got: %v", s.tableName, s.fieldName, all)
			}

			// 2. the lookup must be able to use an index scan
			plan := explainWithSeqScanDisabled(t, app, s.tableName, s.fieldName)
			if strings.Contains(plan, "Seq Scan") {
				t.Fatalf("expected an index-servable lookup for %s.%s, got a sequential scan:\n%s", s.tableName, s.fieldName, plan)
			}
			if !strings.Contains(plan, "Index Scan") {
				t.Fatalf("expected an index scan for identity lookup %s.%s, plan:\n%s", s.tableName, s.fieldName, plan)
			}
		})
	}
}
package apis_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/pocketbase/dbx"
)

// valid seed tokens re-signed by the test harness (see tests/api.go).
const (
	auditSuperuserToken = "eyJhbGciOiJIUzI1NiJ9.eyJpZCI6InN5d2JoZWNuaDQ2cmhtMCIsInR5cGUiOiJhdXRoIiwiY29sbGVjdGlvbklkIjoicGJjXzMxNDI2MzU4MjMiLCJleHAiOjI1MjQ2MDQ0NjEsInJlZnJlc2hhYmxlIjp0cnVlfQ.UXgO3j-0BumcugrFjbd7j0M4MQvbrLggLlcu_YNGjoY"
	auditUserToken      = "eyJhbGciOiJIUzI1NiJ9.eyJpZCI6IjRxMXhsY2xtZmxva3UzMyIsInR5cGUiOiJhdXRoIiwiY29sbGVjdGlvbklkIjoiX3BiX3VzZXJzX2F1dGhfIiwiZXhwIjoyNTI0NjA0NDYxLCJyZWZyZXNoYWJsZSI6dHJ1ZX0.ZT3F0Z3iM-xbGgSG3LEKiEzHrPHr8t8IuHLZGGNuxLo"
)

// seedWriteAudit enables the write trail for demo1 and creates one record
// (producing a single source=system audit row).
func seedWriteAudit(t testing.TB, app *tests.TestApp) {
	s := app.Settings()
	s.Audit.Enabled = true
	s.Audit.Collections = []string{"demo1"}

	col, err := app.FindCollectionByNameOrId("demo1")
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("text", "seeded-audit")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
}

func TestAuditsApiList(t *testing.T) {
	t.Parallel()

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthorized",
			Method:          http.MethodGet,
			URL:             "/api/audits",
			ExpectedStatus:  401,
			ExpectedContent: []string{`"data":{}`},
			ExpectedEvents:  map[string]int{"*": 0},
		},
		{
			Name:            "regular user (forbidden)",
			Method:          http.MethodGet,
			URL:             "/api/audits",
			Headers:         map[string]string{"Authorization": auditUserToken},
			ExpectedStatus:  403,
			ExpectedContent: []string{`"data":{}`},
			ExpectedEvents:  map[string]int{"*": 0},
		},
		{
			Name:           "superuser",
			Method:         http.MethodGet,
			URL:            "/api/audits",
			Headers:        map[string]string{"Authorization": auditSuperuserToken},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) { seedWriteAudit(t, app) },
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`"totalItems":1`,
				`"collection_name":"demo1"`,
				`"event":"create"`,
			},
			ExpectedEvents: map[string]int{"*": 0},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuditsApiView(t *testing.T) {
	t.Parallel()

	scenarios := []tests.ApiScenario{
		{
			Name:            "missing id as superuser",
			Method:          http.MethodGet,
			URL:             "/api/audits/nonexistent000000",
			Headers:         map[string]string{"Authorization": auditSuperuserToken},
			ExpectedStatus:  404,
			ExpectedContent: []string{`"data":{}`},
			ExpectedEvents:  map[string]int{"*": 0},
		},
		{
			Name:            "unauthorized",
			Method:          http.MethodGet,
			URL:             "/api/audits/nonexistent000000",
			ExpectedStatus:  401,
			ExpectedContent: []string{`"data":{}`},
			ExpectedEvents:  map[string]int{"*": 0},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func TestAuditReadsApiList(t *testing.T) {
	t.Parallel()

	scenarios := []tests.ApiScenario{
		{
			Name:            "unauthorized",
			Method:          http.MethodGet,
			URL:             "/api/audits/reads",
			ExpectedStatus:  401,
			ExpectedContent: []string{`"data":{}`},
			ExpectedEvents:  map[string]int{"*": 0},
		},
		{
			Name:    "superuser",
			Method:  http.MethodGet,
			URL:     "/api/audits/reads",
			Headers: map[string]string{"Authorization": auditSuperuserToken},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				_, err := app.DB().NewQuery(
					`INSERT INTO "_audit_reads" ("collection_name","event","auth_id","source","filter")
					 VALUES ('demo1','list','someauthid','request','myfilter')`,
				).Execute()
				if err != nil {
					t.Fatal(err)
				}
			},
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`"totalItems":1`,
				`"collection_name":"demo1"`,
				`"event":"list"`,
				`"filter":"myfilter"`,
			},
			ExpectedEvents: map[string]int{"*": 0},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

// TestAuditWriteActorViaApi asserts the request actor is bridged onto the
// Execute-hook audit row (source=request + auth populated).
func TestAuditWriteActorViaApi(t *testing.T) {
	t.Parallel()

	scenario := tests.ApiScenario{
		Name:    "superuser create demo1 record",
		Method:  http.MethodPost,
		URL:     "/api/collections/demo1/records",
		Body:    strings.NewReader(`{"text":"api-created"}`),
		Headers: map[string]string{"Authorization": auditSuperuserToken},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			s := app.Settings()
			s.Audit.Enabled = true
			s.Audit.Collections = []string{"demo1"}
		},
		ExpectedStatus:  200,
		ExpectedContent: []string{`"text":"api-created"`},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			audit := &core.Audit{}
			err := app.AuditQuery().
				AndWhere(dbx.HashExp{"collection_name": "demo1", "event": "create"}).
				OrderBy("created DESC").
				Limit(1).
				One(audit)
			if err != nil {
				t.Fatalf("expected an audit row: %v", err)
			}
			if audit.Source != "request" {
				t.Fatalf("expected source=request, got %q", audit.Source)
			}
			if audit.AuthCollection != "_superusers" {
				t.Fatalf("expected auth_collection=_superusers, got %q", audit.AuthCollection)
			}
			if audit.AuthId == "" {
				t.Fatal("expected a non-empty auth_id")
			}
		},
	}

	scenario.Test(t)
}

func TestAuditReadTrailList(t *testing.T) {
	t.Parallel()

	scenario := tests.ApiScenario{
		Name:    "superuser list demo1 records",
		Method:  http.MethodGet,
		URL:     "/api/collections/demo1/records?filter=number>0&sort=-created&page=1&perPage=5",
		Headers: map[string]string{"Authorization": auditSuperuserToken},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			s := app.Settings()
			s.Audit.ReadEnabled = true
			s.Audit.Collections = []string{"demo1"}
		},
		ExpectedStatus:  200,
		ExpectedContent: []string{`"items":[`},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			core.FlushAuditReads(app)

			reads := []*core.AuditRead{}
			err := app.ModelQuery(&core.AuditRead{}).
				AndWhere(dbx.HashExp{"collection_name": "demo1", "event": "list"}).
				All(&reads)
			if err != nil {
				t.Fatal(err)
			}
			if len(reads) != 1 {
				t.Fatalf("expected 1 list read audit, got %d", len(reads))
			}

			r := reads[0]
			if r.Filter != "number>0" {
				t.Fatalf("expected filter=number>0, got %q", r.Filter)
			}
			if r.Sort != "-created" {
				t.Fatalf("expected sort=-created, got %q", r.Sort)
			}
			if r.Page != 1 || r.PerPage != 5 {
				t.Fatalf("expected page=1 perPage=5, got page=%d perPage=%d", r.Page, r.PerPage)
			}
			if r.TotalItems < 1 {
				t.Fatalf("expected total_items>=1, got %d", r.TotalItems)
			}
			if r.AuthCollection != "_superusers" || r.AuthId == "" {
				t.Fatalf("expected superuser actor, got coll=%q id=%q", r.AuthCollection, r.AuthId)
			}
		},
	}

	scenario.Test(t)
}

func TestAuditReadTrailView(t *testing.T) {
	t.Parallel()

	scenario := tests.ApiScenario{
		Name:    "superuser view demo1 record",
		Method:  http.MethodGet,
		URL:     "/api/collections/demo1/records/imy661ixudk5izi",
		Headers: map[string]string{"Authorization": auditSuperuserToken},
		BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
			s := app.Settings()
			s.Audit.ReadEnabled = true
			s.Audit.Collections = []string{"demo1"}
		},
		ExpectedStatus:  200,
		ExpectedContent: []string{`"id":"imy661ixudk5izi"`},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			core.FlushAuditReads(app)

			reads := []*core.AuditRead{}
			err := app.ModelQuery(&core.AuditRead{}).
				AndWhere(dbx.HashExp{"collection_name": "demo1", "event": "view"}).
				All(&reads)
			if err != nil {
				t.Fatal(err)
			}
			if len(reads) != 1 {
				t.Fatalf("expected 1 view read audit, got %d", len(reads))
			}
			if reads[0].RecordId != "imy661ixudk5izi" {
				t.Fatalf("expected record_id=imy661ixudk5izi, got %q", reads[0].RecordId)
			}
		},
	}

	scenario.Test(t)
}

func TestAuditReadTrailSkips(t *testing.T) {
	t.Parallel()

	scenarios := []tests.ApiScenario{
		{
			Name:    "read disabled -> no rows",
			Method:  http.MethodGet,
			URL:     "/api/collections/demo1/records",
			Headers: map[string]string{"Authorization": auditSuperuserToken},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				s := app.Settings()
				s.Audit.ReadEnabled = false
				s.Audit.Collections = []string{"demo1"}
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items":[`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				core.FlushAuditReads(app)
				assertAuditReadsCount(t, app, "demo1", 0)
			},
		},
		{
			Name:    "non-allowlisted collection -> no rows",
			Method:  http.MethodGet,
			URL:     "/api/collections/demo2/records",
			Headers: map[string]string{"Authorization": auditSuperuserToken},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				s := app.Settings()
				s.Audit.ReadEnabled = true
				s.Audit.Collections = []string{"demo1"} // demo2 NOT included
			},
			ExpectedStatus:  200,
			ExpectedContent: []string{`"items":[`},
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
				core.FlushAuditReads(app)
				assertAuditReadsCount(t, app, "demo2", 0)
			},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

func assertAuditReadsCount(t testing.TB, app *tests.TestApp, collection string, expected int) {
	var n int
	err := app.ModelQuery(&core.AuditRead{}).
		Select("COUNT(*)").
		AndWhere(dbx.HashExp{"collection_name": collection}).
		Row(&n)
	if err != nil {
		t.Fatal(err)
	}
	if n != expected {
		t.Fatalf("expected %d read audits for %q, got %d", expected, collection, n)
	}
}

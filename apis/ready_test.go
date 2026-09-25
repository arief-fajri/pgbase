package apis_test

import (
	"net/http"
	"testing"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tests"
	"github.com/pocketbase/dbx"
)

// closeDataPool simulates a database outage by closing the app's data pool:
// subsequent queries fail fast with "sql: database is closed" while the
// process keeps serving (liveness intact).
func closeDataPool(t testing.TB, app *tests.TestApp) {
	t.Helper()

	db, ok := app.DB().(*dbx.DB)
	if !ok {
		t.Fatalf("expected the data pool to be *dbx.DB, got %T", app.DB())
	}
	if err := db.DB().Close(); err != nil {
		t.Fatalf("failed to close the data pool: %v", err)
	}
}

// TestReadyAPI verifies the readiness endpoint: 200 while the data pool can
// query the core schema, 503 when it cannot — while /api/health stays
// liveness-only (200 even with the database down).
func TestReadyAPI(t *testing.T) {
	t.Parallel()

	scenarios := []tests.ApiScenario{
		{
			Name:           "GET ready status (guest, db healthy)",
			Method:         http.MethodGet,
			URL:            "/api/ready",
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`"code":200`,
				`"message":"API is ready."`,
				`"data":{}`,
			},
			NotExpectedContent: []string{
				"canBackup",
				"realIP",
				"possibleProxyHeader",
			},
			ExpectedEvents: map[string]int{"*": 0},
		},
		{
			Name:   "GET ready status (superuser, db healthy)",
			Method: http.MethodGet,
			URL:    "/api/ready",
			Headers: map[string]string{
				"Authorization": "eyJhbGciOiJIUzI1NiJ9.eyJpZCI6IjNiZWNuaDQ2cmhtMCIsInR5cGUiOiJhdXRoIiwiY29sbGVjdGlvbklkIjoicGJjXzMxNDI2MzU4MjMiLCJleHAiOjI1MjQ2MDQ0NjEsInJlZnJlc2hhYmxlIjp0cnVlfQ.UXgO3j-0BumcugrFjbd7j0M4MQvbrLggLlcu_YNGjoY",
			},
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`"code":200`,
				`"message":"API is ready."`,
				`"data":{}`,
			},
			NotExpectedContent: []string{
				// readiness is intentionally superuser-free (unlike /api/health)
				"canBackup",
				"realIP",
				"possibleProxyHeader",
			},
			ExpectedEvents: map[string]int{"*": 0},
		},
		{
			Name:           "GET ready status (db closed)",
			Method:         http.MethodGet,
			URL:            "/api/ready",
			ExpectedStatus: 503,
			ExpectedContent: []string{
				`"message":"API is not ready."`,
				`"status":503`,
			},
			NotExpectedContent: []string{
				// the raw driver error must never leak into the response body
				// (it can contain connection details) — regression guard for
				// the static-message decision in DRR-0002
				"sql: database is closed",
				"failed to connect",
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				// simulate a database outage: the readiness probe must fail
				// fast with 503 while the process keeps serving (liveness)
				closeDataPool(t, app)
			},
			ExpectedEvents: map[string]int{"*": 0},
		},
		{
			Name:           "GET health status (db closed) stays liveness",
			Method:         http.MethodGet,
			URL:            "/api/health",
			ExpectedStatus: 200,
			ExpectedContent: []string{
				`"code":200`,
			},
			BeforeTestFunc: func(t testing.TB, app *tests.TestApp, e *core.ServeEvent) {
				// liveness must not depend on the database (readiness split,
				// Phase 0): the health endpoint answers 200 even mid-outage
				closeDataPool(t, app)
			},
			ExpectedEvents: map[string]int{"*": 0},
		},
	}

	for _, scenario := range scenarios {
		scenario.Test(t)
	}
}

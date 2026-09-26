package apis

import (
	"context"
	"net/http"
	"time"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tools/router"
)

// readyProbeTimeout bounds how long the readiness probe waits for the database
// check before answering 503. It stays well below the server-side
// statement_timeout and the app-level query timeout so an unreachable database
// turns into a fast "not ready" instead of a hanging handler (it also matches
// the compose healthcheck timeout in docker-compose.prod.yml).
const readyProbeTimeout = 5 * time.Second

// bindReadyApi registers the readiness api endpoint.
//
// Readiness is deliberately distinct from liveness (bindHealthApi):
// /api/health answers 200 as long as the process serves requests — a load
// balancer must not kill a healthy process just because the database is
// down — while /api/ready answers 503 until the app can actually serve:
// the data pool can execute a query and the core schema is readable.
// Orchestrators and load balancers should probe /api/ready for readiness
// and /api/health for liveness.
func bindReadyApi(app core.App, rg *router.RouterGroup[*core.RequestEvent]) {
	subGroup := rg.Group("/ready")
	subGroup.GET("", readyCheck)
}

// readyCheck answers 200 when the app is ready to serve traffic.
//
// On failure it responds with a static 503 message; the raw error is logged
// server-side only and never leaked into the response body, because driver
// errors can contain connection details (host, user, database).
func readyCheck(e *core.RequestEvent) error {
	execCtx, cancel := context.WithTimeout(e.Request.Context(), readyProbeTimeout)
	defer cancel()

	// a single round trip proves three things: the data pool can acquire a
	// connection, the server answers, and the core `_collections` table is
	// readable (i.e. migrations have applied).
	var collections int64
	if err := e.App.ConcurrentDB().NewQuery(
		`SELECT (SELECT count(*) FROM "_collections")`,
	).WithContext(execCtx).Row(&collections); err != nil {
		e.App.Logger().Warn("Readiness probe failed", "error", err.Error())
		return NewApiError(http.StatusServiceUnavailable, "API is not ready.", nil)
	}

	resp := struct {
		Message string         `json:"message"`
		Code    int            `json:"code"`
		Data    map[string]any `json:"data"`
	}{
		Code:    http.StatusOK,
		Message: "API is ready.",
		Data:    map[string]any{}, // ensure that it is returned as object
	}

	return e.JSON(http.StatusOK, resp)
}

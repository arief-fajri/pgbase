package apis

import (
	"net/http"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tools/router"
	"github.com/arief-fajri/pgbase/tools/search"
)

// bindAuditsApi registers the audit trail api endpoints (superuser-only).
func bindAuditsApi(app core.App, rg *router.RouterGroup[*core.RequestEvent]) {
	sub := rg.Group("/audits").Bind(RequireSuperuserAuth(), SkipSuccessActivityLog())
	sub.GET("", auditsList)
	sub.GET("/reads", auditReadsList)
	sub.GET("/{id}", auditsView)
}

// auditFilterFields lists the columns filterable on the write trail. The JSONB
// columns are only reachable via a path (e.g. changes.status) — a bare
// "changes"/"snapshot" would compile to `jsonb = text` (operator does not exist),
// so they are intentionally NOT exposed as scalar filter fields.
var auditFilterFields = []string{
	"id", "created", "collection_name", "record_id", "event",
	"auth_id", "auth_collection", "source",
	`^changes\.[\w\.\:]*\w+$`,
}

// auditReadFilterFields lists the columns filterable on the read trail (all
// scalar → safe).
var auditReadFilterFields = []string{
	"id", "created", "collection_name", "record_id", "event",
	"auth_id", "source", "filter",
}

func auditsList(e *core.RequestEvent) error {
	fieldResolver := search.NewSimpleFieldResolver(auditFilterFields...)

	// explicit column list omits "snapshot" for list perf (Select replaces
	// tableName.* — the provider preserves the projection). Full snapshot is
	// only returned by auditsView.
	query := e.App.AuditQuery().Select(
		"id", "created", "collection_name", "record_id", "event",
		"auth_id", "auth_collection", "source", "changes", "user_ip", "user_agent",
	)

	provider := search.NewProvider(fieldResolver).Query(query)

	if err := provider.Parse(e.Request.URL.Query().Encode()); err != nil {
		return e.BadRequestError("", err)
	}

	// newest-first fallback ordering (secondary to any caller-supplied sort)
	provider.AddSort(search.SortField{Name: "created", Direction: search.SortDesc})

	result, err := provider.Exec(&[]*core.Audit{})
	if err != nil {
		return e.BadRequestError("", err)
	}

	return e.JSON(http.StatusOK, result)
}

func auditReadsList(e *core.RequestEvent) error {
	fieldResolver := search.NewSimpleFieldResolver(auditReadFilterFields...)

	provider := search.NewProvider(fieldResolver).
		Query(e.App.ModelQuery(&core.AuditRead{}))

	if err := provider.Parse(e.Request.URL.Query().Encode()); err != nil {
		return e.BadRequestError("", err)
	}

	provider.AddSort(search.SortField{Name: "created", Direction: search.SortDesc})

	result, err := provider.Exec(&[]*core.AuditRead{})
	if err != nil {
		return e.BadRequestError("", err)
	}

	return e.JSON(http.StatusOK, result)
}

func auditsView(e *core.RequestEvent) error {
	id := e.Request.PathValue("id")
	if id == "" {
		return e.NotFoundError("", nil)
	}

	audit, err := e.App.FindAuditById(id)
	if err != nil || audit == nil {
		return e.NotFoundError("", err)
	}

	return e.JSON(http.StatusOK, audit)
}

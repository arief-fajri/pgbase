package core

import "github.com/pocketbase/dbx"

// AuditQuery returns a new Audit select query.
func (app *BaseApp) AuditQuery() *dbx.SelectQuery {
	return app.ModelQuery(&Audit{})
}

// FindAuditById finds a single Audit entry by its id.
//
// Note: the "_audits" table is partitioned by "created", so a lookup by id
// alone scans all partitions (no pruning) — acceptable for admin-only reads.
func (app *BaseApp) FindAuditById(id string) (*Audit, error) {
	model := &Audit{}

	err := app.AuditQuery().
		AndWhere(dbx.HashExp{"id": id}).
		Limit(1).
		One(model)

	if err != nil {
		return nil, err
	}

	return model, nil
}

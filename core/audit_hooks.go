package core

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/arief-fajri/pgbase/tools/hook"
	"github.com/arief-fajri/pgbase/tools/types"
	"github.com/pocketbase/dbx"
)

// auditActors bridges the request actor (auth/ip/ua) captured in the record
// request hooks to the corresponding Execute hook. It is keyed by the *Record
// pointer, whose identity is stable across request → Save → Execute (verified
// core/events.go newRecordEventFromModelEvent). Package-level is safe because
// the keys are unique per in-flight record.
var auditActors = newAuditActorStore()

type auditActor struct {
	id, coll, ip, ua, source string
}

type auditActorStore struct {
	m sync.Map // *Record -> auditActor
}

func newAuditActorStore() *auditActorStore {
	return &auditActorStore{}
}

func (s *auditActorStore) store(rec *Record, a auditActor) { s.m.Store(rec, a) }
func (s *auditActorStore) delete(rec *Record)              { s.m.Delete(rec) }
func (s *auditActorStore) load(rec *Record) (auditActor, bool) {
	if v, ok := s.m.Load(rec); ok {
		return v.(auditActor), true
	}
	return auditActor{}, false
}

// registerAuditHooks wires the write (create/update/delete) and read
// (view/list) audit trail for the app. It is called once from
// registerBaseHooks (constructor time).
//
// The read-audit writer is created here and stored on the app (via app.Store),
// which keeps it per-app/per-DB — required for correctness under parallel tests
// where each app owns a separate database.
func (app *BaseApp) registerAuditHooks() {
	writer := newAuditReadWriter(app)
	app.Store().Set(auditReadWriterStoreKey, writer)

	// -----------------------------------------------------------------
	// Write trail — Execute hooks (best-effort; INSERT after e.Next()).
	//
	// Default priority (0) is intentionally < core's delete-execute handler
	// (Priority 99, core/record_model.go:446) so the audit hook runs OUTER:
	//   - top-level delete: e.App is restored to the non-tx app before our
	//     e.Next() returns → plain autocommit INSERT.
	//   - cascaded/batch/user-tx writes: e.App is transactional → the INSERT
	//     is SAVEPOINT-guarded so a failure can't poison the outer PG tx.
	// -----------------------------------------------------------------

	app.OnRecordCreateExecute().Bind(&hook.Handler[*RecordEvent]{
		Id: "__pbAuditCreateExecute__",
		Func: func(e *RecordEvent) error {
			if err := e.Next(); err != nil {
				return err
			}
			writeAudit(e, "create")
			return nil
		},
	})

	app.OnRecordUpdateExecute().Bind(&hook.Handler[*RecordEvent]{
		Id: "__pbAuditUpdateExecute__",
		Func: func(e *RecordEvent) error {
			if err := e.Next(); err != nil {
				return err
			}
			writeAudit(e, "update")
			return nil
		},
	})

	app.OnRecordDeleteExecute().Bind(&hook.Handler[*RecordEvent]{
		Id: "__pbAuditDeleteExecute__",
		Func: func(e *RecordEvent) error {
			if err := e.Next(); err != nil {
				return err
			}
			writeAudit(e, "delete")
			return nil
		},
	})

	// -----------------------------------------------------------------
	// Actor bridge — request hooks store the actor keyed by *Record.
	// -----------------------------------------------------------------

	app.OnRecordCreateRequest().Bind(&hook.Handler[*RecordRequestEvent]{
		Id:   "__pbAuditCreateRequest__",
		Func: auditStoreRequestActor,
	})
	app.OnRecordUpdateRequest().Bind(&hook.Handler[*RecordRequestEvent]{
		Id:   "__pbAuditUpdateRequest__",
		Func: auditStoreRequestActor,
	})
	app.OnRecordDeleteRequest().Bind(&hook.Handler[*RecordRequestEvent]{
		Id:   "__pbAuditDeleteRequest__",
		Func: auditStoreRequestActor,
	})

	// -----------------------------------------------------------------
	// Read trail — view/list request hooks → batched writer (best-effort).
	// -----------------------------------------------------------------

	app.OnRecordViewRequest().Bind(&hook.Handler[*RecordRequestEvent]{
		Id: "__pbAuditReadView__",
		Func: func(e *RecordRequestEvent) error {
			if err := e.Next(); err != nil {
				return err
			}
			if r := buildViewReadAudit(e); r != nil {
				writer.enqueue(r)
			}
			return nil
		},
	})

	app.OnRecordsListRequest().Bind(&hook.Handler[*RecordsListRequestEvent]{
		Id: "__pbAuditReadList__",
		Func: func(e *RecordsListRequestEvent) error {
			if err := e.Next(); err != nil {
				return err
			}
			if r := buildListReadAudit(e); r != nil {
				writer.enqueue(r)
			}
			return nil
		},
	})

	// -----------------------------------------------------------------
	// Lifecycle — start the batched writer's ticker on serve, flush+stop
	// on terminate. (Tests never Serve → ticker never starts → tests drain
	// deterministically via FlushAuditReads.)
	// -----------------------------------------------------------------

	app.OnServe().Bind(&hook.Handler[*ServeEvent]{
		Id: "__pbAuditReadWriterStart__",
		Func: func(e *ServeEvent) error {
			writer.start()
			return e.Next()
		},
	})

	app.OnTerminate().Bind(&hook.Handler[*TerminateEvent]{
		Id: "__pbAuditReadWriterStop__",
		Func: func(e *TerminateEvent) error {
			writer.stop()
			return e.Next()
		},
		Priority: -999,
	})

	// -----------------------------------------------------------------
	// Partition management + retention crons (run only while serving —
	// cron starts on OnServe). Settings are read at tick time (live reloads).
	// -----------------------------------------------------------------

	app.Cron().Add("__pbAuditsPartition__", "0 0 * * *", func() {
		err := app.runWithCronLock("__pbAuditsPartition__", app.Logger(), func() error {
			return ensureAuditPartitions(app)
		})
		if err != nil {
			app.Logger().Warn("Failed to ensure audit partitions", "error", err.Error())
		}
	})

	app.Cron().Add("__pbAuditsCleanup__", "0 0 * * *", func() {
		err := app.runWithCronLock("__pbAuditsCleanup__", app.Logger(), func() error {
			s := app.Settings().Audit
			if s.RetentionDays > 0 {
				if err := dropOldAuditPartitions(app, AuditsTableName, s.RetentionDays); err != nil {
					return err
				}
			}
			if s.ReadRetentionDays > 0 {
				if err := dropOldAuditPartitions(app, AuditReadsTableName, s.ReadRetentionDays); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			app.Logger().Warn("Failed to cleanup old audit partitions", "error", err.Error())
		}
	})
}

// -------------------------------------------------------------------
// Write trail
// -------------------------------------------------------------------

func writeAudit(e *RecordEvent, event string) {
	s := e.App.Settings().Audit
	if !s.Enabled {
		return
	}

	coll := e.Record.Collection()
	if !auditCollectionAllowed(s, coll) {
		return
	}

	var changes, snapshot types.JSONMap[any]

	switch event {
	case "create":
		snapshot = redactFields(coll, e.Record.FieldsData())
		changes = types.JSONMap[any]{}
	case "update":
		old := redactFields(coll, e.Record.Original().FieldsData())
		cur := redactFields(coll, e.Record.FieldsData())
		changes = diffFields(old, cur)
		if len(changes) == 0 {
			return // nothing meaningful changed
		}
		snapshot = types.JSONMap[any]{}
	case "delete":
		snapshot = redactFields(coll, e.Record.FieldsData())
		changes = types.JSONMap[any]{}
	default:
		return
	}

	actor, ok := auditActors.load(e.Record)
	if !ok {
		actor.source = "system"
	}

	ip, ua := actor.ip, actor.ua
	if !s.LogIP {
		ip, ua = "", ""
	}

	params := dbx.Params{
		"collection_name": coll.Name,
		"record_id":       e.Record.Id,
		"event":           event,
		"auth_id":         actor.id,
		"auth_collection": actor.coll,
		"source":          actor.source,
		"changes":         changes,  // types.JSONMap[any] (driver.Valuer)
		"snapshot":        snapshot, // types.JSONMap[any] (driver.Valuer)
		"user_ip":         ip,
		"user_agent":      ua,
		// "created" omitted → SQL DEFAULT NOW()
	}

	if e.App.IsTransactional() {
		insertAuditWithSavepoint(e, params, coll.Name, event)
		return
	}

	if _, err := e.App.NonconcurrentDB().Insert(AuditsTableName, params).WithContext(e.Context).Execute(); err != nil {
		e.App.Logger().Error("audit write failed", "error", err.Error(),
			"collection", coll.Name, "record_id", e.Record.Id, "event", event)
	}
}

// insertAuditWithSavepoint performs the audit INSERT inside a SAVEPOINT so that
// an in-transaction failure (cascade delete / batch / user RunInTransaction) is
// isolated and does NOT poison the surrounding PG tx (which would roll back the
// data write). The data change is always preserved (best-effort audit).
func insertAuditWithSavepoint(e *RecordEvent, params dbx.Params, collName, event string) {
	db := e.App.NonconcurrentDB() // same tx connection

	if _, err := db.NewQuery("SAVEPOINT pb_audit").WithContext(e.Context).Execute(); err != nil {
		// couldn't establish a savepoint — bail without touching the tx
		e.App.Logger().Error("audit savepoint failed", "error", err.Error(),
			"collection", collName, "record_id", e.Record.Id, "event", event)
		return
	}

	if _, err := db.Insert(AuditsTableName, params).WithContext(e.Context).Execute(); err != nil {
		if _, rbErr := db.NewQuery("ROLLBACK TO SAVEPOINT pb_audit").WithContext(e.Context).Execute(); rbErr != nil {
			e.App.Logger().Error("audit rollback-to-savepoint failed", "error", rbErr.Error())
		}
		e.App.Logger().Error("audit write failed (rolled back to savepoint)", "error", err.Error(),
			"collection", collName, "record_id", e.Record.Id, "event", event)
		return
	}

	if _, err := db.NewQuery("RELEASE SAVEPOINT pb_audit").WithContext(e.Context).Execute(); err != nil {
		e.App.Logger().Error("audit release-savepoint failed", "error", err.Error())
	}
}

// redactFields returns a shallow copy of data with secret fields removed
// (hidden fields, password-type fields, and the well-known "password"/"tokenKey"
// names). Applied to BOTH snapshot and the pre-diff old/new maps so secrets
// never leak into the trail.
func redactFields(coll *Collection, data map[string]any) types.JSONMap[any] {
	result := make(types.JSONMap[any], len(data))
	for k, v := range data {
		result[k] = v
	}

	if coll != nil {
		for _, f := range coll.Fields {
			name := f.GetName()
			if f.GetHidden() || f.Type() == FieldTypePassword {
				delete(result, name)
			}
		}
	}

	delete(result, "password")
	delete(result, "tokenKey")

	return result
}

// diffFields builds a {field:{old,new}} map for keys whose values differ.
func diffFields(old, cur types.JSONMap[any]) types.JSONMap[any] {
	changes := types.JSONMap[any]{}

	for k, curVal := range cur {
		if !reflect.DeepEqual(old[k], curVal) {
			changes[k] = map[string]any{"old": old[k], "new": curVal}
		}
	}

	// keys present in old but removed from cur (rare — field set is stable)
	for k, oldVal := range old {
		if _, exists := cur[k]; !exists {
			changes[k] = map[string]any{"old": oldVal, "new": nil}
		}
	}

	return changes
}

// -------------------------------------------------------------------
// Actor bridge
// -------------------------------------------------------------------

func auditStoreRequestActor(e *RecordRequestEvent) error {
	actor := auditActor{
		source: "request",
		ip:     e.RealIP(),
		ua:     e.Request.UserAgent(),
	}
	if e.Auth != nil {
		actor.id = e.Auth.Id
		if ac := e.Auth.Collection(); ac != nil {
			actor.coll = ac.Name
		}
	}

	auditActors.store(e.Record, actor)
	defer auditActors.delete(e.Record)

	return e.Next() // form.Submit() + Execute hooks run within here
}

// -------------------------------------------------------------------
// Read trail
// -------------------------------------------------------------------

func buildViewReadAudit(e *RecordRequestEvent) *AuditRead {
	s := e.App.Settings().Audit
	if !s.ReadEnabled || e.Record == nil || !auditCollectionAllowed(s, e.Collection) {
		return nil
	}

	r := &AuditRead{
		CollectionName: e.Collection.Name,
		RecordId:       e.Record.Id,
		Event:          "view",
		Source:         "request",
	}
	fillReadActor(r, e.RequestEvent, s)

	return r
}

func buildListReadAudit(e *RecordsListRequestEvent) *AuditRead {
	s := e.App.Settings().Audit
	if !s.ReadEnabled || !auditCollectionAllowed(s, e.Collection) {
		return nil
	}

	q := e.Request.URL.Query()
	r := &AuditRead{
		CollectionName: e.Collection.Name,
		Event:          "list",
		Source:         "request",
		Filter:         q.Get("filter"),
		Sort:           q.Get("sort"),
		Page:           auditAtoi(q.Get("page")),
		PerPage:        auditAtoi(q.Get("perPage")),
	}
	if e.Result != nil {
		r.TotalItems = e.Result.TotalItems
	}
	fillReadActor(r, e.RequestEvent, s)

	return r
}

func fillReadActor(r *AuditRead, e *RequestEvent, s AuditConfig) {
	if e.Auth != nil {
		r.AuthId = e.Auth.Id
		if ac := e.Auth.Collection(); ac != nil {
			r.AuthCollection = ac.Name
		}
	}
	if s.LogIP {
		r.UserIP = e.RealIP()
		r.UserAgent = e.Request.UserAgent()
	}
}

// -------------------------------------------------------------------
// Shared helpers
// -------------------------------------------------------------------

// auditCollectionAllowed reports whether the collection is in the audit
// allowlist and is not an internal/system/view collection.
func auditCollectionAllowed(s AuditConfig, coll *Collection) bool {
	if coll == nil || coll.System || coll.IsView() {
		return false
	}
	if coll.Name == AuditsTableName || coll.Name == AuditReadsTableName {
		return false
	}
	return slices.Contains(s.Collections, coll.Name)
}

func auditAtoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// -------------------------------------------------------------------
// Partition management
// -------------------------------------------------------------------

// ensureAuditPartitions pre-creates the next two monthly partitions for both
// audit tables so future inserts route to pruneable named partitions. It only
// provisions FUTURE months (never the current one) so the range is always empty
// at creation time and never conflicts with existing DEFAULT-partition rows.
// Each statement is best-effort (logged, non-fatal).
func ensureAuditPartitions(app App) error {
	now := time.Now().UTC()
	months := []time.Time{
		now.AddDate(0, 1, 0),
		now.AddDate(0, 2, 0),
	}

	for _, table := range []string{AuditsTableName, AuditReadsTableName} {
		for _, m := range months {
			start := time.Date(m.Year(), m.Month(), 1, 0, 0, 0, 0, time.UTC)
			end := start.AddDate(0, 1, 0)
			partName := fmt.Sprintf("%s_y%04dm%02d", table, start.Year(), int(start.Month()))

			//nolint:gosec // table/partition names are constants + computed dates, not user input
			sql := fmt.Sprintf(
				`CREATE TABLE IF NOT EXISTS %q PARTITION OF %q FOR VALUES FROM ('%s') TO ('%s')`,
				partName, table, start.Format("2006-01-02"), end.Format("2006-01-02"),
			)
			if _, err := app.DB().NewQuery(sql).Execute(); err != nil {
				app.Logger().Warn("Failed to create audit partition", "partition", partName, "error", err.Error())
			}
		}
	}

	return nil
}

type auditPartitionRow struct {
	Name string `db:"relname"`
}

// dropOldAuditPartitions drops named monthly partitions of the given table whose
// entire time range is older than retentionDays. The DEFAULT partition is never
// dropped.
func dropOldAuditPartitions(app App, table string, retentionDays int) error {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

	rows := []auditPartitionRow{}
	err := app.DB().NewQuery(`
		SELECT c.relname
		FROM pg_inherits i
		JOIN pg_class c ON c.oid = i.inhrelid
		JOIN pg_class p ON p.oid = i.inhparent
		WHERE p.relname = {:parent}
	`).Bind(dbx.Params{"parent": table}).All(&rows)
	if err != nil {
		return err
	}

	prefix := table + "_y"
	for _, row := range rows {
		name := row.Name
		if !strings.HasPrefix(name, prefix) {
			continue // skip the DEFAULT partition and anything unexpected
		}

		suffix := strings.TrimPrefix(name, prefix) // "YYYYmMM"
		parts := strings.SplitN(suffix, "m", 2)
		if len(parts) != 2 {
			continue
		}
		year, yErr := strconv.Atoi(parts[0])
		month, mErr := strconv.Atoi(parts[1])
		if yErr != nil || mErr != nil || month < 1 || month > 12 {
			continue
		}

		partEnd := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
		if partEnd.After(cutoff) {
			continue // still (partly) within retention
		}

		//nolint:gosec // name derived from pg_class of our own parent table
		if _, err := app.DB().NewQuery(fmt.Sprintf(`DROP TABLE IF EXISTS %q`, name)).Execute(); err != nil {
			app.Logger().Warn("Failed to drop old audit partition", "partition", name, "error", err.Error())
		}
	}

	return nil
}

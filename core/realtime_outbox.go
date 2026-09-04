package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pocketbase/dbx"
)

// RealtimeOutboxTableName is the DB-backed event log used by the cross-instance
// realtime broadcaster (D-4, step 1: data layer + publisher). Events are rows;
// NOTIFY is only a wake-up signal, so payload size is not bounded by the 8KB
// NOTIFY limit and delete snapshots can carry the full serialized record.
const RealtimeOutboxTableName = "_realtime_outbox"

// realtimeOutboxNotifyChannel is the PostgreSQL NOTIFY channel used to wake up
// other instances' listeners (payload empty - the data lives in the table).
// Exported so the apis listener can LISTEN on it.
const realtimeOutboxNotifyChannel = "pb_realtime_outbox"

// RealtimeOutboxNotifyChannel exposes the NOTIFY channel name to the listener
// layer (apis).
func RealtimeOutboxNotifyChannel() string {
	return realtimeOutboxNotifyChannel
}

// realtimeOutboxAckColumnNote: `processed_at` is intentionally NOT used as an
// ack: the outbox is a *broadcast* queue (every instance consumes every row for
// its own local fanout), so a single per-row ack would let one instance
// "consume" an event before another instance read it. Instances dedup in memory
// per event id and rows are removed by TTL cleanup.

// Realtime outbox event actions.
const (
	RealtimeActionCreate = "create"
	RealtimeActionUpdate = "update"
	RealtimeActionDelete = "delete"
)

// realtimeOutboxEnabled reports whether the cross-instance realtime outbox is
// enabled (PB_REALTIME_OUTBOX). Default off - the single-instance default is
// unchanged and no rows are written.
func (app *BaseApp) realtimeOutboxEnabled() bool {
	return getEnvBool("PB_REALTIME_OUTBOX")
}

// RealtimeOutboxEnabled exposes whether the realtime outbox is enabled.
func (app *BaseApp) RealtimeOutboxEnabled() bool {
	return app.realtimeOutboxEnabled()
}

// PublishRealtimeEvent appends an event to the realtime outbox and wakes other
// instances with a NOTIFY. It is a no-op when the outbox is disabled.
//
// For create/update only identifiers are stored and the receiver re-fetches the
// record by id (record still exists post-commit). For delete the full serialized
// record snapshot is stored, because a re-fetch is impossible after the delete
// commits and a faithful access re-evaluation requires the record data.
func (app *BaseApp) PublishRealtimeEvent(action string, collectionName string, recordId string, snapshot *Record) error {
	if !app.realtimeOutboxEnabled() {
		return nil
	}

	var snapshotJSON []byte
	if action == RealtimeActionDelete && snapshot != nil {
		snapshotJSON = sanitizeOutboxSnapshot(snapshot)
	}

	db := app.NonconcurrentDB()

	// bound the outbox writes with a client-side deadline so they cannot hang
	// indefinitely on a silently dropped connection (see withWriteDeadline)
	execCtx, cancel := withWriteDeadline(context.Background(), app.config.QueryTimeout)
	defer cancel()

	insertSQL := fmt.Sprintf(
		`INSERT INTO "%s" ("action", "collection", "record_id", "snapshot", "origin") VALUES ({:action}, {:collection}, {:recordId}, {:snapshot}, {:origin})`,
		RealtimeOutboxTableName,
	)
	if _, err := db.NewQuery(insertSQL).
		Bind(dbx.Params{
			"action":     action,
			"collection": collectionName,
			"recordId":   recordId,
			"snapshot":   snapshotJSON,
			"origin":     app.realtimeOutboxOrigin,
		}).
		WithContext(execCtx).
		Execute(); err != nil {
		return fmt.Errorf("failed to append realtime outbox event: %w", err)
	}

	// wake up other instances (empty payload; data is in the table)
	_, err := db.NewQuery(fmt.Sprintf("SELECT pg_notify('%s', '')", realtimeOutboxNotifyChannel)).
		WithContext(execCtx).
		Execute()

	return err
}

// sanitizeOutboxSnapshot serializes a deleted record for the realtime outbox,
// explicitly stripping auth secrets. Record serialization already hides
// password/tokenKey for auth collections via PublicExport; this keeps the
// invariant explicit and guards against future serialization changes. The
// snapshot is stored at rest (and included in DB backups), so it must never
// carry password hashes or token keys.
func sanitizeOutboxSnapshot(record *Record) []byte {
	export := record.PublicExport()
	if record.Collection().IsAuth() {
		delete(export, FieldNamePassword)
		delete(export, FieldNameTokenKey)
	}

	raw, _ := json.Marshal(export)

	return raw
}

// realtimeOutboxCleanupCronKey is the hourly cleanup cron for the outbox.
var realtimeOutboxCleanupCronKey = "__pbRealtimeOutboxCleanup__"

// RealtimeOutboxEvent is a single pending cross-instance realtime event.
type RealtimeOutboxEvent struct {
	Id         string
	Action     string
	Collection string
	RecordId   string
	Snapshot   []byte // full serialized record for delete; nil for create/update
	Created    time.Time
}

// realtimeOutboxStabilityLag is a short "settle" window: a listener only
// advances its (created,id) high-water cursor past rows whose created is at
// least this old. Without it, a transaction that started slightly earlier (so
// has a smaller `created`) but commits slightly later could become visible
// AFTER the cursor has already moved past its timestamp and be skipped forever.
// Outbox rows are written by short, single-statement post-commit inserts, so
// their created≈commit time and 1s is a very wide safety margin. It bounds only
// cross-instance delivery latency (the publisher's own clients are still served
// synchronously by the local broadcast).
const realtimeOutboxStabilityLag = 1 * time.Second

// RealtimeOutboxEventsAfter returns up to `limit` settled outbox events strictly
// after the (afterCreated, afterId) cursor, in (created, id) order. A zero
// cursor returns the oldest settled events.
//
// Rows published by THIS instance are excluded (origin = app's own id): the
// publisher already broadcasts to its local clients synchronously at write
// time, so re-delivering its own rows here would double-broadcast. Rows with a
// NULL origin (legacy/unknown publisher) are always returned. Since the filter
// lives in the WHERE clause, self rows never consume a LIMIT slot and never
// stall the cursor.
//
// Each listener advances its own cursor as it processes, so the read window
// always moves forward. This replaces a fixed LIMIT-from-oldest read, which
// stalls permanently once the (never-immediately-deleted) backlog exceeds the
// batch size — every pass would re-return the same oldest rows and never reach
// newer events. The stability lag (created <= NOW() - lag) keeps the cursor
// correct under concurrent commits (see realtimeOutboxStabilityLag).
func (app *BaseApp) RealtimeOutboxEventsAfter(afterCreated time.Time, afterId string, limit int) ([]RealtimeOutboxEvent, error) {
	if limit <= 0 {
		return nil, nil
	}

	rows := []struct {
		Id         string    `db:"id"`
		Action     string    `db:"action"`
		Collection string    `db:"collection"`
		RecordId   string    `db:"record_id"`
		Snapshot   []byte    `db:"snapshot"`
		Created    time.Time `db:"created"`
	}{}

	err := app.NonconcurrentDB().NewQuery(fmt.Sprintf(
		`SELECT "id", "action", "collection", "record_id", COALESCE("snapshot", '{}'::jsonb) AS "snapshot", "created"
		 FROM "%s"
		 WHERE ("created", "id") > ({:afterCreated}::timestamptz, {:afterId})
		   AND "created" <= NOW() - {:lag}::interval
		   AND "origin" IS DISTINCT FROM {:self}
		 ORDER BY "created", "id" LIMIT {:limit}`,
		RealtimeOutboxTableName,
	)).
		Bind(dbx.Params{
			"afterCreated": afterCreated,
			"afterId":      afterId,
			"lag":          realtimeOutboxStabilityLag.String(),
			"self":         app.realtimeOutboxOrigin,
			"limit":        limit,
		}).
		All(&rows)
	if err != nil {
		return nil, fmt.Errorf("failed to read realtime outbox events: %w", err)
	}

	result := make([]RealtimeOutboxEvent, 0, len(rows))
	for _, r := range rows {
		result = append(result, RealtimeOutboxEvent{
			Id:         r.Id,
			Action:     r.Action,
			Collection: r.Collection,
			RecordId:   r.RecordId,
			Snapshot:   r.Snapshot,
			Created:    r.Created,
		})
	}

	return result, nil
}

// RealtimeOutboxTailCursor returns the (created, id) of the newest outbox row,
// or a zero cursor when the table is empty. A freshly started listener seeds
// its cursor from the tail so it forwards only events published after start
// (rather than replaying up to the whole retention window of history).
func (app *BaseApp) RealtimeOutboxTailCursor() (time.Time, string, error) {
	var row struct {
		Id      string    `db:"id"`
		Created time.Time `db:"created"`
	}

	err := app.NonconcurrentDB().NewQuery(fmt.Sprintf(
		`SELECT "id", "created" FROM "%s" ORDER BY "created" DESC, "id" DESC LIMIT 1`,
		RealtimeOutboxTableName,
	)).One(&row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, "", nil
		}
		return time.Time{}, "", fmt.Errorf("failed to read realtime outbox tail cursor: %w", err)
	}

	return row.Created, row.Id, nil
}

// OpenRealtimeOutboxListener opens a dedicated pgx-native connection for
// LISTENing on the realtime outbox channel. It needs a native connection
// (not database/sql) for WaitForNotification. The connection targets the same
// database the app is connected to (via current_database, since a custom
// DBConnect closure or test harness may vary the database name).
func (app *BaseApp) OpenRealtimeOutboxListener(connectCtx context.Context) (*pgx.Conn, error) {
	if app.dataDB == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	cfg := ResolveDBConfig(DBConfig{})

	var currentDB string
	if err := app.NonconcurrentDB().NewQuery("SELECT current_database()").Row(&currentDB); err != nil {
		return nil, fmt.Errorf("failed to read current database name: %w", err)
	}
	cfg.DBName = currentDB

	return pgx.Connect(connectCtx, buildDSN(cfg))
}

// registerRealtimeOutboxCleanup registers a cleanup cron for the realtime
// outbox. It runs only when the outbox is enabled, so the default single-
// instance setup stays untouched. The cleanup removes processed events and any
// stale pending events (eg. from a dead publisher), keeping the table tiny.
func (app *BaseApp) registerRealtimeOutboxCleanup() {
	app.Cron().Add(realtimeOutboxCleanupCronKey, "37 * * * *", func() {
		if !app.realtimeOutboxEnabled() {
			return
		}
		if err := app.CleanupStaleRealtimeEvents(24 * time.Hour); err != nil {
			app.Logger().Warn("Failed to clean up realtime outbox", "error", err.Error())
		}
	})
}

// CleanupStaleRealtimeEvents removes processed events (and any pending event
// older than staleAge - e.g. a dead publisher) so the outbox table stays tiny.
func (app *BaseApp) CleanupStaleRealtimeEvents(staleAge time.Duration) error {
	// bound the cleanup write with a client-side deadline (see withWriteDeadline)
	execCtx, cancel := withWriteDeadline(context.Background(), app.config.QueryTimeout)
	defer cancel()

	_, err := app.NonconcurrentDB().NewQuery(fmt.Sprintf(
		`DELETE FROM "%s" WHERE "processed_at" IS NOT NULL OR "created" < {:cutoff}::timestamptz`,
		RealtimeOutboxTableName,
	)).
		Bind(dbx.Params{"cutoff": time.Now().Add(-staleAge)}).
		WithContext(execCtx).
		Execute()

	return err
}

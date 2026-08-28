package core

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/pocketbase/dbx"
)

// RealtimeOutboxTableName is the DB-backed event log used by the cross-instance
// realtime broadcaster (D-4, step 1: data layer + publisher). Events are rows;
// NOTIFY is only a wake-up signal, so payload size is not bounded by the 8KB
// NOTIFY limit and delete snapshots can carry the full serialized record.
const RealtimeOutboxTableName = "_realtime_outbox"

// realtimeOutboxNotifyChannel is the PostgreSQL NOTIFY channel used to wake up
// other instances' listeners (payload empty - the data lives in the table).
const realtimeOutboxNotifyChannel = "pb_realtime_outbox"

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
		snapshotJSON, _ = json.Marshal(snapshot.Fresh())
	}

	db := app.NonconcurrentDB()

	insertSQL := fmt.Sprintf(
		`INSERT INTO "%s" ("action", "collection", "record_id", "snapshot") VALUES ({:action}, {:collection}, {:recordId}, {:snapshot})`,
		RealtimeOutboxTableName,
	)
	if _, err := db.NewQuery(insertSQL).
		Bind(dbx.Params{
			"action":     action,
			"collection": collectionName,
			"recordId":   recordId,
			"snapshot":   snapshotJSON,
		}).
		Execute(); err != nil {
		return fmt.Errorf("failed to append realtime outbox event: %w", err)
	}

	// wake up other instances (empty payload; data is in the table)
	_, err := db.NewQuery(fmt.Sprintf("SELECT pg_notify('%s', '')", realtimeOutboxNotifyChannel)).Execute()

	return err
}

// realtimeOutboxCleanupCronKey is the hourly cleanup cron for the outbox.
var realtimeOutboxCleanupCronKey = "__pbRealtimeOutboxCleanup__"

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
	_, err := app.NonconcurrentDB().NewQuery(fmt.Sprintf(
		`DELETE FROM "%s" WHERE "processed_at" IS NOT NULL OR "created" < {:cutoff}::timestamptz`,
		RealtimeOutboxTableName,
	)).
		Bind(dbx.Params{"cutoff": time.Now().Add(-staleAge)}).
		Execute()

	return err
}

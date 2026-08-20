package migrations

import (
	"fmt"

	"github.com/arief-fajri/pgbase/core"
)

// Creates the "_audits" (write trail) and "_audit_reads" (read trail) tables.
//
// Both are native RANGE-partitioned by month on "created" with a mandatory
// DEFAULT partition so inserts never fail before the monthly cron creates a
// named partition. The partition key must be part of the primary key, hence
// PRIMARY KEY (id, created).
func init() {
	core.SystemMigrations.Register(func(txApp core.App) error {
		// safety, idempotent (already enabled by _init/_aux_init)
		if _, err := txApp.DB().NewQuery(
			"CREATE EXTENSION IF NOT EXISTS pgcrypto",
		).Execute(); err != nil {
			return fmt.Errorf("pgcrypto extension error: %w", err)
		}

		if _, err := txApp.DB().NewQuery(`
			CREATE TABLE IF NOT EXISTS "_audits" (
				"id"              TEXT DEFAULT ('r'||lower(encode(gen_random_bytes(7),'hex'))) NOT NULL,
				"collection_name" TEXT NOT NULL,
				"record_id"       TEXT NOT NULL,
				"event"           TEXT NOT NULL,
				"auth_id"         TEXT DEFAULT '' NOT NULL,
				"auth_collection" TEXT DEFAULT '' NOT NULL,
				"source"          TEXT DEFAULT '' NOT NULL,
				"changes"         JSONB DEFAULT '{}'::jsonb NOT NULL,
				"snapshot"        JSONB DEFAULT '{}'::jsonb NOT NULL,
				"user_ip"         TEXT DEFAULT '' NOT NULL,
				"user_agent"      TEXT DEFAULT '' NOT NULL,
				"created"         TIMESTAMPTZ DEFAULT NOW() NOT NULL,
				PRIMARY KEY ("id", "created")
			) PARTITION BY RANGE ("created");

			CREATE TABLE IF NOT EXISTS "_audits_default" PARTITION OF "_audits" DEFAULT;

			CREATE INDEX IF NOT EXISTS idx_audits_coll_rec ON "_audits" ("collection_name", "record_id", "created" DESC);
			CREATE INDEX IF NOT EXISTS idx_audits_created ON "_audits" ("created");
		`).Execute(); err != nil {
			return fmt.Errorf("_audits exec error: %w", err)
		}

		if _, err := txApp.DB().NewQuery(`
			CREATE TABLE IF NOT EXISTS "_audit_reads" (
				"id"              TEXT DEFAULT ('r'||lower(encode(gen_random_bytes(7),'hex'))) NOT NULL,
				"collection_name" TEXT NOT NULL,
				"record_id"       TEXT DEFAULT '' NOT NULL,
				"event"           TEXT NOT NULL,
				"auth_id"         TEXT DEFAULT '' NOT NULL,
				"auth_collection" TEXT DEFAULT '' NOT NULL,
				"source"          TEXT DEFAULT 'request' NOT NULL,
				"filter"          TEXT DEFAULT '' NOT NULL,
				"sort"            TEXT DEFAULT '' NOT NULL,
				"page"            INTEGER DEFAULT 0 NOT NULL,
				"per_page"        INTEGER DEFAULT 0 NOT NULL,
				"total_items"     INTEGER DEFAULT 0 NOT NULL,
				"user_ip"         TEXT DEFAULT '' NOT NULL,
				"user_agent"      TEXT DEFAULT '' NOT NULL,
				"created"         TIMESTAMPTZ DEFAULT NOW() NOT NULL,
				PRIMARY KEY ("id", "created")
			) PARTITION BY RANGE ("created");

			CREATE TABLE IF NOT EXISTS "_audit_reads_default" PARTITION OF "_audit_reads" DEFAULT;

			CREATE INDEX IF NOT EXISTS idx_audit_reads_coll ON "_audit_reads" ("collection_name", "created" DESC);
			CREATE INDEX IF NOT EXISTS idx_audit_reads_created ON "_audit_reads" ("created");
		`).Execute(); err != nil {
			return fmt.Errorf("_audit_reads exec error: %w", err)
		}

		return nil
	}, func(txApp core.App) error {
		if _, err := txApp.DB().NewQuery(`DROP TABLE IF EXISTS "_audits" CASCADE`).Execute(); err != nil {
			return err
		}

		_, err := txApp.DB().NewQuery(`DROP TABLE IF EXISTS "_audit_reads" CASCADE`).Execute()

		return err
	})
}

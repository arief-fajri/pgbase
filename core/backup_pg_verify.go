package core

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/arief-fajri/pgbase/tools/dbutils"
	"github.com/pocketbase/dbx"
)

// pgTocObject is a schema-qualified object name parsed from a pg_dump archive
// TOC (see pgRestoreListTables).
type pgTocObject struct {
	Schema string
	Name   string
}

// pgRestoreToc holds the restore-completeness expectations derived from the
// backup archive itself: which tables (with data) and which indexes it
// contains. Because the TOC is the dump's own content listing, the
// expectations are exact, race-free (no dependence on concurrent writes at
// backup time — unlike a manifest of row counts captured around pg_dump) and
// version-agnostic (an older archive is only required to restore what it
// actually contains; never-created tables are never expected).
type pgRestoreToc struct {
	Tables  []pgTocObject
	Indexes []pgTocObject
}

// pgRestoreListTables reads the archive TOC via `pg_restore --list`. It serves
// two purposes: it validates that the archive is parseable at all (a corrupt
// or truncated dump fails here, BEFORE the destructive pg_restore wipes the
// current database) and it derives the expected object set that
// verifyRestoredDatabase checks afterwards.
//
// env is the connection environment (pgConn.envList) and is mandatory (W-14):
// export and the destructive restore both run with it, and on Debian/Ubuntu
// hosts the PATH binaries are pg_wrapper symlinks whose client-VERSION choice
// depends on that environment (pg_wrapper(1): PGHOST set → the default/newest
// client; no env → a local cluster is selected by port/only-cluster rules). An
// env-less `--list` can therefore read the TOC through a different client
// major than the one that wrote the archive — observed in CI pg-matrix (17)
// as pg_restore 16.15 rejecting a 17.11 archive ("unsupported version (1.16)
// in file header"). Same env in, same client out.
func pgRestoreListTables(ctx context.Context, bin, dumpPath string, env []string) (pgRestoreToc, error) {
	cmd := exec.CommandContext(ctx, bin, "--list", dumpPath)
	cmd.Env = append(os.Environ(), env...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return pgRestoreToc{}, fmt.Errorf(
			"could not read the archive TOC (is the dump intact?): %w: %s",
			err, strings.TrimSpace(stderr.String()),
		)
	}

	return parsePGRestoreToc(stdout.String())
}

// parsePGRestoreToc parses `pg_restore --list` output into the expected
// tables and indexes. Observed entry shape (pg_dump 16/17 custom format):
//
//	<oid>; <catalog-oid> <object-oid> <TYPE> <schema|-> <name> [owner]
//	232;   1259 17656 TABLE      public _collections test
//	3820;  0    17656 TABLE DATA public _collections test
//	3656;  1259 17832 INDEX       public idx_audit_reads_coll test
//
// "TABLE DATA" spans two fields (the type and the literal DATA), which shifts
// the schema/name positions by one. CONSTRAINT/FK CONSTRAINT/SEQUENCE entries
// are deliberately not collected: constraint creation failures fail loudly
// through pg_restore's own error output (classified by
// pgRestoreFatalStderrErrors) and PG-BASE tables carry no sequences (TEXT ids).
func parsePGRestoreToc(out string) (pgRestoreToc, error) {
	var toc pgRestoreToc

	seenTables := map[string]bool{}

	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue // header/comment lines and short entries
		}

		switch fields[3] {
		case "TABLE":
			switch {
			case len(fields) >= 8 && fields[4] == "DATA":
				// "TABLE DATA <schema> <name> <owner>"
				if isTocNamespace(fields[5]) {
					addTocTable(&toc, seenTables, fields[5], fields[6])
				}
			case len(fields) >= 8 && fields[4] == "ATTACH":
				// "TABLE ATTACH <schema> <partition> <owner>" — the post-data
				// step that attaches a partition to its parent; the partition
				// table itself is already collected from its TABLE entry
			default:
				// "TABLE <schema> <name> <owner>"
				if len(fields) >= 7 && isTocNamespace(fields[4]) {
					addTocTable(&toc, seenTables, fields[4], fields[5])
				}
			}
		case "INDEX":
			if len(fields) >= 8 && fields[4] == "ATTACH" {
				// "INDEX ATTACH <schema> <child-index> <owner>" — attaches a
				// partition's child index to the partitioned parent index;
				// the parent index is collected from its regular INDEX entry
				continue
			}
			// "INDEX <schema> <name> <owner>"
			if len(fields) >= 7 && isTocNamespace(fields[4]) {
				toc.Indexes = append(toc.Indexes, pgTocObject{Schema: fields[4], Name: fields[5]})
			}
		}
	}

	if len(toc.Tables) == 0 {
		return toc, fmt.Errorf("the archive TOC contains no tables (not a full pg_dump archive?)")
	}

	return toc, nil
}

// isTocNamespace reports whether the TOC field is a real schema name (pg_restore
// prints "-" for objects without a schema, e.g. extensions).
func isTocNamespace(field string) bool {
	return field != "" && field != "-"
}

func addTocTable(toc *pgRestoreToc, seen map[string]bool, schema, name string) {
	key := schema + "." + name
	if seen[key] {
		return // TABLE and its TABLE DATA entry refer to the same table
	}
	seen[key] = true
	toc.Tables = append(toc.Tables, pgTocObject{Schema: schema, Name: name})
}

// pgRestoreBenignStderrError reports whether a pg_restore stderr ERROR line is
// a known-benign artifact of the routine restore paths. Observed families
// (2026-09-26, pg_dump 18.3 against PostgreSQL 16.15, and the documented
// extension behavior of --clean):
//
//   - `unrecognized configuration parameter "..."` — the SET preamble of a
//     NEWER pg_dump client is not known to an older server (observed:
//     "transaction_timeout"); the restore proceeds with server defaults;
//   - `cannot drop inherited constraint ... of relation ...` — the --clean DROP
//     of a partition child's inherited primary key (observed:
//     "_audits_default_pkey"/"_audit_reads_default_pkey"; the child is dropped
//     wholesale with its table instead);
//   - `cannot drop extension ... because other objects depend on it` and
//     `already exists` — the --clean DROP EXTENSION conflict when live objects
//     still depend on it and the CREATE EXTENSION that follows (documented in
//     the pre-W-08 restore path; kept for pg client versions that emit it).
//
// Any other error line is a real restore failure and must be fatal: the
// restore gate prefers failing loudly over passing a partial restore (W-08). A
// future benign pattern must be added here deliberately, after a real restore
// log proves it — never to silence a new failure.
func pgRestoreBenignStderrError(line string) bool {
	if !strings.Contains(line, " error: ") {
		return false // warnings and informational lines are not errors
	}

	return strings.Contains(line, "unrecognized configuration parameter") ||
		strings.Contains(line, "already exists") ||
		(strings.Contains(line, "cannot drop inherited constraint") &&
			strings.Contains(line, "of relation")) ||
		(strings.Contains(line, "cannot drop extension") &&
			strings.Contains(line, "because other objects depend on it"))
}

// pgRestoreFatalStderrErrors returns the pg_restore stderr lines that represent
// real restore failures (every error line that is not a known-benign conflict).
func pgRestoreFatalStderrErrors(stderr string) []string {
	var fatal []string

	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.Contains(line, " error: ") && !pgRestoreBenignStderrError(line) {
			fatal = append(fatal, line)
		}
	}

	return fatal
}

// verifyRestoredDatabase is the post-restore success gate (W-08): it compares
// the restored database against the expectations derived from the archive TOC
// plus the semantic invariants of a bootable PG-BASE database:
//
//  1. every table the archive contains must exist in the restored catalog and
//     be readable (a table that is present only because pg_restore died
//     before dropping it is caught by the missing-index checks below);
//  2. every index the archive contains must exist — indexes are created in the
//     post-data section, AFTER all table data, so a missing index is the
//     deterministic fingerprint of a restore that did not finish;
//  3. semantic sanity: `_collections` non-empty, the settings row present in
//     `_params`, and at least one superuser with a non-empty password hash
//     (auth preserved).
//
// Row-count comparison against a manifest captured at backup time is
// deliberately NOT used: backups run concurrently with live traffic, so
// manifest counts can legitimately differ from the dump's own snapshot and
// would fail valid restores (false alarms destroy trust in the gate).
func (app *BaseApp) verifyRestoredDatabase(ctx context.Context, toc pgRestoreToc) error {
	db := app.ConcurrentDB()

	for _, t := range toc.Tables {
		var exists int
		if err := db.NewQuery(
			`SELECT count(*) FROM information_schema.tables WHERE table_schema = {:schema} AND table_name = {:name}`,
		).Bind(dbx.Params{"schema": t.Schema, "name": t.Name}).
			WithContext(ctx).
			Row(&exists); err != nil {
			return fmt.Errorf("could not read the catalog for table %q.%q: %w", t.Schema, t.Name, err)
		}
		if exists == 0 {
			return fmt.Errorf("expected table %q.%q is missing after the restore", t.Schema, t.Name)
		}

		// readable: proves the restored object accepts queries, without a
		// full-table scan (LIMIT 1 inside; always returns exactly one row)
		var readable int
		if err := db.NewQuery(fmt.Sprintf(
			`SELECT count(*) FROM (SELECT 1 FROM %s.%s LIMIT 1) probe`,
			dbutils.DefaultDialect.QuoteIdentifier(t.Schema),
			dbutils.DefaultDialect.QuoteIdentifier(t.Name),
		)).
			WithContext(ctx).
			Row(&readable); err != nil {
			return fmt.Errorf("expected table %q.%q is not readable after the restore: %w", t.Schema, t.Name, err)
		}
	}

	for _, ix := range toc.Indexes {
		var exists int
		if err := db.NewQuery(
			`SELECT count(*) FROM pg_indexes WHERE schemaname = {:schema} AND indexname = {:name}`,
		).Bind(dbx.Params{"schema": ix.Schema, "name": ix.Name}).
			WithContext(ctx).
			Row(&exists); err != nil {
			return fmt.Errorf("could not read pg_indexes for %q.%q: %w", ix.Schema, ix.Name, err)
		}
		if exists == 0 {
			return fmt.Errorf(
				"expected index %q.%q is missing after the restore (the post-data section did not finish — partial restore?)",
				ix.Schema, ix.Name,
			)
		}
	}

	var collections int
	if err := db.NewQuery(`SELECT count(*) FROM "_collections"`).
		WithContext(ctx).
		Row(&collections); err != nil {
		return fmt.Errorf("could not read _collections: %w", err)
	}
	if collections == 0 {
		return fmt.Errorf("the restored database contains no collections")
	}

	var settingsRows int
	if err := db.NewQuery(`SELECT count(*) FROM "_params" WHERE id = 'settings'`).
		WithContext(ctx).
		Row(&settingsRows); err != nil {
		return fmt.Errorf("could not read _params: %w", err)
	}
	if settingsRows == 0 {
		return fmt.Errorf("the restored database has no settings row in _params")
	}

	var superusers int
	if err := db.NewQuery(`SELECT count(*) FROM "_superusers" WHERE COALESCE(password, '') <> ''`).
		WithContext(ctx).
		Row(&superusers); err != nil {
		return fmt.Errorf("could not read _superusers: %w", err)
	}
	if superusers == 0 {
		return fmt.Errorf("the restored database has no superuser with a non-empty password (auth not preserved)")
	}

	return nil
}

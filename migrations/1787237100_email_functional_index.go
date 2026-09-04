package migrations

import (
	"fmt"
	"strings"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tools/dbutils"
)

// PERF-I01: convert the case-sensitive auth "email" unique indexes into
// case-insensitive functional partial indexes of the form
//
//	CREATE UNIQUE INDEX "..." ON "..." (LOWER("email")) WHERE "email" <> ''
//
// so the case-insensitive identity/email lookups (LOWER(email) = LOWER($1))
// can be served by an index scan instead of a sequential scan. It also closes
// the case-variant duplicate-email loophole (eg. "a@b.co" vs "A@B.co").
//
// This migration is idempotent: databases initialized after this change already
// receive the functional index from initEmailField during the init migration,
// so those collections are detected as already functional and skipped.
func init() {
	core.SystemMigrations.Register(func(txApp core.App) error {
		return ConvertAuthEmailIndexesToFunctional(txApp)
	}, nil)
}

// ConvertAuthEmailIndexesToFunctional swaps every auth collection's
// single-column case-sensitive unique "email" index to the functional
// LOWER("email") form. Collections that already use the functional form are
// left untouched.
//
// If a collection holds emails that differ only by letter case the case
// insensitive unique index cannot be created, so the conversion aborts with a
// descriptive error instead of failing with an opaque Postgres unique-violation.
func ConvertAuthEmailIndexesToFunctional(txApp core.App) error {
	collections, err := txApp.FindAllCollections()
	if err != nil {
		return err
	}

	for _, collection := range collections {
		if !collection.IsAuth() {
			continue
		}

		// locate the single-column unique email index (the matcher already
		// treats LOWER("email") as the "email" column)
		existing, ok := dbutils.FindSingleColumnUniqueIndex(collection.Indexes, core.FieldNameEmail)
		if !ok {
			continue
		}

		// already functional -> nothing to do
		if strings.Contains(strings.ToLower(existing.Columns[0].Name), "lower(") {
			continue
		}

		// abort loudly on case-variant duplicate emails - creating the
		// case-insensitive unique index over them would fail with a hard
		// Postgres error, so surface a clear, actionable message instead
		if err := ensureNoCaseVariantEmailDuplicates(txApp, collection.Name); err != nil {
			return err
		}

		// swap the metadata to the functional form, preserving the existing
		// index name so the table sync drops the old physical index and
		// recreates it in place (Indexes.String() differs -> change detected).
		//
		// SaveNoValidate is required because the collection validator forbids
		// changing a system field's unique index expression on update
		// (validation_invalid_unique_system_field_index) - a user-facing guard
		// that this controlled system migration intentionally bypasses. The
		// duplicate check above guarantees the new unique index can be built,
		// and onCollectionSaveExecute still runs the physical index sync.
		functionalSQL := fmt.Sprintf(
			`CREATE UNIQUE INDEX "%s" ON "%s" (LOWER("%s")) WHERE "%s" <> ''`,
			existing.IndexName, collection.Name, core.FieldNameEmail, core.FieldNameEmail,
		)
		for i, raw := range collection.Indexes {
			if strings.EqualFold(dbutils.ParseIndex(raw).IndexName, existing.IndexName) {
				collection.Indexes[i] = functionalSQL
				break
			}
		}

		if err := txApp.SaveNoValidate(collection); err != nil {
			return fmt.Errorf("failed to convert email index for collection %q: %w", collection.Name, err)
		}
	}

	return nil
}

func ensureNoCaseVariantEmailDuplicates(txApp core.App, tableName string) error {
	rows := []struct {
		Email string `db:"email"`
		Count int64  `db:"count"`
	}{}

	query := fmt.Sprintf(
		`SELECT LOWER("%s") AS "email", COUNT(*) AS "count" FROM "%s" WHERE "%s" <> '' GROUP BY LOWER("%s") HAVING COUNT(*) > 1 ORDER BY "count" DESC, "email" ASC`,
		core.FieldNameEmail, tableName, core.FieldNameEmail, core.FieldNameEmail,
	)

	if err := txApp.DB().NewQuery(query).All(&rows); err != nil {
		return fmt.Errorf("failed to check for case-variant duplicate emails in %q: %w", tableName, err)
	}

	if len(rows) == 0 {
		return nil
	}

	const maxList = 10
	conflicts := make([]string, 0, maxList+1)
	for i, r := range rows {
		if i >= maxList {
			conflicts = append(conflicts, fmt.Sprintf("...and %d more", len(rows)-maxList))
			break
		}
		conflicts = append(conflicts, fmt.Sprintf("%q (%d records)", r.Email, r.Count))
	}

	return fmt.Errorf(
		"cannot create a case-insensitive unique email index on collection %q because it contains %d email(s) that differ only by letter case: %s; resolve these duplicates (merge or reassign the affected records) and re-run the upgrade",
		tableName, len(rows), strings.Join(conflicts, ", "),
	)
}

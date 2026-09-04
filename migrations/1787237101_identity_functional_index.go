package migrations

import (
	"fmt"
	"strings"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tools/dbutils"
	"github.com/arief-fajri/pgbase/tools/list"
)

// IDX-1: extend the PERF-I01 email functional-index fix to every password-auth
// identity field (eg. username, custom identity fields).
//
// The auth lookups build `LOWER(field) = LOWER($1) AND field <> ''` for every
// identity field. Email already gets a functional partial unique index
// (initEmailField + migration 1787237100), but username and any other identity
// field previously only had a plain case-sensitive unique index, forcing a
// sequential scan on every username-based login. It also left the
// case-variant-duplicate-username loophole (eg. "Bob" vs "bob") open.
//
// This migration is idempotent: collections initialized after this change
// already receive functional identity indexes from initIdentityFieldIndexes
// and are detected as already functional and skipped.
func init() {
	core.SystemMigrations.Register(func(txApp core.App) error {
		return ConvertAuthIdentityIndexesToFunctional(txApp)
	}, nil)
}

// ConvertAuthIdentityIndexesToFunctional swaps every auth collection's
// single-column case-sensitive unique indexes on its password-auth identity
// fields (excluding the system email field which is handled by
// ConvertAuthEmailIndexesToFunctional) to the functional LOWER("field") form.
//
// Collections that already use the functional form are left untouched.
//
// If a collection holds identity values that differ only by letter case the
// case-insensitive unique index cannot be created, so the conversion aborts
// with a descriptive error instead of failing with an opaque Postgres
// unique-violation.
func ConvertAuthIdentityIndexesToFunctional(txApp core.App) error {
	collections, err := txApp.FindAllCollections()
	if err != nil {
		return err
	}

	for _, collection := range collections {
		if !collection.IsAuth() {
			continue
		}

		for _, fieldName := range list.ToUniqueStringSlice(collection.PasswordAuth.IdentityFields) {
			if fieldName == "" || strings.EqualFold(fieldName, core.FieldNameEmail) {
				continue // email is handled by the dedicated email migration
			}

			field := collection.Fields.GetByName(fieldName)
			if field == nil || (field.Type() != core.FieldTypeText && field.Type() != core.FieldTypeEmail) {
				continue // not a LOWER()-compatible identity field
			}

			// locate the single-column unique index on the identity field
			existing, ok := dbutils.FindSingleColumnUniqueIndex(collection.Indexes, fieldName)
			if !ok {
				continue // no unique index (initIdentityFieldIndexes adds it on save)
			}

			// already functional -> nothing to do
			if strings.Contains(strings.ToLower(existing.Columns[0].Name), "lower(") {
				continue
			}

			// abort loudly on case-variant duplicates
			if err := ensureNoCaseVariantIdentityDuplicates(txApp, collection.Name, fieldName); err != nil {
				return err
			}

			// swap the metadata to the functional form, preserving the index name
			// so the table sync drops the old physical index and recreates it in
			// place (definition change is detected by the index-diff).
			functionalSQL := fmt.Sprintf(
				`CREATE UNIQUE INDEX "%s" ON "%s" (LOWER("%s")) WHERE "%s" <> ''`,
				existing.IndexName, collection.Name, fieldName, fieldName,
			)
			for i, raw := range collection.Indexes {
				if strings.EqualFold(dbutils.ParseIndex(raw).IndexName, existing.IndexName) {
					collection.Indexes[i] = functionalSQL
					break
				}
			}

			if err := txApp.SaveNoValidate(collection); err != nil {
				return fmt.Errorf("failed to convert %q identity index for collection %q: %w", fieldName, collection.Name, err)
			}
		}
	}

	return nil
}

// ensureNoCaseVariantIdentityDuplicates guards the conversion of a plain
// case-sensitive unique identity index to the case-insensitive functional form
// by surfacing a clear error when case-variant duplicates exist.
func ensureNoCaseVariantIdentityDuplicates(txApp core.App, tableName string, fieldName string) error {
	rows := []struct {
		Identity string `db:"identity"`
		Count    int64  `db:"count"`
	}{}

	query := fmt.Sprintf(
		`SELECT LOWER("%s") AS "identity", COUNT(*) AS "count" FROM "%s" WHERE "%s" <> '' GROUP BY LOWER("%s") HAVING COUNT(*) > 1 ORDER BY "count" DESC, "identity" ASC`,
		fieldName, tableName, fieldName, fieldName,
	)

	if err := txApp.DB().NewQuery(query).All(&rows); err != nil {
		return fmt.Errorf("failed to check for case-variant duplicate %s in %q: %w", fieldName, tableName, err)
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
		conflicts = append(conflicts, fmt.Sprintf("%q (%d records)", r.Identity, r.Count))
	}

	return fmt.Errorf(
		"cannot create a case-insensitive unique %q index on collection %q because it contains %d value(s) that differ only by letter case: %s; resolve these duplicates (merge or reassign the affected records) and re-run the upgrade",
		fieldName, tableName, len(rows), strings.Join(conflicts, ", "),
	)
}
package core

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/arief-fajri/pgbase/tools/dbutils"
	"github.com/arief-fajri/pgbase/tools/security"
)

// SyncRecordTableSchema compares the two provided collections
// and applies the necessary related record table changes.
//
// If oldCollection is null, then only newCollection is used to create the record table.
//
// This method is automatically invoked as part of a collection create/update/delete operation.
func (app *BaseApp) SyncRecordTableSchema(newCollection *Collection, oldCollection *Collection) error {
	if newCollection.IsView() {
		return nil // nothing to sync since views don't have records table
	}

	txErr := app.RunInTransaction(func(txApp App) error {
		// create
		// -----------------------------------------------------------
		if oldCollection == nil || !app.HasTable(oldCollection.Name) {
			tableName := newCollection.Name

			fields := newCollection.Fields

			cols := make(map[string]string, len(fields))

			// add fields definition
			for _, field := range fields {
				cols[field.GetName()] = field.ColumnType(app)
			}

			// create table
			if _, err := txApp.DB().CreateTable(tableName, cols).Execute(); err != nil {
				return err
			}

			return createCollectionIndexes(txApp, newCollection)
		}

		// update
		// -----------------------------------------------------------
		oldTableName := oldCollection.Name
		newTableName := newCollection.Name
		oldFields := oldCollection.Fields
		newFields := newCollection.Fields

		needTableRename := !strings.EqualFold(oldTableName, newTableName)

		// Select the index-sync strategy (v1 conservative):
		//   - structural field change (add/drop/rename/column type/single-multiple)
		//     -> rebuild all indexes (legacy behavior, safe fallback)
		//   - only the index definitions changed
		//     -> rebuild only the affected indexes (diff)
		//   - otherwise (pure rename / cosmetic field change)
		//     -> no index DDL
		structuralFieldChange := hasStructuralFieldChanges(txApp, oldFields, newFields)
		indexesChanged := !indexDefinitionsEquivalent(oldCollection, newCollection)

		if structuralFieldChange {
			// drop old indexes (if any)
			if err := dropCollectionIndexes(txApp, oldCollection); err != nil {
				return err
			}
		} else if indexesChanged {
			if err := applyIndexDiffDrop(txApp, oldCollection, newCollection); err != nil {
				return err
			}
		}

		// check for renamed table
		if needTableRename {
			_, err := txApp.DB().RenameTable("{{"+oldTableName+"}}", "{{"+newTableName+"}}").Execute()
			if err != nil {
				return err
			}
		}

		// check for deleted columns
		for _, oldField := range oldFields {
			if f := newFields.GetById(oldField.GetId()); f != nil {
				continue // exist
			}

			_, err := txApp.DB().DropColumn(newTableName, oldField.GetName()).Execute()
			if err != nil {
				return fmt.Errorf("failed to drop column %s - %w", oldField.GetName(), err)
			}
		}

		// check for new or renamed columns
		toRename := map[string]string{}
		for _, field := range newFields {
			oldField := oldFields.GetById(field.GetId())
			// Note:
			// We are using a temporary column name when adding or renaming columns
			// to ensure that there are no name collisions in case there is
			// names switch/reuse of existing columns (eg. name, title -> title, name).
			// This way we are always doing 1 more rename operation but it provides better less ambiguous experience.

			if oldField == nil {
				tempName := field.GetName() + security.PseudorandomString(5)
				toRename[tempName] = field.GetName()

				// add
				_, err := txApp.DB().AddColumn(newTableName, tempName, field.ColumnType(txApp)).Execute()
				if err != nil {
					return fmt.Errorf("failed to add column %s - %w", field.GetName(), err)
				}
			} else if oldField.GetName() != field.GetName() {
				tempName := field.GetName() + security.PseudorandomString(5)
				toRename[tempName] = field.GetName()

				// rename
				_, err := txApp.DB().RenameColumn(newTableName, oldField.GetName(), tempName).Execute()
				if err != nil {
					return fmt.Errorf("failed to rename column %s - %w", oldField.GetName(), err)
				}
			}
		}

		// set the actual columns name
		for tempName, actualName := range toRename {
			_, err := txApp.DB().RenameColumn(newTableName, tempName, actualName).Execute()
			if err != nil {
				return err
			}
		}

		if err := normalizeSingleVsMultipleFieldChanges(txApp, newCollection, oldCollection); err != nil {
			return err
		}

		if structuralFieldChange {
			return createCollectionIndexes(txApp, newCollection)
		}

		if indexesChanged {
			return applyIndexDiffCreate(txApp, oldCollection, newCollection)
		}

		return nil
	})
	if txErr != nil {
		return txErr
	}

	// run analyze to update query planner statistics
	if analyzeErr := app.Analyze(); analyzeErr != nil {
		app.Logger().Warn("Failed to run ANALYZE after record table sync", slog.String("error", analyzeErr.Error()))
	}

	return nil
}

func normalizeSingleVsMultipleFieldChanges(app App, newCollection *Collection, oldCollection *Collection) error {
	if newCollection.IsView() || oldCollection == nil {
		return nil // view or not an update
	}

	return app.RunInTransaction(func(txApp App) error {
		type viewRow struct {
			Name string `db:"name"`
			SQL  string `db:"sql"`
		}

		for _, newField := range newCollection.Fields {
			// allow to continue even if there is no old field for the cases
			// when a new field is added and there are already inserted data
			var isOldMultiple bool
			if oldField := oldCollection.Fields.GetById(newField.GetId()); oldField != nil {
				if mv, ok := oldField.(MultiValuer); ok {
					isOldMultiple = mv.IsMultiple()
				}
			}

			var isNewMultiple bool
			if mv, ok := newField.(MultiValuer); ok {
				isNewMultiple = mv.IsMultiple()
			}

			if isOldMultiple == isNewMultiple {
				continue // no change
			}

			// -------------------------------------------------------
			// update the field column definition
			// -------------------------------------------------------

			// temporary drop all views to prevent reference errors during the columns renaming
			// (this is used as an "alternative" to the writable_schema PRAGMA)
			//
			// note: PostgreSQL, unlike SQLite, strictly enforces view->column and
			// view->view dependencies. We capture the view definitions ordered by
			// oid (i.e. creation order, which is a valid dependency order since a
			// view referencing another is always created after it), drop them with
			// CASCADE (to also remove interdependent views such as view2 -> view1
			// regardless of order) and later recreate them in the same order.
			views := []viewRow{}
			err := txApp.DB().NewQuery(`
				SELECT c.relname AS name, pg_get_viewdef(c.oid) AS sql
				FROM pg_class c
				JOIN pg_namespace n ON n.oid = c.relnamespace
				WHERE c.relkind = 'v' AND n.nspname = current_schema()
				ORDER BY c.oid
			`).All(&views)
			if err != nil {
				return err
			}
			for _, view := range views {
				_, err = txApp.DB().NewQuery(fmt.Sprintf(`DROP VIEW IF EXISTS "%s" CASCADE`, view.Name)).Execute()
				if err != nil {
					return err
				}
			}

			originalName := newField.GetName()
			oldTempName := "_" + newField.GetName() + security.PseudorandomString(5)

			// rename temporary the original column to something else to allow inserting a new one in its place
			_, err = txApp.DB().RenameColumn(newCollection.Name, originalName, oldTempName).Execute()
			if err != nil {
				return err
			}

			// reinsert the field column with the new type
			_, err = txApp.DB().AddColumn(newCollection.Name, originalName, newField.ColumnType(txApp)).Execute()
			if err != nil {
				return err
			}

			var copyQuery *dbx.Query

			if !isOldMultiple && isNewMultiple {
				// single -> multiple (convert to array)
			copyQuery = txApp.DB().NewQuery(fmt.Sprintf(
				`UPDATE "%s" set "%s" = (
						CASE
							WHEN COALESCE("%s"::text, '') = ''
							THEN '[]'::jsonb
							ELSE (
								CASE
									WHEN "%s"::text LIKE '[' || '%%'
									THEN "%s"::jsonb
									ELSE jsonb_build_array("%s"::text)
								END
							)
						END
					)`,
				newCollection.Name,
				originalName,
				oldTempName,
				oldTempName,
				oldTempName,
				oldTempName,
			))
			} else {
				// multiple -> single (keep only the last element)
				//
				// note: for file fields the actual file objects are not
				// deleted allowing additional custom handling via migration
			copyQuery = txApp.DB().NewQuery(fmt.Sprintf(
				`UPDATE "%s" set "%s" = (
					CASE
						WHEN COALESCE("%s"::text, '[]') = '[]'
						THEN ''
						ELSE (
							CASE
								WHEN jsonb_typeof("%s"::jsonb) = 'array'
								THEN COALESCE("%s"::jsonb->>(jsonb_array_length("%s"::jsonb)-1), '')
								ELSE "%s"::text
							END
						)
					END
				)`,
				newCollection.Name,
				originalName,
				oldTempName,
				oldTempName,
				oldTempName,
				oldTempName,
				oldTempName,
			))
			}

			// copy the normalized values
			_, err = copyQuery.Execute()
			if err != nil {
				return err
			}

			// drop the original column
			_, err = txApp.DB().DropColumn(newCollection.Name, oldTempName).Execute()
			if err != nil {
				return err
			}

			// restore the views in their captured (ascending oid) order so that
			// interdependent views are recreated after their dependencies
			// (e.g. view1 before view2 -> view1). Ordering is required because a
			// single failed statement would abort the whole PostgreSQL transaction.
			for _, view := range views {
				_, err = txApp.DB().NewQuery(fmt.Sprintf(`CREATE VIEW "%s" AS %s`, view.Name, view.SQL)).Execute()
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
}

func dropCollectionIndexes(app App, collection *Collection) error {
	if collection.IsView() {
		return nil // views don't have indexes
	}

	return app.RunInTransaction(func(txApp App) error {
		for _, raw := range collection.Indexes {
			parsed := dbutils.ParseIndex(raw)

			// note: don't check IsValid because the index table name may not be populated
			// (https://github.com/arief-fajri/pgbase/issues/7689)
			if parsed.IndexName == "" {
				return fmt.Errorf("failed to dop index - missing index name: %s", raw)
			}

			_, err := txApp.DB().NewQuery(fmt.Sprintf("DROP INDEX IF EXISTS \"%s\"", parsed.IndexName)).Execute()
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func createCollectionIndexes(app App, collection *Collection) error {
	if collection.IsView() {
		return nil // views don't have indexes
	}

	return app.RunInTransaction(func(txApp App) error {
		// upsert new indexes
		//
		// note: we are returning validation errors because the indexes cannot be
		//       easily validated in a form, aka. before persisting the related
		//       collection record table changes
		errs := validation.Errors{}
		for i, idx := range collection.Indexes {
			parsed := dbutils.ParseIndex(idx)

			// ensure that the index is always for the current collection
			parsed.TableName = collection.Name

			if !parsed.IsValid() {
				errs[strconv.Itoa(i)] = validation.NewError(
					"validation_invalid_index_expression",
					"Invalid CREATE INDEX expression.",
				)
				continue
			}

			if _, err := txApp.DB().NewQuery(parsed.Build()).Execute(); err != nil {
				errs[strconv.Itoa(i)] = validation.NewError(
					"validation_invalid_index_expression",
					fmt.Sprintf("Failed to create index %s - %v.", parsed.IndexName, err.Error()),
				)
				continue
			}
		}

		if len(errs) > 0 {
			return validation.Errors{"indexes": errs}
		}

		return nil
	})
}

// hasStructuralFieldChanges reports whether the old and new fields differ in a
// way that affects the physical column layout (added/removed field, renamed
// column, changed column type or single/multiple toggle). Changes limited to
// field options that do not alter the column definition (eg. labels,
// validation ranges, autogenerate patterns) are NOT considered structural.
func hasStructuralFieldChanges(app App, oldFields, newFields FieldsList) bool {
	if len(oldFields) != len(newFields) {
		return true
	}

	for _, oldField := range oldFields {
		newField := newFields.GetById(oldField.GetId())
		if newField == nil {
			return true
		}

		if !strings.EqualFold(oldField.GetName(), newField.GetName()) {
			return true
		}

		if oldField.ColumnType(app) != newField.ColumnType(app) {
			return true
		}
	}

	return false
}

// indexDefKey returns a canonical definition of an index that ignores the
// schema/table identifiers but preserves everything that changes the physical
// index shape (unique flag, columns/expressions, collations, sort order and
// the WHERE predicate).
func indexDefKey(parsed dbutils.Index) string {
	var b strings.Builder

	if parsed.Unique {
		b.WriteString("unique|")
	} else {
		b.WriteString("nonunique|")
	}

	for _, col := range parsed.Columns {
		b.WriteString(strings.TrimSpace(col.Name))
		b.WriteString(";")
		b.WriteString(strings.ToUpper(strings.TrimSpace(col.Collate)))
		b.WriteString(";")
		b.WriteString(strings.ToUpper(strings.TrimSpace(col.Sort)))
		b.WriteString("|")
	}

	b.WriteString("where=")
	b.WriteString(strings.TrimSpace(parsed.Where))

	return b.String()
}

// indexDefMap maps each index name to its normalized definition key.
func indexDefMap(indexes []string) map[string]string {
	result := make(map[string]string, len(indexes))

	for _, raw := range indexes {
		parsed := dbutils.ParseIndex(raw)
		if parsed.IndexName == "" {
			continue
		}
		result[parsed.IndexName] = indexDefKey(parsed)
	}

	return result
}

// indexDefinitionsEquivalent reports whether the old and new collections have
// the same index definitions (compared by index name and normalized shape,
// ignoring schema/table identifiers).
func indexDefinitionsEquivalent(oldCollection, newCollection *Collection) bool {
	oldMap := indexDefMap(oldCollection.Indexes)
	newMap := indexDefMap(newCollection.Indexes)

	if len(oldMap) != len(newMap) {
		return false
	}

	for name, oldKey := range oldMap {
		if newMap[name] != oldKey {
			return false
		}
	}

	return true
}

// applyIndexDiffDrop drops the old indexes that have been removed or whose
// definition changed in the new collection. Indexes whose name and definition
// are unchanged are left untouched.
func applyIndexDiffDrop(app App, oldCollection, newCollection *Collection) error {
	newMap := indexDefMap(newCollection.Indexes)

	for _, raw := range oldCollection.Indexes {
		parsed := dbutils.ParseIndex(raw)
		if parsed.IndexName == "" {
			continue
		}

		newKey, exists := newMap[parsed.IndexName]
		if exists && newKey == indexDefKey(parsed) {
			continue // unchanged
		}

		if _, err := app.DB().NewQuery(fmt.Sprintf("DROP INDEX IF EXISTS \"%s\"", parsed.IndexName)).Execute(); err != nil {
			return err
		}
	}

	return nil
}

// applyIndexDiffCreate creates only the new indexes that are missing or whose
// definition changed compared to the old collection. It mirrors the validation
// error handling of createCollectionIndexes.
func applyIndexDiffCreate(app App, oldCollection, newCollection *Collection) error {
	oldMap := indexDefMap(oldCollection.Indexes)

	errs := validation.Errors{}
	for i, raw := range newCollection.Indexes {
		parsed := dbutils.ParseIndex(raw)

		oldKey, exists := oldMap[parsed.IndexName]
		if exists && oldKey == indexDefKey(parsed) {
			continue // unchanged
		}

		parsed.TableName = newCollection.Name

		if !parsed.IsValid() {
			errs[strconv.Itoa(i)] = validation.NewError(
				"validation_invalid_index_expression",
				"Invalid CREATE INDEX expression.",
			)
			continue
		}

		if _, err := app.DB().NewQuery(parsed.Build()).Execute(); err != nil {
			errs[strconv.Itoa(i)] = validation.NewError(
				"validation_invalid_index_expression",
				fmt.Sprintf("Failed to create index %s - %v.", parsed.IndexName, err.Error()),
			)
			continue
		}
	}

	if len(errs) > 0 {
		return errs
	}

	return nil
}

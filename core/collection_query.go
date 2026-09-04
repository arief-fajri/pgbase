package core

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/arief-fajri/pgbase/tools/list"
)

const StoreKeyCachedCollections = "pbAppCachedCollections"

// storeKeyCachedCollectionsLookup holds an id + lower(name) index over the
// cached collections slice so FindCachedCollectionByNameOrId can resolve in
// O(1) instead of scanning the whole slice on every call (hot path).
const storeKeyCachedCollectionsLookup = "pbAppCachedCollectionsLookup"

// CollectionQuery returns a new Collection select query.
func (app *BaseApp) CollectionQuery() *dbx.SelectQuery {
	return app.ModelQuery(&Collection{})
}

// FindCollections finds all collections by the given type(s).
//
// If collectionTypes is not set, it returns all collections.
//
// Example:
//
//	app.FindAllCollections() // all collections
//	app.FindAllCollections("auth", "view") // only auth and view collections
func (app *BaseApp) FindAllCollections(collectionTypes ...string) ([]*Collection, error) {
	collections := []*Collection{}

	q := app.CollectionQuery()

	types := list.NonzeroUniques(collectionTypes)
	if len(types) > 0 {
		q.AndWhere(dbx.In("type", list.ToInterfaceSlice(types)...))
	}

	err := q.OrderBy("id ASC").All(&collections)
	if err != nil {
		return nil, err
	}

	return collections, nil
}

// ReloadCachedCollections fetches all collections and caches them into the app store.
func (app *BaseApp) ReloadCachedCollections() error {
	collections, err := app.FindAllCollections()
	if err != nil {
		return err
	}

	// build an id + lower(name) lookup index alongside the slice so that
	// FindCachedCollectionByNameOrId is O(1). Keys are inserted in the slice
	// order (id ASC) with the id before the name and on a first-wins basis so
	// the resolved collection matches the previous linear scan semantics
	// (c.Id == q || EqualFold(c.Name, q)).
	lookup := make(map[string]*Collection, len(collections)*2)
	for _, c := range collections {
		if _, ok := lookup[c.Id]; !ok {
			lookup[c.Id] = c
		}
		nameKey := strings.ToLower(c.Name)
		if _, ok := lookup[nameKey]; !ok {
			lookup[nameKey] = c
		}
	}

	app.Store().Set(StoreKeyCachedCollections, collections)
	app.Store().Set(storeKeyCachedCollectionsLookup, lookup)

	return nil
}

// FindCollectionByNameOrId finds a single collection by its name (case insensitive) or id.
func (app *BaseApp) FindCollectionByNameOrId(nameOrId string) (*Collection, error) {
	m := &Collection{}

	err := app.CollectionQuery().
		AndWhere(dbx.NewExp("[[id]]={:id} OR LOWER([[name]])={:name}", dbx.Params{
			"id":   nameOrId,
			"name": strings.ToLower(nameOrId),
		})).
		Limit(1).
		One(m)
	if err != nil {
		return nil, err
	}

	return m, nil
}

// FindCachedCollectionByNameOrId is similar to [BaseApp.FindCollectionByNameOrId]
// but retrieves the Collection from the app cache instead of making a db call.
//
// NB! This method is suitable for read-only Collection operations.
//
// Returns [sql.ErrNoRows] if no Collection is found for consistency
// with the [BaseApp.FindCollectionByNameOrId] method.
//
// If you plan making changes to the returned Collection model,
// use [BaseApp.FindCollectionByNameOrId] instead.
//
// Caveats:
//
//   - The returned Collection should be used only for read-only operations.
//     Avoid directly modifying the returned cached Collection as it will affect
//     the global cached value even if you don't persist the changes in the database!
//   - If you are updating a Collection in a transaction and then call this method before commit,
//     it'll return the cached Collection state and not the one from the uncommitted transaction.
//   - The cache is automatically updated on collections db change (create/update/delete).
//     To manually reload the cache you can call [BaseApp.ReloadCachedCollections].
func (app *BaseApp) FindCachedCollectionByNameOrId(nameOrId string) (*Collection, error) {
	lookup, ok := app.Store().Get(storeKeyCachedCollectionsLookup).(map[string]*Collection)
	if !ok {
		// cache is not initialized yet (eg. run in a system migration)
		return app.FindCollectionByNameOrId(nameOrId)
	}

	// exact match resolves case-sensitive ids (and already-lowercased names);
	// the lowered fallback resolves the case-insensitive name lookup
	if c, ok := lookup[nameOrId]; ok {
		return c, nil
	}
	if c, ok := lookup[strings.ToLower(nameOrId)]; ok {
		return c, nil
	}

	return nil, sql.ErrNoRows
}

// FindCollectionReferences returns information for all relation fields
// referencing the provided collection.
//
// If the provided collection has reference to itself then it will be
// also included in the result. To exclude it, pass the collection id
// as the excludeIds argument.
func (app *BaseApp) FindCollectionReferences(collection *Collection, excludeIds ...string) (map[*Collection][]Field, error) {
	collections := []*Collection{}

	query := app.CollectionQuery()

	if uniqueExcludeIds := list.NonzeroUniques(excludeIds); len(uniqueExcludeIds) > 0 {
		query.AndWhere(dbx.NotIn("id", list.ToInterfaceSlice(uniqueExcludeIds)...))
	}

	if err := query.All(&collections); err != nil {
		return nil, err
	}

	result := map[*Collection][]Field{}

	for _, c := range collections {
		for _, rawField := range c.Fields {
			f, ok := rawField.(*RelationField)
			if ok && f.CollectionId == collection.Id {
				result[c] = append(result[c], f)
			}
		}
	}

	return result, nil
}

// FindCachedCollectionReferences is similar to [BaseApp.FindCollectionReferences]
// but retrieves the Collection from the app cache instead of making a db call.
//
// NB! This method is suitable for read-only Collection operations.
//
// If you plan making changes to the returned Collection model,
// use [BaseApp.FindCollectionReferences] instead.
//
// Caveats:
//
//   - The returned Collection should be used only for read-only operations.
//     Avoid directly modifying the returned cached Collection as it will affect
//     the global cached value even if you don't persist the changes in the database!
//   - If you are updating a Collection in a transaction and then call this method before commit,
//     it'll return the cached Collection state and not the one from the uncommitted transaction.
//   - The cache is automatically updated on collections db change (create/update/delete).
//     To manually reload the cache you can call [BaseApp.ReloadCachedCollections].
func (app *BaseApp) FindCachedCollectionReferences(collection *Collection, excludeIds ...string) (map[*Collection][]Field, error) {
	collections, _ := app.Store().Get(StoreKeyCachedCollections).([]*Collection)
	if collections == nil {
		// cache is not initialized yet (eg. run in a system migration)
		return app.FindCollectionReferences(collection, excludeIds...)
	}

	result := map[*Collection][]Field{}

	for _, c := range collections {
		if slices.Contains(excludeIds, c.Id) {
			continue
		}

		for _, rawField := range c.Fields {
			f, ok := rawField.(*RelationField)
			if ok && f.CollectionId == collection.Id {
				result[c] = append(result[c], f)
			}
		}
	}

	return result, nil
}

// IsCollectionNameUnique checks that there is no existing collection
// with the provided name (case insensitive!).
//
// Note: case insensitive check because the name is used also as
// table name for the records.
func (app *BaseApp) IsCollectionNameUnique(name string, excludeIds ...string) bool {
	if name == "" {
		return false
	}

	query := app.CollectionQuery().
		Select("count(*)").
		AndWhere(dbx.NewExp("LOWER([[name]])={:name}", dbx.Params{"name": strings.ToLower(name)})).
		Limit(1)

	if uniqueExcludeIds := list.NonzeroUniques(excludeIds); len(uniqueExcludeIds) > 0 {
		query.AndWhere(dbx.NotIn("id", list.ToInterfaceSlice(uniqueExcludeIds)...))
	}

	var total int

	return query.Row(&total) == nil && total == 0
}

// TruncateCollection deletes all records associated with the provided collection.
//
// The truncate operation is executed in a single transaction,
// aka. either everything is deleted or none.
//
// Note that this method will also trigger the records related
// cascade and file delete actions.
func (app *BaseApp) TruncateCollection(collection *Collection) error {
	if collection.IsView() {
		return errors.New("view collections cannot be truncated since they don't store their own records")
	}

	return app.RunInTransaction(func(txApp App) error {
		records := make([]*Record, 0, 500)

		for {
			err := txApp.RecordQuery(collection).Limit(500).All(&records)
			if err != nil {
				return err
			}

			if len(records) == 0 {
				return nil
			}

			for _, record := range records {
				err = txApp.Delete(record)
				if err != nil && !errors.Is(err, sql.ErrNoRows) {
					return err
				}
			}

			records = records[:0]
		}
	})
}

// -------------------------------------------------------------------

// normalizeViewQueryId wraps (if necessary) the provided view query
// with a subselect to ensure that the id column is a text since
// currently we don't support non-string model ids
// (see https://github.com/arief-fajri/pgbase/issues/3110).
func normalizeViewQueryId(app App, query string) (string, error) {
	query = strings.Trim(strings.TrimSpace(query), ";")

	info, err := getQueryTableInfo(app, query)
	if err != nil {
		return "", err
	}

	for _, row := range info {
		if strings.EqualFold(row.Name, FieldNameId) && strings.EqualFold(row.Type, "TEXT") {
			return query, nil // no wrapping needed
		}
	}

	// raw parse to preserve the columns order
	rawParsed := new(identifiersParser)
	if err := rawParsed.parse(query); err != nil {
		return "", err
	}

	columns := make([]string, 0, len(rawParsed.columns))
	for _, col := range rawParsed.columns {
		if col.alias == FieldNameId {
			columns = append(columns, fmt.Sprintf("CAST([[%s]] as TEXT) [[%s]]", col.alias, col.alias))
		} else {
			columns = append(columns, "[["+col.alias+"]]")
		}
	}

	query = fmt.Sprintf("SELECT %s FROM (%s)", strings.Join(columns, ","), query)

	return query, nil
}

// resaveViewsWithChangedFields updates all view collections with changed fields.
func resaveViewsWithChangedFields(app App, excludeIds ...string) error {
	collections, err := app.FindAllCollections(CollectionTypeView)
	if err != nil {
		return err
	}

	return app.RunInTransaction(func(txApp App) error {
		var collectionErrors []error

		for _, collection := range collections {
			if len(excludeIds) > 0 && list.ExistInSlice(collection.Id, excludeIds) {
				continue
			}

			check := func() error {
				// clone the existing fields for temp modifications
				oldFields, err := collection.Fields.Clone()
				if err != nil {
					return err
				}

				// generate new fields from the query
				newFields, err := txApp.CreateViewFields(collection.ViewQuery)
				if err != nil {
					return err
				}

				// unset the fields' ids to exclude from the comparison
				for _, f := range oldFields {
					f.SetId("")
				}
				for _, f := range newFields {
					f.SetId("")
				}

				encodedNewFields, err := json.Marshal(newFields)
				if err != nil {
					return err
				}

				encodedOldFields, err := json.Marshal(oldFields)
				if err != nil {
					return err
				}

				if bytes.EqualFold(encodedNewFields, encodedOldFields) {
					return nil // no changes
				}

				return txApp.Save(collection)
			}

			if err := check(); err != nil {
				collectionErrors = append(
					collectionErrors,
					fmt.Errorf("[%s] %w", collection.Name, err),
				)
			}
		}

		return errors.Join(collectionErrors...)
	})
}

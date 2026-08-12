package core

import (
	"database/sql"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/arief-fajri/pgbase/tools/dbutils"
)

func (app *BaseApp) TableColumns(tableName string) ([]string, error) {
	columns := []string{}

	err := app.DB().NewQuery(dbutils.DefaultDialect.TableColumnsQuery()).
		Bind(dbx.Params{"tableName": tableName}).
		Column(&columns)

	return columns, err
}

type TableInfoRow struct {
	PK           int            `db:"pk"`
	Index        int            `db:"cid"`
	Name         string         `db:"name"`
	Type         string         `db:"type"`
	NotNull      bool           `db:"notnull"`
	DefaultValue sql.NullString `db:"dflt_value"`
}

func (app *BaseApp) TableInfo(tableName string) ([]*TableInfoRow, error) {
	info := []*TableInfoRow{}

	err := app.DB().NewQuery(dbutils.DefaultDialect.TableInfoQuery()).
		Bind(dbx.Params{"tableName": tableName}).
		All(&info)
	if err != nil {
		return nil, err
	}

	if len(info) == 0 {
		return nil, fmt.Errorf("empty table info probably due to invalid or missing table %s", tableName)
	}

	return info, nil
}

func (app *BaseApp) TableIndexes(tableName string) (map[string]string, error) {
	indexes := []struct {
		Name string
		Sql  string
	}{}

	err := app.DB().NewQuery(dbutils.DefaultDialect.TableIndexesQuery()).
		Bind(dbx.Params{"tableName": tableName}).
		All(&indexes)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string, len(indexes))

	for _, idx := range indexes {
		result[idx.Name] = idx.Sql
	}

	return result, nil
}

func (app *BaseApp) DeleteTable(dangerousTableName string) error {
	_, err := app.DB().NewQuery(fmt.Sprintf(
		"DROP TABLE IF EXISTS %s",
		dbutils.DefaultDialect.QuoteIdentifier(dangerousTableName),
	)).Execute()

	return err
}

func (app *BaseApp) HasTable(tableName string) bool {
	return app.hasTable(app.DB(), tableName)
}

func (app *BaseApp) AuxHasTable(tableName string) bool {
	return app.hasTable(app.AuxDB(), tableName)
}

func (app *BaseApp) hasTable(db dbx.Builder, tableName string) bool {
	var exists int

	err := db.NewQuery(dbutils.DefaultDialect.HasTableQuery()).
		Bind(dbx.Params{"tableName": tableName}).
		Row(&exists)

	return err == nil && exists > 0
}

func (app *BaseApp) Vacuum() error {
	_, err := app.DB().NewQuery(dbutils.DefaultDialect.OptimizeQuery()).Execute()
	return err
}

func (app *BaseApp) AuxVacuum() error {
	_, err := app.AuxDB().NewQuery(dbutils.DefaultDialect.OptimizeQuery()).Execute()
	return err
}

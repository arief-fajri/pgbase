package dbutils

var DefaultDialect Dialect = &PgSQLDialect{}

func SetDialect(d Dialect) {
	DefaultDialect = d
}

func JSONEach(column string) string {
	return DefaultDialect.JSONEach(column)
}

func JSONArrayLength(column string) string {
	return DefaultDialect.JSONArrayLength(column)
}

func JSONExtract(column string, path string) string {
	return DefaultDialect.JSONExtract(column, path)
}

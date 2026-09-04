package dbutils

type Dialect interface {
	JSONEach(column string) string
	JSONArrayLength(column string) string
	JSONExtract(column, path string) string

	TableColumnsQuery() string
	TableInfoQuery() string
	TableIndexesQuery() string
	HasTableQuery() string

	GenerateIDExpression() string

	CollateNocase(column string) string

	QuoteIdentifier(name string) string

	RandomExpression() string

	OptimizeQuery() string

	VacuumQuery() string
}

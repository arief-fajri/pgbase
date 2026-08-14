package dbutils

import (
	"fmt"
	"strings"
)

type PgSQLDialect struct{}

func (d *PgSQLDialect) JSONEach(column string) string {
	return fmt.Sprintf(
		`jsonb_array_elements(
			CASE WHEN jsonb_typeof([[%s]]) = 'array'
			THEN [[%s]]::jsonb
			ELSE jsonb_build_array([[%s]]::text)
			END
		)`,
		column, column, column,
	)
}

func (d *PgSQLDialect) JSONArrayLength(column string) string {
	return fmt.Sprintf(
		`jsonb_array_length(
			CASE WHEN jsonb_typeof([[%s]]) = 'array'
			THEN [[%s]]::jsonb
			ELSE CASE WHEN [[%s]] = '' OR [[%s]] IS NULL
				 THEN '[]'::jsonb
				 ELSE jsonb_build_array([[%s]])
			END
			END
		)`,
		column, column, column, column, column,
	)
}

func (d *PgSQLDialect) JSONExtract(column, path string) string {
	if path != "" && !strings.HasPrefix(path, "[") {
		path = "." + path
	}

	return fmt.Sprintf(
		`(CASE WHEN [[%s]] IS NOT NULL AND jsonb_typeof([[%s]]::jsonb) IS NOT NULL
		 THEN [[%s]]::jsonb #>> '%s'
		 ELSE (jsonb_build_object('pb', [[%s]]::text)) #>> '%s'
		 END)`,
		column, column, column, path, column, ".pb"+path,
	)
}

func (d *PgSQLDialect) TableColumnsQuery() string {
	return `SELECT column_name
            FROM information_schema.columns
            WHERE table_name = {:tableName} AND table_schema = current_schema()
            ORDER BY ordinal_position`
}

func (d *PgSQLDialect) TableInfoQuery() string {
	return `SELECT
               c.ordinal_position - 1 AS cid,
               c.column_name AS name,
               c.data_type AS type,
               (c.is_nullable = 'NO')::boolean AS notnull,
               c.column_default AS dflt_value,
               CASE WHEN pk.contype IS NOT NULL THEN 1 ELSE 0 END AS pk
            FROM information_schema.columns c
            LEFT JOIN pg_constraint pk
                ON pk.conrelid = (quote_ident(c.table_schema) || '.' || quote_ident(c.table_name))::regclass
                AND pk.contype = 'p'
                AND pk.conkey @> ARRAY[c.ordinal_position::int]::smallint[]
            WHERE c.table_name = {:tableName} AND c.table_schema = current_schema()
            ORDER BY c.ordinal_position`
}

func (d *PgSQLDialect) TableIndexesQuery() string {
	return `SELECT indexname AS name, indexdef AS sql
            FROM pg_indexes
            WHERE tablename = {:tableName} AND schemaname = current_schema()`
}

func (d *PgSQLDialect) HasTableQuery() string {
	return `SELECT 1
            FROM information_schema.tables
            WHERE LOWER(table_name) = LOWER({:tableName})
            AND table_schema = current_schema()
            LIMIT 1`
}

func (d *PgSQLDialect) GenerateIDExpression() string {
	return "DEFAULT ('r' || lower(encode(gen_random_bytes(7), 'hex')))"
}

func (d *PgSQLDialect) Strftime(column, format string) string {
	pgFormat := convertStrftimeFormat(format)
	return fmt.Sprintf("to_char(%s, '%s')", column, pgFormat)
}

func (d *PgSQLDialect) CollateNocase(column string) string {
	return fmt.Sprintf("LOWER(%s)", column)
}

func (d *PgSQLDialect) QuoteIdentifier(name string) string {
	return fmt.Sprintf(`"%s"`, name)
}

func (d *PgSQLDialect) RandomExpression() string {
	return "RANDOM()"
}

func (d *PgSQLDialect) OptimizeQuery() string {
	return "ANALYZE"
}

func (d *PgSQLDialect) VacuumQuery() string {
	return "VACUUM"
}

func convertStrftimeFormat(format string) string {
	replacer := strings.NewReplacer(
		"%Y", "YYYY",
		"%m", "MM",
		"%d", "DD",
		"%H", "HH24",
		"%M", "MI",
		"%S", "SS",
		"%f", "MS",
		"%w", "D",
		"%j", "DDD",
	)
	return replacer.Replace(format)
}

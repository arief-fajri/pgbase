package dbutils

import (
	"fmt"
	"regexp"
	"strings"
)

var jsonIndexRegexp = regexp.MustCompile(`\[(\d+)\]`)

type PgSQLDialect struct{}

func (d *PgSQLDialect) JSONEach(column string) string {
	// Works for both jsonb array columns (multi relation/select/file) and
	// plain text scalar columns (single relation, stored as a bare id):
	//   - a JSON array text/jsonb  -> unnested elements
	//   - a non-empty scalar        -> a single-element set
	//   - NULL or empty string      -> an empty set
	// The leading "[" heuristic avoids casting non-JSON text (e.g. a bare id
	// like "abc") to jsonb, which would raise SQLSTATE 22P02.
	return fmt.Sprintf(
		`jsonb_array_elements_text(
			CASE
				WHEN [[%s]] IS NULL OR [[%s]]::text = '' THEN '[]'::jsonb
				WHEN left(ltrim([[%s]]::text), 1) = '[' THEN ([[%s]]::text)::jsonb
				ELSE jsonb_build_array([[%s]]::text)
			END
		)`,
		column, column, column, column, column,
	)
}

func (d *PgSQLDialect) JSONArrayLength(column string) string {
	// Mirrors [PgSQLDialect.JSONEach] typing rules: JSON array -> element
	// count, non-empty scalar -> 1, NULL/empty -> 0. Comparing the column as
	// ::text avoids folding the untyped '' literal to jsonb (SQLSTATE 22P02).
	return fmt.Sprintf(
		`(CASE
			WHEN [[%s]] IS NULL OR [[%s]]::text = '' THEN 0
			WHEN left(ltrim([[%s]]::text), 1) = '[' THEN jsonb_array_length(([[%s]]::text)::jsonb)
			ELSE 1
		END)`,
		column, column, column, column,
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
		column, column, column, pgJSONPath(path), column, pgJSONPath(".pb"+path),
	)
}

// pgJSONPath converts a SQLite-style dotted json path (eg. ".a.b[0]" or
// "[1].a[2]") into the text[]-array lookup notation expected by PostgreSQL's
// #>/#>> operators (eg. "{a,b,0}").
//
// A leading "$" root marker (SQLite json path syntax, eg. "$.a.b") is ignored
// since PostgreSQL's path arrays are relative to the document root and would
// otherwise treat "$" as a literal key.
func pgJSONPath(path string) string {
	parts := []string{}
	for _, seg := range strings.Split(path, ".") {
		// extract the key segment (text before the first "[")
		key := seg
		indices := ""
		if i := strings.IndexByte(seg, '['); i >= 0 {
			key = seg[:i]
			indices = seg[i:]
		}
		if key != "" && key != "$" {
			parts = append(parts, key)
		}
		// extract each "[n]" array index
		for _, m := range jsonIndexRegexp.FindAllStringSubmatch(indices, -1) {
			parts = append(parts, m[1])
		}
	}
	return "{" + strings.Join(parts, ",") + "}"
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
	// Exclude the auto-created primary key index (<table>_pkey) so the
	// result mirrors SQLite, whose sqlite_master query filters out
	// auto-indexes (they have a NULL "sql"). Only explicitly created
	// indexes (CREATE [UNIQUE] INDEX ...) are returned.
	return `SELECT i.relname AS name, pg_get_indexdef(x.indexrelid) AS sql
            FROM pg_index x
            JOIN pg_class i ON i.oid = x.indexrelid
            JOIN pg_class t ON t.oid = x.indrelid
            JOIN pg_namespace n ON n.oid = t.relnamespace
            WHERE t.relname = {:tableName}
                AND n.nspname = current_schema()
                AND NOT x.indisprimary`
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

func (d *PgSQLDialect) CollateNocase(column string) string {
	return fmt.Sprintf("LOWER(%s)", column)
}

func (d *PgSQLDialect) QuoteIdentifier(name string) string {
	// escape embedded double-quotes (PGB-L01); already-quoted names are
	// normalized rather than double-wrapped (mirrors dbx quoteSimpleIdentifier)
	if len(name) >= 2 && strings.HasPrefix(name, `"`) && strings.HasSuffix(name, `"`) {
		return `"` + strings.ReplaceAll(name[1:len(name)-1], `"`, `""`) + `"`
	}
	return fmt.Sprintf(`"%s"`, strings.ReplaceAll(name, `"`, `""`))
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

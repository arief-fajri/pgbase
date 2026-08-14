package search_test

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/arief-fajri/pgbase/tools/search"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func pgTestDB(t *testing.T) *dbx.DB {
	t.Helper()

	sqlDB, err := sql.Open("pgx", "host=localhost port=5433 user=test password=test dbname=pgbase_test sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })

	return dbx.NewFromDB(sqlDB, "pgx")
}

func TestFilterDataBuildExpr(t *testing.T) {
	resolver := search.NewSimpleFieldResolver("test1", "test2", "test3", `^test4_\w+$`, `^test5\.[\w\.\:]*\w+$`)

	scenarios := []struct {
		name          string
		filterData    search.FilterData
		expectError   bool
		expectPattern string
	}{
		{
			"empty",
			"",
			true,
			"",
		},
		{
			"invalid format",
			"(test1 > 1",
			true,
			"",
		},
		{
			"invalid operator",
			"test1 + 123",
			true,
			"",
		},
		{
			"unknown field",
			"test1 = 'example' && unknown > 1",
			true,
			"",
		},
		{
			"simple expression",
			"test1 > 1",
			false,
			"[[test1]] > {:TEST}",
		},
		{
			"empty string vs null",
			"'' = null && null != ''",
			false,
			"('' = '' AND '' != '')",
		},
		{
			"like with 2 columns",
			"test1 ~ test2",
			false,
			"LOWER([[test1]]) LIKE LOWER(('%' || [[test2]] || '%')) ESCAPE '\\'",
		},
		{
			"like with right column operand",
			"'lorem' ~ test1",
			false,
			"LOWER({:TEST}) LIKE LOWER(('%' || [[test1]] || '%')) ESCAPE '\\'",
		},
		{
			"like with left column operand and text as right operand",
			"test1 ~ 'lorem'",
			false,
			"LOWER([[test1]]) LIKE LOWER({:TEST}) ESCAPE '\\'",
		},
		{
			"not like with 2 columns",
			"test1 !~ test2",
			false,
			"LOWER([[test1]]) NOT LIKE LOWER(('%' || [[test2]] || '%')) ESCAPE '\\'",
		},
		{
			"not like with right column operand",
			"'lorem' !~ test1",
			false,
			"LOWER({:TEST}) NOT LIKE LOWER(('%' || [[test1]] || '%')) ESCAPE '\\'",
		},
		{
			"like with left column operand and text as right operand",
			"test1 !~ 'lorem'",
			false,
			"LOWER([[test1]]) NOT LIKE LOWER({:TEST}) ESCAPE '\\'",
		},
		{
			"nested json no coalesce",
			"test5.a = test5.b || test5.c != test5.d",
			false,
			"((CASE WHEN [[test5]] IS NOT NULL AND jsonb_typeof([[test5]]::jsonb) IS NOT NULL\n\t\t THEN [[test5]]::jsonb #>> '.$.a'\n\t\t ELSE (jsonb_build_object('pb', [[test5]]::text)) #>> '.pb.$.a'\n\t\t END) IS (CASE WHEN [[test5]] IS NOT NULL AND jsonb_typeof([[test5]]::jsonb) IS NOT NULL\n\t\t THEN [[test5]]::jsonb #>> '.$.b'\n\t\t ELSE (jsonb_build_object('pb', [[test5]]::text)) #>> '.pb.$.b'\n\t\t END) OR (CASE WHEN [[test5]] IS NOT NULL AND jsonb_typeof([[test5]]::jsonb) IS NOT NULL\n\t\t THEN [[test5]]::jsonb #>> '.$.c'\n\t\t ELSE (jsonb_build_object('pb', [[test5]]::text)) #>> '.pb.$.c'\n\t\t END) IS DISTINCT FROM (CASE WHEN [[test5]] IS NOT NULL AND jsonb_typeof([[test5]]::jsonb) IS NOT NULL\n\t\t THEN [[test5]]::jsonb #>> '.$.d'\n\t\t ELSE (jsonb_build_object('pb', [[test5]]::text)) #>> '.pb.$.d'\n\t\t END))",
		},
		{
			"macros",
			`
				test4_1 > @now &&
				test4_2 > @second &&
				test4_3 > @minute &&
				test4_4 > @hour &&
				test4_5 > @day &&
				test4_6 > @year &&
				test4_7 > @month &&
				test4_9 > @weekday &&
				test4_9 > @todayStart &&
				test4_10 > @todayEnd &&
				test4_11 > @monthStart &&
				test4_12 > @monthEnd &&
				test4_13 > @yearStart &&
				test4_14 > @yearEnd
			`,
			false,
			"([[test4_1]] > {:TEST} AND [[test4_2]] > {:TEST} AND [[test4_3]] > {:TEST} AND [[test4_4]] > {:TEST} AND [[test4_5]] > {:TEST} AND [[test4_6]] > {:TEST} AND [[test4_7]] > {:TEST} AND [[test4_9]] > {:TEST} AND [[test4_9]] > {:TEST} AND [[test4_10]] > {:TEST} AND [[test4_11]] > {:TEST} AND [[test4_12]] > {:TEST} AND [[test4_13]] > {:TEST} AND [[test4_14]] > {:TEST})",
		},
		{
			"complex expression",
			"((test1 > 1) || (test2 != 2)) && test3 ~ '%%example' && test4_sub = null",
			false,
			"(([[test1]] > {:TEST} OR [[test2]] != {:TEST}) AND LOWER([[test3]]) LIKE LOWER({:TEST}) ESCAPE '\\' AND ([[test4_sub]] = '' OR [[test4_sub]] IS NULL))",
		},
		{
			"combination of special literals (null, true, false)",
			"test1=true && test2 != false && null = test3 || null != test4_sub",
			false,
			"([[test1]] = 1 AND [[test2]] != 0 AND ('' = [[test3]] OR [[test3]] IS NULL) OR ('' != [[test4_sub]] AND [[test4_sub]] IS NOT NULL))",
		},
		{
			"all operators",
			"(test1 = test2 || test2 != test3) && (test2 ~ 'example' || test2 !~ '%%abc') && 'switch1%%' ~ test1 && 'switch2' !~ test2 && test3 > 1 && test3 >= 0 && test3 <= 4 && 2 < 5",
			false,
			"((COALESCE([[test1]], '') = COALESCE([[test2]], '') OR COALESCE([[test2]], '') != COALESCE([[test3]], '')) AND (LOWER([[test2]]) LIKE LOWER({:TEST}) ESCAPE '\\' OR LOWER([[test2]]) NOT LIKE LOWER({:TEST}) ESCAPE '\\') AND LOWER({:TEST}) LIKE LOWER(('%' || [[test1]] || '%')) ESCAPE '\\' AND LOWER({:TEST}) NOT LIKE LOWER(('%' || [[test2]] || '%')) ESCAPE '\\' AND [[test3]] > {:TEST} AND [[test3]] >= {:TEST} AND [[test3]] <= {:TEST} AND 1 = 1)",
		},
		{
			"geoDistance function",
			"geoDistance(1,2,3,4) < 567",
			false,
			"(6371 * acos(cos(radians({:TEST})) * cos(radians({:TEST})) * cos(radians({:TEST}) - radians({:TEST})) + sin(radians({:TEST})) * sin(radians({:TEST})))) < {:TEST}",
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			expr, err := s.filterData.BuildExpr(resolver)

			hasErr := err != nil
			if hasErr != s.expectError {
				t.Fatalf("[%s] Expected hasErr %v, got %v (%v)", s.name, s.expectError, hasErr, err)
			}

			if hasErr {
				return
			}

			dummyDB := &dbx.DB{}

			rawSql := expr.Build(dummyDB, dbx.Params{})

			// replace TEST placeholder with .+ regex pattern
			expectPattern := strings.ReplaceAll(
				"^"+regexp.QuoteMeta(s.expectPattern)+"$",
				"TEST",
				`\w+`,
			)

			pattern := regexp.MustCompile(expectPattern)
			if !pattern.MatchString(rawSql) {
				t.Fatalf("[%s] Pattern %v don't match with expression: \n%v", s.name, expectPattern, rawSql)
			}
		})
	}
}

func TestFilterDataBuildExprWithParams(t *testing.T) {
	db := pgTestDB(t)

	calledQueries := []string{}
	db.QueryLogFunc = func(ctx context.Context, t time.Duration, sql string, rows *sql.Rows, err error) {
		calledQueries = append(calledQueries, sql)
	}
	db.ExecLogFunc = func(ctx context.Context, t time.Duration, sql string, result sql.Result, err error) {
		calledQueries = append(calledQueries, sql)
	}

	date, err := time.Parse("2006-01-02", "2023-01-01")
	if err != nil {
		t.Fatal(err)
	}

	resolver := search.NewSimpleFieldResolver(`^test\w+$`)

	filter := search.FilterData(`
		test1 = {:test1} ||
		test2 = {:test2} ||
		test3a = {:test3} ||
		test3b = {:test3} ||
		test4 = {:test4} ||
		test5 = {:test5} ||
		test6 = {:test6} ||
		test7 = {:test7} ||
		test8 = {:test8} ||
		test9 = {:test9} ||
		test10 = {:test10} ||
		test11 = {:test11} ||
		test12 = {:test12} ||
		test13 = {:test13}
	`)

	replacements := []dbx.Params{
		{"test1": true},
		{"test2": false},
		{"test3": 123.456},
		{"test4": nil},
		{"test5": "", "test6": "simple", "test7": `'single_quotes'`, "test8": `"double_quotes"`, "test9": "'\"quote_with_backslash\\"},
		{"test10": date},
		{"test11": []string{"a", "'quote", `"quote`}},
		{"test12": map[string]any{"a": 123, "b": `quote"`}},
		{"test13": "a\nb"},
	}

	expr, err := filter.BuildExpr(resolver, replacements...)
	if err != nil {
		t.Fatal(err)
	}

	db.Select().Where(expr).Build().Execute()

	if len(calledQueries) != 1 {
		t.Fatalf("Expected 1 query, got %d", len(calledQueries))
	}

	expectedQuery := `SELECT * WHERE ([[test1]] = 1 OR [[test2]] = 0 OR [[test3a]] = 123.456 OR [[test3b]] = 123.456 OR ([[test4]] = '' OR [[test4]] IS NULL) OR [[test5]] = '""' OR [[test6]] = 'simple' OR [[test7]] = '''single_quotes''' OR [[test8]] = '"double_quotes"' OR [[test9]] = '''"quote_with_backslash\' OR [[test10]] = '2023-01-01 00:00:00 +0000 UTC' OR [[test11]] = '["a","''quote","\"quote"]' OR [[test12]] = '{"a":123,"b":"quote\""}' OR [[test13]] = 'a`
	expectedQuery += "\nb')"
	if expectedQuery != calledQueries[0] {
		t.Fatalf("Expected query \n%s, \ngot \n%s", expectedQuery, calledQueries[0])
	}
}

func TestFilterDataBuildExprWithLimit(t *testing.T) {
	resolver := search.NewSimpleFieldResolver(`^\w+$`)

	scenarios := []struct {
		limit       int
		filter      search.FilterData
		expectError bool
	}{
		{1, "1 = 1", false},
		{0, "1 = 1", true}, // new cache entry should be created
		{2, "1 = 1 || 1 = 1", false},
		{1, "1 = 1 || 1 = 1", true},
		{3, "1 = 1 || 1 = 1", false},
		{6, "(1=1 || 1=1) && (1=1 || (1=1 || 1=1)) && (1=1)", false},
		{5, "(1=1 || 1=1) && (1=1 || (1=1 || 1=1)) && (1=1)", true},
	}

	for i, s := range scenarios {
		t.Run(fmt.Sprintf("limit_%d:%d", i, s.limit), func(t *testing.T) {
			_, err := s.filter.BuildExprWithLimit(resolver, s.limit)

			hasErr := err != nil
			if hasErr != s.expectError {
				t.Fatalf("Expected hasErr %v, got %v", s.expectError, hasErr)
			}
		})
	}
}

func TestLikeParamsWrapping(t *testing.T) {
	db := pgTestDB(t)

	calledQueries := []string{}
	db.QueryLogFunc = func(ctx context.Context, t time.Duration, sql string, rows *sql.Rows, err error) {
		calledQueries = append(calledQueries, sql)
	}
	db.ExecLogFunc = func(ctx context.Context, t time.Duration, sql string, result sql.Result, err error) {
		calledQueries = append(calledQueries, sql)
	}

	resolver := search.NewSimpleFieldResolver(`^test\w+$`)

	filter := search.FilterData(`
		test1 ~ {:p1} ||
		test2 ~ {:p2} ||
		test3 ~ {:p3} ||
		test4 ~ {:p4} ||
		test5 ~ {:p5} ||
		test6 ~ {:p6} ||
		test7 ~ {:p7} ||
		test8 ~ {:p8} ||
		test9 ~ {:p9} ||
		test10 ~ {:p10} ||
		test11 ~ {:p11} ||
		test12 ~ {:p12} ||
		test13 ~ {:p13}
	`)

	replacements := []dbx.Params{
		{"p1": `abc`},
		{"p2": `ab%c`},
		{"p3": `ab\%c`},
		{"p4": `%ab\%c`},
		{"p5": `ab\\%c`},
		{"p6": `ab\\\%c`},
		{"p7": `ab_c`},
		{"p8": `ab\_c`},
		{"p9": `%ab_c`},
		{"p10": `ab\c`},
		{"p11": `_ab\c_`},
		{"p12": `ab\c%`},
		{"p13": "a\nb\\"},
	}

	expr, err := filter.BuildExpr(resolver, replacements...)
	if err != nil {
		t.Fatal(err)
	}

	db.Select().Where(expr).Build().Execute()

	if len(calledQueries) != 1 {
		t.Fatalf("Expected 1 query, got %d", len(calledQueries))
	}

	expectedQuery := `SELECT * WHERE (LOWER([[test1]]) LIKE LOWER('%abc%') ESCAPE '\' OR LOWER([[test2]]) LIKE LOWER('ab%c') ESCAPE '\' OR LOWER([[test3]]) LIKE LOWER('%ab\%c%') ESCAPE '\' OR LOWER([[test4]]) LIKE LOWER('%ab\%c') ESCAPE '\' OR LOWER([[test5]]) LIKE LOWER('ab\\%c') ESCAPE '\' OR LOWER([[test6]]) LIKE LOWER('%ab\\\%c%') ESCAPE '\' OR LOWER([[test7]]) LIKE LOWER('%ab\_c%') ESCAPE '\' OR LOWER([[test8]]) LIKE LOWER('%ab\_c%') ESCAPE '\' OR LOWER([[test9]]) LIKE LOWER('%ab_c') ESCAPE '\' OR LOWER([[test10]]) LIKE LOWER('%ab\\c%') ESCAPE '\' OR LOWER([[test11]]) LIKE LOWER('%\_ab\\c\_%') ESCAPE '\' OR LOWER([[test12]]) LIKE LOWER('ab\c%') ESCAPE '\' OR LOWER([[test13]]) LIKE LOWER('%a`
	expectedQuery += "\n" + `b\\%') ESCAPE '\')`
	if expectedQuery != calledQueries[0] {
		t.Fatalf("Expected query \n%s, \ngot \n%s", expectedQuery, calledQueries[0])
	}
}

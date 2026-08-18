package dbutils_test

import (
	"testing"

	"github.com/arief-fajri/pgbase/tools/dbutils"
)

func TestJSONEach(t *testing.T) {
	result := dbutils.JSONEach("a.b")

	expected := `jsonb_array_elements_text(
			CASE
				WHEN [[a.b]] IS NULL OR [[a.b]]::text = '' THEN '[]'::jsonb
				WHEN left(ltrim([[a.b]]::text), 1) = '[' THEN ([[a.b]]::text)::jsonb
				ELSE jsonb_build_array([[a.b]]::text)
			END
		)`

	if result != expected {
		t.Fatalf("Expected\n%v\ngot\n%v", expected, result)
	}
}

func TestJSONArrayLength(t *testing.T) {
	result := dbutils.JSONArrayLength("a.b")

	expected := `(CASE
			WHEN [[a.b]] IS NULL OR [[a.b]]::text = '' THEN 0
			WHEN left(ltrim([[a.b]]::text), 1) = '[' THEN jsonb_array_length(([[a.b]]::text)::jsonb)
			ELSE 1
		END)`

	if result != expected {
		t.Fatalf("Expected\n%v\ngot\n%v", expected, result)
	}
}

func TestJSONExtract(t *testing.T) {
	scenarios := []struct {
		name     string
		column   string
		path     string
		expected string
	}{
		{
			"empty path",
			"a.b",
			"",
			`(CASE WHEN [[a.b]] IS NOT NULL AND jsonb_typeof([[a.b]]::jsonb) IS NOT NULL
		 THEN [[a.b]]::jsonb #>> '{}'
		 ELSE (jsonb_build_object('pb', [[a.b]]::text)) #>> '{pb}'
		 END)`,
		},
		{
			"starting with array index",
			"a.b",
			"[1].a[2]",
			`(CASE WHEN [[a.b]] IS NOT NULL AND jsonb_typeof([[a.b]]::jsonb) IS NOT NULL
		 THEN [[a.b]]::jsonb #>> '{1,a,2}'
		 ELSE (jsonb_build_object('pb', [[a.b]]::text)) #>> '{pb,1,a,2}'
		 END)`,
		},
		{
			"starting with key",
			"a.b",
			"a.b[2].c",
			`(CASE WHEN [[a.b]] IS NOT NULL AND jsonb_typeof([[a.b]]::jsonb) IS NOT NULL
		 THEN [[a.b]]::jsonb #>> '{a,b,2,c}'
		 ELSE (jsonb_build_object('pb', [[a.b]]::text)) #>> '{pb,a,b,2,c}'
		 END)`,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			result := dbutils.JSONExtract(s.column, s.path)

			if result != s.expected {
				t.Fatalf("Expected\n%v\ngot\n%v", s.expected, result)
			}
		})
	}
}

package core

import (
	"testing"

	"github.com/arief-fajri/pgbase/tools/dbutils"
)

func TestIndexDefKeyDistinguishesFunctionalFromPlain(t *testing.T) {
	scenarios := []struct {
		name     string
		a        string
		b        string
		expected bool // true only if the two definitions are treated as equivalent
	}{
		{
			"functional vs plain email index are different shapes",
			`CREATE UNIQUE INDEX "idx" ON "c" (LOWER("email")) WHERE "email" <> ''`,
			`CREATE UNIQUE INDEX "idx" ON "c" ("email") WHERE "email" <> ''`,
			false,
		},
		{
			"same definition regardless of schema/table identifiers",
			`CREATE INDEX "idx" ON "c" ("title")`,
			`CREATE INDEX "idx" ON "other_table" ("title")`,
			true,
		},
		{
			"collate/sort differences are shape changes",
			`CREATE INDEX "idx" ON "c" ("title" COLLATE "C")`,
			`CREATE INDEX "idx" ON "c" ("title")`,
			false,
		},
		{
			"unique flag difference is a shape change",
			`CREATE UNIQUE INDEX "idx" ON "c" ("title")`,
			`CREATE INDEX "idx" ON "c" ("title")`,
			false,
		},
		{
			"where predicate difference is a shape change",
			`CREATE INDEX "idx" ON "c" ("title") WHERE "title" <> ''`,
			`CREATE INDEX "idx" ON "c" ("title")`,
			false,
		},
		{
			"multi-column order matters",
			`CREATE INDEX "idx" ON "c" ("a", "b")`,
			`CREATE INDEX "idx" ON "c" ("b", "a")`,
			false,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			a := dbutils.ParseIndex(s.a)
			b := dbutils.ParseIndex(s.b)

			got := indexDefKey(a) == indexDefKey(b)

			if got != s.expected {
				t.Fatalf("expected equivalence=%v, got %v\na key: %q\nb key: %q", s.expected, got, indexDefKey(a), indexDefKey(b))
			}
		})
	}
}

func TestIndexDefinitionsEquivalent(t *testing.T) {
	scenarios := []struct {
		name     string
		oldIdx   []string
		newIdx   []string
		expected bool
	}{
		{
			"identical definitions",
			[]string{`CREATE INDEX "idx_a" ON "c" ("a")`, `CREATE UNIQUE INDEX "idx_b" ON "c" (LOWER("email"))`},
			[]string{`CREATE INDEX "idx_a" ON "c" ("a")`, `CREATE UNIQUE INDEX "idx_b" ON "c" (LOWER("email"))`},
			true,
		},
		{
			"order of the list does not matter",
			[]string{`CREATE INDEX "idx_a" ON "c" ("a")`, `CREATE INDEX "idx_b" ON "c" ("b")`},
			[]string{`CREATE INDEX "idx_b" ON "c" ("b")`, `CREATE INDEX "idx_a" ON "c" ("a")`},
			true,
		},
		{
			"added index",
			[]string{`CREATE INDEX "idx_a" ON "c" ("a")`},
			[]string{`CREATE INDEX "idx_a" ON "c" ("a")`, `CREATE INDEX "idx_c" ON "c" ("c")`},
			false,
		},
		{
			"removed index",
			[]string{`CREATE INDEX "idx_a" ON "c" ("a")`, `CREATE INDEX "idx_b" ON "c" ("b")`},
			[]string{`CREATE INDEX "idx_a" ON "c" ("a")`},
			false,
		},
		{
			"replaced plain username index with functional one",
			[]string{`CREATE UNIQUE INDEX "idx_user" ON "c" ("username")`},
			[]string{`CREATE UNIQUE INDEX "idx_user" ON "c" (LOWER("username")) WHERE "username" <> ''`},
			false,
		},
		{
			"same definition after table rename",
			[]string{`CREATE INDEX "idx_a" ON "old_c" ("a")`},
			[]string{`CREATE INDEX "idx_a" ON "new_c" ("a")`},
			true,
		},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			oldCol := NewBaseCollection("old_c")
			oldCol.Indexes = s.oldIdx
			newCol := NewBaseCollection("new_c")
			newCol.Indexes = s.newIdx

			if got := indexDefinitionsEquivalent(oldCol, newCol); got != s.expected {
				t.Fatalf("expected equivalence=%v, got %v\nold: %v\nnew: %v", s.expected, got, s.oldIdx, s.newIdx)
			}
		})
	}
}
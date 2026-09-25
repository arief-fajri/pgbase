package core

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestNoTestEnvFallbacksInProductionCode is the mechanical enforcement of hard
// rule 5 (test env fallbacks `PGTEST_*` must not appear in production code
// paths) and guard rail G-DB-05. W-06 was exactly such a leak in the realtime
// outbox listener; this scan keeps the rule from regressing anywhere in the
// production packages.
func TestNoTestEnvFallbacksInProductionCode(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve the test file path")
	}
	root := filepath.Dir(filepath.Dir(thisFile))

	prodDirs := []string{"apis", "cmd", "core", "examples", "forms", "migrations", "plugins", "tools"}
	var violations []string

	for _, dir := range prodDirs {
		base := filepath.Join(root, dir)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" || d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for i, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, "PGTEST_") {
					rel, relErr := filepath.Rel(root, path)
					if relErr != nil {
						rel = path
					}
					violations = append(violations, rel+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed to scan %s: %v", base, err)
		}
	}

	if len(violations) > 0 {
		t.Fatalf(
			"PGTEST_* found in production code (hard rule 5 / G-DB-05); test env fallbacks are only allowed in tests/ and *_test.go:\n%s",
			strings.Join(violations, "\n"),
		)
	}
}

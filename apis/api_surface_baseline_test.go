package apis_test

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/arief-fajri/pgbase/apis"
	"github.com/arief-fajri/pgbase/tests"
)

// TestAPISurfaceBaseline guards the PocketBase-compatible REST surface against
// silent route drift (method or path added/removed) — methodology gate E4.
//
// The golden file (`api_surface.golden`) is a committed snapshot of the
// registered `/api` routes. Any intentional change to the public surface must
// update it deliberately:
//
//	PGBASE_UPDATE_GOLDEN=1 go test ./apis/ -run TestAPISurfaceBaseline -count=1
//
// and pair it with a fork-delta entry + a regression test (AGENTS.md hard
// rule 2). Unintentional drift fails this test instead of shipping.
func TestAPISurfaceBaseline(t *testing.T) {
	t.Parallel()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("failed to create test app: %v", err)
	}
	defer app.Cleanup()

	finder, err := apis.NewRouter(app)
	if err != nil {
		t.Fatalf("failed to create router: %v", err)
	}

	var apiRoutes []string
	for _, pattern := range finder.Routes() {
		if _, rest, ok := strings.Cut(pattern, " "); ok {
			if strings.HasPrefix(rest, "/api") {
				apiRoutes = append(apiRoutes, pattern)
			}
		} else if strings.HasPrefix(pattern, "/api") {
			apiRoutes = append(apiRoutes, pattern)
		}
	}

	sort.Strings(apiRoutes)
	got := strings.Join(apiRoutes, "\n")
	if got != "" {
		got += "\n"
	}

	goldenPath := filepath.Join("api_surface.golden")

	if os.Getenv("PGBASE_UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("failed to write golden file: %v", err)
		}
		t.Logf("golden file updated: %s", goldenPath)
		return
	}

	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("failed to read golden file %s (regenerate with PGBASE_UPDATE_GOLDEN=1): %v", goldenPath, err)
	}

	want := string(raw)
	if want != got {
		t.Errorf("\nAPI surface drifted from baseline.\n--- want (golden) ---\n%s\n--- got (actual) ---\n%s\nRegenerate with PGBASE_UPDATE_GOLDEN=1 if the change is intentional (fork-delta + regression test required).", want, got)
	}
}

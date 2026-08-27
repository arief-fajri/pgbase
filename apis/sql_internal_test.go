package apis

import "testing"

func TestIsSQLConsoleReadQuery(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		name     string
		query    string
		expected bool
	}{
		{"select", "select 1", true},
		{"with", "\n\tWITH cte AS (SELECT 1) SELECT * FROM cte", true},
		{"show", "SHOW server_version", true},
		{"explain", "EXPLAIN SELECT 1", true},
		{"select prefix is not enough", "selection from users", false},
		{"ambiguous command is write safe", "VACUUM", false},
		{"mutation command", "truncate table users", false},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			result := isSQLConsoleReadQuery(scenario.query)
			if result != scenario.expected {
				t.Fatalf("expected %v, got %v", scenario.expected, result)
			}
		})
	}
}

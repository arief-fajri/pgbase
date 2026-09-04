package core

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestQueryExecModeDSNValid verifies that every supported pgx query exec mode
// (CFG-1) produces a DSN that pgx accepts.
func TestQueryExecModeDSNValid(t *testing.T) {
	config := DBConfig{
		Host: "localhost", Port: 5432, User: "u", Password: "p",
		DBName: "mydb", SSLMode: "disable",
	}

	for _, mode := range []string{"exec", "simple_protocol", "cache_describe", "describe_exec", "cache_statement"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("PB_POSTGRES_DEFAULT_QUERY_EXEC_MODE", mode)
			dsn := buildDSN(config)

			cfg, err := pgconn.ParseConfig(dsn)
			if err != nil {
				t.Fatalf("pgx failed to parse exec-mode DSN %q: %v", dsn, err)
			}

			// the param must survive parsing as a startup runtime parameter
			if cfg.RuntimeParams["default_query_exec_mode"] != mode {
				t.Fatalf("expected default_query_exec_mode=%q in parse result, got %q",
					mode, cfg.RuntimeParams["default_query_exec_mode"])
			}
		})
	}
}

func TestBuildDSNDefaultQueryExecMode(t *testing.T) {
	config := DBConfig{
		Host: "localhost", Port: 5432, User: "u", Password: "p",
		DBName: "db", SSLMode: "disable",
	}

	t.Cleanup(func() {
		t.Setenv("PB_POSTGRES_DEFAULT_QUERY_EXEC_MODE", "")
	})

	// unset -> no exec mode param (pgx default cache_statement is preserved)
	t.Setenv("PB_POSTGRES_DEFAULT_QUERY_EXEC_MODE", "")
	if dsn := buildDSN(config); strings.Contains(dsn, "default_query_exec_mode") {
		t.Fatalf("expected no default_query_exec_mode when unset, got %q", dsn)
	}

	// set -> appended as a startup param
	t.Setenv("PB_POSTGRES_DEFAULT_QUERY_EXEC_MODE", "exec")
	dsn := buildDSN(config)
	if !strings.Contains(dsn, "default_query_exec_mode=exec") {
		t.Fatalf("expected default_query_exec_mode=exec, got %q", dsn)
	}

	// search_path must coexist with the exec mode param
	config.Schema = "tenant"
	dsn = buildDSN(config)
	if !strings.Contains(dsn, "default_query_exec_mode=exec") || !strings.Contains(dsn, "search_path=tenant,public") {
		t.Fatalf("expected both params to coexist, got %q", dsn)
	}
}

func TestBuildDSNConnectTimeout(t *testing.T) {
	config := DBConfig{
		Host: "localhost", Port: 5432, User: "u", Password: "p",
		DBName: "db", SSLMode: "disable",
	}

	t.Cleanup(func() {
		t.Setenv("PB_POSTGRES_CONNECT_TIMEOUT", "")
	})

	// default (env unset) -> connect_timeout=10, and pgx parses it as a 10s dial timeout
	t.Setenv("PB_POSTGRES_CONNECT_TIMEOUT", "")
	dsn := buildDSN(config)
	if !strings.Contains(dsn, "connect_timeout=10") {
		t.Fatalf("expected default connect_timeout=10, got %q", dsn)
	}
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgx failed to parse DSN %q: %v", dsn, err)
	}
	if cfg.ConnectTimeout != 10*time.Second {
		t.Fatalf("expected cfg.ConnectTimeout=10s, got %s", cfg.ConnectTimeout)
	}

	// env override -> connect_timeout uses the override value
	t.Setenv("PB_POSTGRES_CONNECT_TIMEOUT", "3")
	dsn = buildDSN(config)
	if !strings.Contains(dsn, "connect_timeout=3") {
		t.Fatalf("expected connect_timeout=3, got %q", dsn)
	}
	cfg, err = pgconn.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgx failed to parse overridden DSN %q: %v", dsn, err)
	}
	if cfg.ConnectTimeout != 3*time.Second {
		t.Fatalf("expected cfg.ConnectTimeout=3s, got %s", cfg.ConnectTimeout)
	}

	// coexists with search_path and the exec mode param
	t.Setenv("PB_POSTGRES_DEFAULT_QUERY_EXEC_MODE", "exec")
	t.Cleanup(func() { t.Setenv("PB_POSTGRES_DEFAULT_QUERY_EXEC_MODE", "") })
	config.Schema = "tenant"
	dsn = buildDSN(config)
	if !strings.Contains(dsn, "connect_timeout=3") ||
		!strings.Contains(dsn, "default_query_exec_mode=exec") ||
		!strings.Contains(dsn, "search_path=tenant,public") {
		t.Fatalf("expected all params to coexist, got %q", dsn)
	}
	if _, err := pgconn.ParseConfig(dsn); err != nil {
		t.Fatalf("pgx failed to parse combined DSN %q: %v", dsn, err)
	}
}

func TestDefaultPostgresRoleTimeoutActions(t *testing.T) {
	t.Cleanup(func() {
		for _, k := range []string{"PB_POSTGRES_STATEMENT_TIMEOUT", "PB_POSTGRES_LOCK_TIMEOUT"} {
			t.Setenv(k, "")
		}
	})

	// defaults -> SET actions with built-in values
	t.Setenv("PB_POSTGRES_STATEMENT_TIMEOUT", "")
	t.Setenv("PB_POSTGRES_LOCK_TIMEOUT", "")
	actions := defaultPostgresRoleTimeoutActions()
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d: %+v", len(actions), actions)
	}
	joined := strings.Join([]string{actions[0].SQL(), actions[1].SQL()}, "|")
	if !strings.Contains(joined, `SET statement_timeout = '60s'`) ||
		!strings.Contains(joined, `SET lock_timeout = '30s'`) {
		t.Fatalf("expected default SET actions, got %q", joined)
	}

	// env override -> SET with override value
	t.Setenv("PB_POSTGRES_STATEMENT_TIMEOUT", "15s")
	t.Setenv("PB_POSTGRES_LOCK_TIMEOUT", "5s")
	actions = defaultPostgresRoleTimeoutActions()
	joined = strings.Join([]string{actions[0].SQL(), actions[1].SQL()}, "|")
	if !strings.Contains(joined, `SET statement_timeout = '15s'`) ||
		!strings.Contains(joined, `SET lock_timeout = '5s'`) {
		t.Fatalf("expected env override SET actions, got %q", joined)
	}

	// off / 0 -> RESET actions (must not leave a lingering value)
	t.Setenv("PB_POSTGRES_STATEMENT_TIMEOUT", "off")
	t.Setenv("PB_POSTGRES_LOCK_TIMEOUT", "0")
	actions = defaultPostgresRoleTimeoutActions()
	joined = strings.Join([]string{actions[0].SQL(), actions[1].SQL()}, "|")
	if !strings.Contains(joined, `RESET statement_timeout`) ||
		!strings.Contains(joined, `RESET lock_timeout`) {
		t.Fatalf("expected RESET actions for off/0, got %q", joined)
	}
	if strings.Contains(joined, " SET ") {
		t.Fatalf("expected no SET actions when disabled, got %q", joined)
	}
}

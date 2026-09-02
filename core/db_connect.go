package core

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pocketbase/dbx"
)

type DBConfig struct {
	Host         string
	Port         int
	User         string
	Password     string
	DBName       string
	SSLMode      string
	Schema       string
	MaxOpenConns int
	MaxIdleConns int
}

// ResolveDBConfig fills any unset DBConfig field from the PB_POSTGRES_* env
// vars (falling back to the built-in defaults).
//
// It is shared by DefaultDBConnect and by the native pg_dump/pg_restore backup
// path so the two always agree on how host/port/user/password/sslmode are
// resolved. Note that DBName may still be overridden by a custom DBConnect
// closure (see the test harness), so the backup path re-reads the effective
// database name from the live connection rather than relying on this value.
func ResolveDBConfig(config DBConfig) DBConfig {
	if config.Host == "" {
		config.Host = getEnvOrDefault("PB_POSTGRES_HOST", "localhost")
	}
	if config.Port == 0 {
		if v := os.Getenv("PB_POSTGRES_PORT"); v != "" {
			config.Port, _ = strconv.Atoi(v)
		}
		if config.Port == 0 {
			config.Port = 5432
		}
	}
	if config.User == "" {
		config.User = getEnvOrDefault("PB_POSTGRES_USER", "pgbase")
	}
	if config.Password == "" {
		config.Password = os.Getenv("PB_POSTGRES_PASSWORD")
	}
	if config.DBName == "" {
		config.DBName = getEnvOrDefault("PB_POSTGRES_DBNAME", "pgbase")
	}
	if config.SSLMode == "" {
		config.SSLMode = getEnvOrDefault("PB_POSTGRES_SSLMODE", "disable")
	}
	// keep the zero-config fallback in sync with the app-level pool defaults
	// (base.go) so the CLI/raw connection paths never silently exceed the
	// aligned single-instance ceiling; still overridable via generic env vars
	if config.MaxOpenConns == 0 {
		config.MaxOpenConns = getEnvIntOrDefault("PB_POSTGRES_MAX_OPEN_CONNS", DefaultDataMaxOpenConns)
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = getEnvIntOrDefault("PB_POSTGRES_MAX_IDLE_CONNS", DefaultDataMaxIdleConns)
	}

	return config
}

func DefaultDBConnect(config DBConfig) (*dbx.DB, error) {
	config = ResolveDBConfig(config)

	db, err := dbx.Open("pgx", buildDSN(config))
	if err != nil {
		return nil, fmt.Errorf("failed to open dbx: %w", err)
	}

	db.DB().SetMaxOpenConns(config.MaxOpenConns)
	db.DB().SetMaxIdleConns(config.MaxIdleConns)
	db.DB().SetConnMaxIdleTime(getEnvDurationOrDefault("PB_POSTGRES_CONN_MAX_IDLE_TIME", 3*time.Minute))

	// Cap the total lifetime of a pooled connection so long-lived connections
	// are periodically recycled. This helps clients gracefully pick up
	// server-side changes (e.g. a rolling Postgres restart, failover to a new
	// primary, or a PgBouncer redeploy) instead of clinging to stale sockets.
	db.DB().SetConnMaxLifetime(getEnvDurationOrDefault("PB_POSTGRES_CONN_MAX_LIFETIME", 30*time.Minute))

	return db, nil
}

func buildDSN(config DBConfig) string {
	sslmode := config.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}

	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password,
		config.DBName, sslmode,
	)

	// Fail fast when opening a NEW connection during an outage/partition instead
	// of blocking on the OS connect timeout (which can be ~75s+). This bounds
	// pool refill and the initial dial; it does NOT affect in-flight queries
	// (those are bounded client-side by withWriteDeadline / queryTimeoutHook and
	// server-side by statement_timeout). Value is in seconds (libpq semantics
	// understood by pgx). Keepalive DSN params are intentionally omitted because
	// the pgx v5 stdlib driver does not parse libpq `keepalives_*`.
	dsn += fmt.Sprintf(" connect_timeout=%d", getEnvIntOrDefault("PB_POSTGRES_CONNECT_TIMEOUT", 10))

	// Optional pgx client-side query exec mode (CFG-1). The pgx default
	// `cache_statement` uses per-connection named prepared statements, which
	// PgBouncer *transaction* pooling cannot serve (the backend changes per
	// transaction → "prepared statement does not exist" errors). Fronting with
	// PgBouncer transaction mode therefore requires `exec` or
	// `simple_protocol` here. Passing it as a DSN startup parameter is correct
	// because it is a pgx *client* setting (not a Postgres server GUC), so it
	// does not depend on server option forwarding.
	if v := os.Getenv("PB_POSTGRES_DEFAULT_QUERY_EXEC_MODE"); v != "" {
		dsn += " default_query_exec_mode=" + v
	}

	if config.Schema != "" {
		dsn += " search_path=" + config.Schema + ",public"
	}
	return dsn
}

// postgresRoleTimeoutAction is a single role-level timeout mutation: SET when
// Value is non-empty, RESET when Value is empty (see defaultPostgresRoleTimeoutActions).
type postgresRoleTimeoutAction struct {
	param string
	value string
}

// defaultPostgresRoleTimeoutActions returns the effective role-level timeout
// actions derived from env. Unset/empty uses the built-in defaults (SET);
// "off"/"0" explicitly resets the role setting (RESET) so the previous boot's
// value does not linger.
func defaultPostgresRoleTimeoutActions() []postgresRoleTimeoutAction {
	var actions []postgresRoleTimeoutAction

	for _, d := range []struct {
		param string
		env   string
		def   string
	}{
		{"statement_timeout", "PB_POSTGRES_STATEMENT_TIMEOUT", "60s"},
		{"lock_timeout", "PB_POSTGRES_LOCK_TIMEOUT", "30s"},
	} {
		value := os.Getenv(d.env)
		if value == "" {
			value = d.def
		}
		if value == "off" || value == "0" {
			actions = append(actions, postgresRoleTimeoutAction{param: d.param})
			continue
		}
		actions = append(actions, postgresRoleTimeoutAction{param: d.param, value: value})
	}

	return actions
}

// SQL returns the ALTER ROLE expression for the action.
func (a postgresRoleTimeoutAction) SQL() string {
	if a.value == "" {
		return fmt.Sprintf("ALTER ROLE CURRENT_USER RESET %s", a.param)
	}
	return fmt.Sprintf("ALTER ROLE CURRENT_USER SET %s = %s", a.param, quoteParamValue(a.value))
}

// roleTimeoutAlterAttempts is how many times a single ALTER ROLE statement is
// retried before giving up.
const roleTimeoutAlterAttempts = 3

// roleTimeoutAlterRetryDelay is the backoff between ALTER ROLE retries.
const roleTimeoutAlterRetryDelay = 25 * time.Millisecond

// ensurePostgresRoleTimeouts applies server-side statement/lock timeouts at the
// PostgreSQL role level (ALTER ROLE CURRENT_USER ...), so they hold across
// every connection that uses the app role - including connections fronted by
// PgBouncer transaction pooling (TX-2).
//
// The role-level approach is used instead of DSN `options=-c` because PgBouncer
// transaction mode does not reliably forward per-client startup options, while
// a role default applies to every server connection opened for the role.
//
// Each statement runs as its own transaction (autocommit) so a transient
// failure cannot poison a surrounding transaction. Under parallel boots (e.g.
// several app instances, or parallel test apps against distinct databases that
// share the same cluster role) two concurrent ALTER ROLE statements can collide
// on pg_authid with "tuple concurrently updated" (XX000); because the applied
// values are idempotent the statements are retried a few times to converge.
//
// It remains best-effort: a role that cannot alter itself (or a restricted
// managed Postgres) logs a warning and the boot continues. The env vars may be
// set to "off"/"0" to RESET the role setting (removing a previous boot's
// value).
func (app *BaseApp) ensurePostgresRoleTimeouts(logger *slog.Logger) error {
	for _, action := range defaultPostgresRoleTimeoutActions() {
		var lastErr error
		for attempt := 0; attempt < roleTimeoutAlterAttempts; attempt++ {
			if attempt > 0 {
				time.Sleep(roleTimeoutAlterRetryDelay)
			}

			if _, err := app.NonconcurrentDB().NewQuery(action.SQL()).Execute(); err != nil {
				lastErr = err
				continue
			}
			lastErr = nil
			break
		}

		if lastErr != nil {
			logger.Warn(
				"Failed to set PostgreSQL role timeout (continuing without it)",
				slog.String("param", action.param),
				slog.String("error", lastErr.Error()),
			)
			continue
		}
	}

	return nil
}

// quoteParamValue quotes a GUC value as a SQL literal so arbitrary env values
// (eg. "60s", "30s") are passed verbatim to ALTER ROLE.
func quoteParamValue(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// getEnvBool returns whether the given env var is set to a truthy value
// ("1", "true", "yes", "on"), case-insensitive.
func getEnvBool(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// getEnvOrDefault returns the given env var value, or defaultVal if unset/empty.
func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// getEnvIntOrDefault returns the positive integer value of the given env var,
// or defaultVal when the var is unset or is not a positive integer.
func getEnvIntOrDefault(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultVal
}

// getEnvDurationOrDefault returns the time.Duration parsed from the given env
// var (e.g. "30m", "1h30m", "90s"), or defaultVal when unset/invalid.
func getEnvDurationOrDefault(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultVal
}

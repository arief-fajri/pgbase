package core

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/pocketbase/dbx"
	_ "github.com/jackc/pgx/v5/stdlib"
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
	if config.MaxOpenConns == 0 {
		config.MaxOpenConns = 100
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = 10
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
	if config.Schema != "" {
		dsn += " search_path=" + config.Schema + ",public"
	}
	return dsn
}

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

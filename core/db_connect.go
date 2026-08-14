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

func DefaultDBConnect(config DBConfig) (*dbx.DB, error) {
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
	if config.MaxOpenConns == 0 {
		config.MaxOpenConns = 100
	}
	if config.MaxIdleConns == 0 {
		config.MaxIdleConns = 10
	}

	db, err := dbx.Open("pgx", buildDSN(config))
	if err != nil {
		return nil, fmt.Errorf("failed to open dbx: %w", err)
	}

	db.DB().SetMaxOpenConns(config.MaxOpenConns)
	db.DB().SetMaxIdleConns(config.MaxIdleConns)
	db.DB().SetConnMaxIdleTime(3 * time.Minute)

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

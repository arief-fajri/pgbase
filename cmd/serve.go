package cmd

import (
	"errors"
	"net/http"

	"github.com/arief-fajri/pgbase/apis"
	"github.com/arief-fajri/pgbase/core"
	"github.com/spf13/cobra"
)

func NewServeCommand(app core.App, showStartBanner bool) *cobra.Command {
	var allowedOrigins []string
	var httpAddr string
	var httpsAddr string

	var pgHost string
	var pgPort int
	var pgUser string
	var pgPassword string
	var pgDBName string
	var pgSSLMode string

	command := &cobra.Command{
		Use:          "serve [domain(s)]",
		Args:         cobra.ArbitraryArgs,
		Short:        "Starts the web server (default to 127.0.0.1:8090 if no domain is specified)",
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			if httpAddr == "" {
				if len(args) > 0 {
					httpAddr = "0.0.0.0:80"
				} else {
					httpAddr = "127.0.0.1:8090"
				}
			}
			if httpsAddr == "" && len(args) > 0 {
				httpsAddr = "0.0.0.0:443"
			}

			err := apis.Serve(app, apis.ServeConfig{
				HttpAddr:           httpAddr,
				HttpsAddr:          httpsAddr,
				ShowStartBanner:    showStartBanner,
				AllowedOrigins:     allowedOrigins,
				CertificateDomains: args,
			})

			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}

			return err
		},
	}

	command.PersistentFlags().StringSliceVar(
		&allowedOrigins,
		"origins",
		[]string{"*"},
		"CORS allowed domain origins list",
	)

	command.PersistentFlags().StringVar(
		&httpAddr,
		"http",
		"",
		"TCP address to listen for the HTTP server",
	)

	command.PersistentFlags().StringVar(
		&httpsAddr,
		"https",
		"",
		"TCP address to listen for the HTTPS server",
	)

	command.PersistentFlags().StringVar(
		&pgHost,
		"pg-host",
		"",
		"PostgreSQL host (or PB_POSTGRES_HOST env var)",
	)

	command.PersistentFlags().IntVar(
		&pgPort,
		"pg-port",
		5432,
		"PostgreSQL port (or PB_POSTGRES_PORT env var)",
	)

	command.PersistentFlags().StringVar(
		&pgUser,
		"pg-user",
		"",
		"PostgreSQL user (or PB_POSTGRES_USER env var)",
	)

	command.PersistentFlags().StringVar(
		&pgPassword,
		"pg-password",
		"",
		"PostgreSQL password (or PB_POSTGRES_PASSWORD env var)",
	)

	command.PersistentFlags().StringVar(
		&pgDBName,
		"pg-dbname",
		"",
		"PostgreSQL database name (or PB_POSTGRES_DBNAME env var)",
	)

	command.PersistentFlags().StringVar(
		&pgSSLMode,
		"pg-sslmode",
		"disable",
		"PostgreSQL SSL mode (or PB_POSTGRES_SSLMODE env var)",
	)

	return command
}

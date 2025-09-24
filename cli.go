package main

import "github.com/urfave/cli/v3"

func NewCLI(action cli.ActionFunc) *cli.Command {
	cmd := &cli.Command{
		Name:        "tsgw",
		Version:     "1.0.0",
		Description: "Tailscale HTTPS Load Balancer - Routes requests based on Host header to configured backends",
		Flags: []cli.Flag{
			// Basic configuration
			&cli.StringFlag{
				Name:     "tailscale-domain",
				Usage:    "Tailscale network domain",
				Required: true,
				Sources:  cli.EnvVars("TSGW_TAILSCALE_DOMAIN"),
			},
			&cli.IntFlag{
				Name:    "http-port",
				Usage:   "HTTP port to listen on, used only to redirect to HTTPS",
				Value:   80,
				Sources: cli.EnvVars("TSGW_HTTP_PORT"),
			},
			&cli.IntFlag{
				Name:    "https-port",
				Usage:   "HTTPS port to listen on",
				Value:   443,
				Sources: cli.EnvVars("TSGW_HTTPS_PORT"),
			},

			// OAuth configuration
			&cli.StringFlag{
				Name:     "oauth-client-id",
				Usage:    "OAuth client ID",
				Required: true,
				Sources:  cli.EnvVars("TSGW_OAUTH_CLIENT_ID"),
			},
			&cli.StringFlag{
				Name:     "oauth-client-secret",
				Usage:    "OAuth client secret",
				Required: true,
				Sources:  cli.EnvVars("TSGW_OAUTH_CLIENT_SECRET"),
			},
			&cli.StringFlag{
				Name:    "oauth-issuer",
				Usage:   "OAuth issuer URL",
				Value:   "https://login.tailscale.com",
				Sources: cli.EnvVars("TSGW_OAUTH_ISSUER"),
			},

			// Routes (repeating flag)
			&cli.StringSliceFlag{
				Name:     "route",
				Aliases:  []string{"r"},
				Usage:    "Route in format 'name=backend_url' (can be specified multiple times)",
				Required: true,
				Sources:  cli.EnvVars("TSGW_ROUTES"),
			},

			// Other options
			&cli.StringFlag{
				Name:    "log-level",
				Usage:   "Log level (trace, debug, info, warn, error, fatal, panic)",
				Value:   "info",
				Sources: cli.EnvVars("TSGW_LOG_LEVEL"),
			},
			&cli.StringFlag{
				Name:    "log-format",
				Usage:   "Log format (json, text)",
				Value:   "console",
				Sources: cli.EnvVars("TSGW_LOG_FORMAT"),
			},
			&cli.BoolFlag{
				Name:    "skip-tls-verify",
				Usage:   "Skip TLS certificate verification for HTTPS backends",
				Sources: cli.EnvVars("TSGW_SKIP_TLS_VERIFY"),
			},
			&cli.StringFlag{
				Name:    "tsnet-dir",
				Usage:   "Directory for Tailscale machine files (default: ./tsnet)",
				Value:   "./tsnet",
				Sources: cli.EnvVars("TSGW_TSNET_DIR"),
			},
			&cli.BoolFlag{
				Name:    "force-cleanup",
				Usage:   "Force cleanup of existing Tailscale state files before starting",
				Sources: cli.EnvVars("TSGW_FORCE_CLEANUP"),
			},

			// OpenTelemetry options
			&cli.BoolFlag{
				Name:    "otel-enabled",
				Usage:   "Enable OpenTelemetry tracing and metrics",
				Sources: cli.EnvVars("TSGW_OTEL_ENABLED"),
			},
			&cli.StringFlag{
				Name:    "otel-service-name",
				Usage:   "OpenTelemetry service name",
				Value:   "tsgw",
				Sources: cli.EnvVars("TSGW_OTEL_SERVICE_NAME"),
			},
			&cli.StringFlag{
				Name:    "otel-endpoint",
				Usage:   "OpenTelemetry OTLP endpoint (e.g., localhost:4317)",
				Sources: cli.EnvVars("TSGW_OTEL_ENDPOINT"),
			},
			&cli.StringFlag{
				Name:    "otel-protocol",
				Usage:   "OpenTelemetry protocol (grpc or http)",
				Value:   "grpc",
				Sources: cli.EnvVars("TSGW_OTEL_PROTOCOL"),
			},
			&cli.BoolFlag{
				Name:    "otel-insecure",
				Usage:   "Skip TLS verification for OTLP endpoint",
				Sources: cli.EnvVars("TSGW_OTEL_INSECURE"),
			},
		},
		Action: action,
	}

	return cmd
}

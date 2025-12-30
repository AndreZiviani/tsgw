package main

import (
	"context"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

func NewCLI(action cli.ActionFunc) *cli.Command {
	cmd := &cli.Command{
		Name:        "tsgw",
		Version:     "1.0.0",
		Description: "Tailscale HTTPS Load Balancer - Routes requests based on Host header to configured backends",
		Flags: []cli.Flag{
			// Basic configuration
			&cli.StringFlag{
				Name:    "tailscale-tag",
				Usage:   "Tailscale tag to assign to gateway nodes (must exist in Tailscale ACLs)",
				Value:   "tsgw",
				Sources: cli.EnvVars("TSGW_TAILSCALE_TAG"),
				Action: func(ctx context.Context, cmd *cli.Command, value string) error {
					if strings.HasPrefix(value, "tag:") {
						return nil
					}
					// Prepend "tag:" if not already present
					return cmd.Set("tailscale-tag", "tag:"+value)
				},
			},
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
				Action: func(ctx context.Context, cmd *cli.Command, values []string) error {
					routes := make(map[string]string)
					for _, v := range values {
						parts := strings.SplitN(v, "=", 2)
						if len(parts) != 2 {
							return cli.Exit("Invalid route format, must be 'name=backend_url'", 1)
						}
						name := strings.TrimSpace(parts[0])
						backend := strings.TrimSpace(parts[1])

						if _, exists := routes[name]; exists {
							return cli.Exit("Duplicate route name: "+name, 1)
						}

						if !strings.HasPrefix(backend, "http://") && !strings.HasPrefix(backend, "https://") {
							return cli.Exit("Backend URL must start with http:// or https:// for route: "+name, 1)
						}

						// Convert route name to lowercase to ensure consistency
						name = strings.ToLower(name)
						routes[name] = backend
					}
					return nil
				},
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

			// Timeouts
			&cli.DurationFlag{
				Name:    "connect-timeout",
				Usage:   "Timeout for establishing backend connections (dial)",
				Value:   30 * time.Second,
				Sources: cli.EnvVars("TSGW_CONNECT_TIMEOUT"),
			},
			&cli.DurationFlag{
				Name:    "request-timeout",
				Usage:   "Per-request timeout for proxying (0 disables; recommended for long-lived streams like Plex)",
				Value:   30 * time.Second,
				Sources: cli.EnvVars("TSGW_REQUEST_TIMEOUT"),
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

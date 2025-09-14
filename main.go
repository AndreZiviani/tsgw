package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"tailscale.com/tsnet"
)

type RouteServer struct {
	RouteName string
	Hostname  string
	Server    *tsnet.Server
	Backend   string
	FQDN      string
}

func main() {
	cmd := &cli.Command{
		Name:        "tsgw",
		Version:     "1.0.0",
		Description: "Tailscale HTTPS Load Balancer - Routes requests based on Host header to configured backends",
		Flags: []cli.Flag{
			// Configuration file
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to configuration file",
				Value:   "config.yaml",
				Sources: cli.EnvVars("TSGW_CONFIG"),
			},

			// Basic configuration
			&cli.StringFlag{
				Name:    "hostname",
				Usage:   "Base hostname for Tailscale nodes",
				Value:   "tsgw",
				Sources: cli.EnvVars("TSGW_HOSTNAME"),
			},
			&cli.StringFlag{
				Name:    "tailscale-domain",
				Usage:   "Tailscale network domain",
				Sources: cli.EnvVars("TSGW_TAILSCALE_DOMAIN"),
			},
			&cli.IntFlag{
				Name:    "port",
				Usage:   "Port to listen on",
				Value:   443,
				Sources: cli.EnvVars("TSGW_PORT"),
			},

			// OAuth configuration
			&cli.StringFlag{
				Name:    "oauth-client-id",
				Usage:   "OAuth client ID",
				Sources: cli.EnvVars("TSGW_OAUTH_CLIENT_ID"),
			},
			&cli.StringFlag{
				Name:    "oauth-client-secret",
				Usage:   "OAuth client secret",
				Sources: cli.EnvVars("TSGW_OAUTH_CLIENT_SECRET"),
			},
			&cli.StringFlag{
				Name:    "oauth-issuer",
				Usage:   "OAuth issuer URL",
				Value:   "https://login.tailscale.com",
				Sources: cli.EnvVars("TSGW_OAUTH_ISSUER"),
			},

			// Routes (repeating flag)
			&cli.StringSliceFlag{
				Name:    "route",
				Aliases: []string{"r"},
				Usage:   "Route in format 'name=backend_url' (can be specified multiple times)",
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
				Name:    "listen-address",
				Usage:   "Optional: Listen on regular network (e.g., ':8080')",
				Sources: cli.EnvVars("TSGW_LISTEN_ADDRESS"),
			},
			&cli.StringFlag{
				Name:    "tsnet-dir",
				Usage:   "Directory for Tailscale machine files (default: ./tsnet)",
				Value:   "./tsnet",
				Sources: cli.EnvVars("TSGW_TSNET_DIR"),
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
		Action: runServer,
	}

	// Create a context that can be cancelled by signals
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := cmd.Run(ctx, os.Args); err != nil {
		log.Fatal().Err(err).Msg("Failed to run TSGW")
	}
}

func runServer(ctx context.Context, cmd *cli.Command) error {
	// Set up default logging (JSON format) before config loading
	// This ensures all log messages during config loading use a consistent format
	// The format can be changed later by SetupLogging if needed
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Caller().Logger()

	// Load configuration with unified loader
	configLoader := NewConfigLoader(cmd)
	config, err := configLoader.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Setup logging from loaded configuration (may change format from default)
	SetupLogging(config)

	// Setup OpenTelemetry
	otel, err := SetupOpenTelemetry(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to setup OpenTelemetry: %w", err)
	}

	log.Info().Msg("Starting TSGW (Tailscale Gateway)")

	log.Info().Int("routes", len(config.Routes)).Str("hostname", config.Hostname).Str("domain", config.TailscaleDomain).Int("port", config.Port).Msg("Configuration loaded")

	// Log all configured routes
	for routeName, backendURL := range config.Routes {
		fqdn := routeName
		if config.TailscaleDomain != "" {
			fqdn = routeName + "." + config.TailscaleDomain
		}
		log.Info().Str("route", routeName).Str("backend", backendURL).Str("fqdn", fqdn).Msg("Configured route")
	}

	// Create a route manager to handle dynamic route updates
	routeManager := NewRouteManager()

	// Setup Echo server with route manager
	log.Info().Msg("Setting up Echo server")
	e := setupEchoServer(config, routeManager, otel)

	// Setup and start all servers in parallel with context
	if err := setupAndStartServers(ctx, config, e, routeManager, otel); err != nil {
		return fmt.Errorf("failed to setup and start servers: %w", err)
	}

	// Wait for shutdown signal
	<-ctx.Done()
	log.Info().Msg("Shutdown signal received, initiating graceful shutdown...")

	// Shutdown OpenTelemetry
	if err := otel.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Error shutting down OpenTelemetry")
	}

	// Close all Tailscale servers
	if err := routeManager.Close(); err != nil {
		log.Error().Err(err).Msg("Error closing Tailscale servers during shutdown")
	}

	// Perform any additional cleanup if needed
	log.Info().Msg("TSGW shutdown completed")
	return nil
}

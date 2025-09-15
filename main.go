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
	"tailscale.com/client/tailscale/v2"
)

type server struct {
	config   *Config
	otel     *OpenTelemetry
	tsClient *tailscale.Client
}

func main() {
	// Set up default logging (JSON format) before config loading
	// This ensures all log messages during config loading use a consistent format
	// The format can be changed later by SetupLogging if needed
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Caller().Logger()

	// Create a context that can be cancelled by signals
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cmd := NewCLI(runServer)

	if err := cmd.Run(ctx, os.Args); err != nil {
		log.Fatal().Err(err).Msg("Failed to run TSGW")
	}
}

func runServer(ctx context.Context, cmd *cli.Command) error {

	// Build configuration directly from CLI flags (which include environment variables)
	config := buildConfigFromCLI(cmd)

	// Setup logging from loaded configuration (may change format from default)
	SetupLogging(config)

	// Setup OpenTelemetry
	otel, err := SetupOpenTelemetry(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to setup OpenTelemetry: %w", err)
	}

	log.Info().Msg("Starting TSGW (Tailscale Gateway)")

	log.Info().Int("routes", len(config.Routes)).Str("hostname", config.Hostname).Str("domain", config.TailscaleDomain).Int("port", config.Port).Msg("Configuration loaded")

	// Create shared Tailscale client
	tsClient, err := createTailscaleClient(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create Tailscale client: %w", err)
	}

	server := &server{
		config:   config,
		otel:     otel,
		tsClient: tsClient,
	}

	server.LogRoutes()

	if err := server.Start(ctx); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Wait for shutdown signal
	<-ctx.Done()
	log.Info().Msg("Shutdown signal received, initiating graceful shutdown...")

	// Shutdown OpenTelemetry
	if err := otel.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Error shutting down OpenTelemetry")
	}

	log.Info().Msg("TSGW shutdown completed")
	return nil
}

func (s *server) LogRoutes() {
	for routeName, backendURL := range s.config.Routes {
		fqdn := routeName + "." + s.config.TailscaleDomain
		log.Info().Str("route", routeName).Str("backend", backendURL).Str("fqdn", fqdn).Msg("Configured route")
	}
}

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"tailscale.com/client/tailscale/v2"
)

type server struct {
	config   *Config
	otel     *OpenTelemetry
	pyro     *Pyroscope
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

	// Setup Pyroscope continuous profiling (optional)
	pyro, err := SetupPyroscope(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to setup Pyroscope: %w", err)
	}

	// Setup OpenTelemetry
	otel, err := SetupOpenTelemetry(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to setup OpenTelemetry: %w", err)
	}

	// Always attempt to flush/stop telemetry on exit. Use a fresh timeout context so shutdown
	// still works even when the main context is already canceled by a signal.
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if otel != nil {
			if err := otel.Shutdown(shutdownCtx); err != nil {
				log.Error().Err(err).Msg("Error shutting down OpenTelemetry")
			}
		}
		if pyro != nil {
			if err := pyro.Shutdown(shutdownCtx); err != nil {
				log.Error().Err(err).Msg("Error shutting down Pyroscope")
			}
		}
	}()

	log.Info().Msg("Starting TSGW (Tailscale Gateway)")

	log.Info().Int("routes", len(config.Routes)).Str("domain", config.TailscaleDomain).Int("http-port", config.HTTPPort).Int("https-port", config.HTTPSPort).Msg("Configuration loaded")

	// Create shared Tailscale client
	tsClient, err := createTailscaleClient(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to create Tailscale client: %w", err)
	}

	server := &server{
		config:   config,
		otel:     otel,
		pyro:     pyro,
		tsClient: tsClient,
	}

	server.LogRoutes()

	if err := server.Start(ctx); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	if ctx.Err() != nil {
		log.Info().Msg("Shutdown requested; exiting")
	}
	return nil
}

func (s *server) LogRoutes() {
	for routeName, backendURL := range s.config.Routes {
		log.Info().Str("service", "svc:"+routeName).Str("backend", backendURL).Msg("Configured route")
	}
}

package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
	tailscale "tailscale.com/client/tailscale/v2"
)

// setupAndStartServers creates and starts all Tailscale servers in parallel, then starts HTTP servers
func setupAndStartServers(ctx context.Context, config *Config, e *echo.Echo, routeManager *RouteManager, otel *OpenTelemetry) error {
	ctx, span := otel.Tracer.Start(ctx, "setupAndStartServers")
	defer span.End()

	log.Info().Int("routes", len(config.Routes)).Msg("Starting parallel initialization of Tailscale servers")

	// Validate OAuth configuration
	if config.OAuth.ClientID == "" || config.OAuth.ClientSecret == "" {
		return fmt.Errorf("OAuth client_id and client_secret are required")
	}

	// Create context with initialization timeout
	initCtx, cancel := context.WithTimeout(ctx, config.InitTimeout)
	defer cancel()

	tsClient, err := createTailscaleClient(initCtx, config)
	if err != nil {
		return fmt.Errorf("failed to create Tailscale client: %w", err)
	}

	// Initialize Tailscale servers in parallel using errgroup
	g, gctx := errgroup.WithContext(initCtx)

	// Channel to collect successful route servers
	routeServers := make(chan *RouteServer, len(config.Routes))

	// Initialize Tailscale servers in parallel
	for routeName, backendURL := range config.Routes {
		routeName, backendURL := routeName, backendURL // capture loop variables
		g.Go(func() error {
			return initializeRouteServer(gctx, config, tsClient, routeName, backendURL, routeServers, otel)
		})
	}

	// Wait for all initialization to complete
	if err := g.Wait(); err != nil {
		return fmt.Errorf("failed to initialize route servers: %w", err)
	}
	close(routeServers)

	// Add all successful route servers to the manager
	for routeServer := range routeServers {
		routeManager.Add(routeServer.FQDN, routeServer)
	}

	log.Info().Int("servers", routeManager.Count()).Msg("All Tailscale servers initialized in parallel")

	// Now start the HTTP servers in parallel
	return startHTTPServers(ctx, config, routeManager, e)
}

// initializeRouteServer initializes a single route server
func initializeRouteServer(ctx context.Context, config *Config, tsClient *tailscale.Client, routeName, backendURL string, results chan<- *RouteServer, otel *OpenTelemetry) error {
	ctx, span := otel.Tracer.Start(ctx, "initializeRouteServer",
		trace.WithAttributes(
			attribute.String("route.name", routeName),
			attribute.String("route.backend", backendURL),
		))
	defer span.End()

	log.Info().Str("route", routeName).Str("backend", backendURL).Msg("Starting initialization for route")

	hostname := routeName
	fqdn := routeName
	if config.TailscaleDomain != "" {
		fqdn = routeName + "." + config.TailscaleDomain
	}

	// Use configurable tsnet directory with route-specific subdirectory
	tsnetDir := filepath.Join(config.TsnetDir, routeName)
	log.Debug().Str("route", routeName).Str("hostname", hostname).Str("fqdn", fqdn).Str("tsnetDir", tsnetDir).Msg("Route configuration")

	// Try to reuse existing state first
	var routeServer *RouteServer
	if reusedServer, reuseErr := tryReuseExistingState(routeName, hostname, fqdn, backendURL, tsnetDir); reusedServer != nil {
		routeServer = reusedServer
	} else if reuseErr != nil {
		log.Warn().Err(reuseErr).Str("route", routeName).Msg("Failed to reuse existing state, will create new auth key")
	}

	// If we couldn't reuse, create new auth key and server
	if routeServer == nil {
		log.Info().Str("route", routeName).Msg("Creating new auth key for route")
		authKey, err := createNewAuthKey(ctx, tsClient, routeName, hostname)
		if err != nil {
			log.Error().Err(err).Str("route", routeName).Msg("Failed to create auth key for route")
			return fmt.Errorf("failed to create auth key for route %s: %w", routeName, err)
		}

		log.Info().Str("route", routeName).Msg("Starting new Tailscale server for route")
		server, err := startAndConnectServer(ctx, routeName, hostname, fqdn, tsnetDir, authKey, config.ConnectTimeout, otel)
		if err != nil {
			log.Error().Err(err).Str("route", routeName).Msg("Failed to start and connect server for route")
			return fmt.Errorf("failed to start server for route %s: %w", routeName, err)
		}

		routeServer = createRouteServer(routeName, hostname, fqdn, backendURL, server)
		log.Info().Str("route", routeName).Msg("Route initialization completed (new server)")
	}

	// Send successful route server to results channel
	select {
	case results <- routeServer:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

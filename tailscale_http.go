package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

// startHTTPServers starts the HTTP servers for all route servers
func startHTTPServers(ctx context.Context, config *Config, routeManager *RouteManager, e *echo.Echo) error {
	g, gctx := errgroup.WithContext(ctx)

	// Start a server for each route
	routeManager.mutex.RLock()
	for _, rs := range routeManager.Routes {
		rs := rs // capture loop variable
		g.Go(func() error {
			return startRouteServer(gctx, config, rs, e)
		})
	}
	routeManager.mutex.RUnlock()

	// Optionally start regular network server
	if config.ListenAddress != "" {
		g.Go(func() error {
			return startRegularServer(gctx, config, e)
		})
	}

	// Wait for all servers to complete and return any error
	return g.Wait()
}

// startRouteServer starts a single route server with proper error handling
func startRouteServer(ctx context.Context, config *Config, routeServer *RouteServer, e *echo.Echo) error {
	// Listen on tailnet with TLS for this route
	port := config.Port
	if port == 0 {
		port = 443 // default port
	}
	portStr := fmt.Sprintf(":%d", port)

	ln, err := routeServer.Server.ListenTLS("tcp", portStr)
	if err != nil {
		log.Error().Err(err).Str("route", routeServer.RouteName).Msg("Failed to listen on Tailscale TLS")
		return fmt.Errorf("failed to listen on TLS for route %s: %w", routeServer.RouteName, err)
	}
	defer ln.Close()

	log.Info().Str("route", routeServer.RouteName).Str("fqdn", routeServer.FQDN).Int("port", port).Str("address", ln.Addr().String()).Msg("Tailscale HTTPS server listening for route")
	server := &http.Server{Handler: e}

	// Start server in a goroutine so we can listen for context cancellation
	serverErrChan := make(chan error, 1)
	go func() {
		serverErrChan <- server.Serve(ln)
	}()

	// Wait for either server error or context cancellation
	select {
	case err := <-serverErrChan:
		if err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Str("route", routeServer.RouteName).Msg("Failed to start Tailscale server")
			return fmt.Errorf("failed to start server for route %s: %w", routeServer.RouteName, err)
		}
	case <-ctx.Done():
		log.Info().Str("route", routeServer.RouteName).Msg("Shutting down Tailscale server due to context cancellation")
		if err := server.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Str("route", routeServer.RouteName).Msg("Error shutting down Tailscale server")
		}
	}

	return nil
}

// startRegularServer starts the regular network server with proper error handling
func startRegularServer(ctx context.Context, config *Config, e *echo.Echo) error {
	log.Info().Str("address", config.ListenAddress).Msg("Starting regular network HTTP server")

	// Start server in a goroutine so we can listen for context cancellation
	serverErrChan := make(chan error, 1)
	go func() {
		serverErrChan <- e.Start(config.ListenAddress)
	}()

	// Wait for either server error or context cancellation
	select {
	case err := <-serverErrChan:
		if err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Failed to start regular network server")
			return fmt.Errorf("failed to start regular network server: %w", err)
		}
	case <-ctx.Done():
		log.Info().Msg("Shutting down regular network server due to context cancellation")
		if err := e.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Msg("Error shutting down regular network server")
		}
	}

	return nil
}

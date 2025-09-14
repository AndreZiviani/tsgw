package main

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/labstack/echo/v4"
	gommon "github.com/labstack/gommon/log"
	"github.com/rs/zerolog/log"
	lecho "github.com/ziflex/lecho/v3"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

// RouteManager manages the route mappings with thread-safe operations
type RouteManager struct {
	Routes map[string]*RouteServer
	mutex  sync.RWMutex
}

// NewRouteManager creates a new RouteManager instance
func NewRouteManager() *RouteManager {
	return &RouteManager{
		Routes: make(map[string]*RouteServer),
	}
}

// Get returns a route by FQDN
func (rm *RouteManager) Get(route string) *RouteServer {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	return rm.Routes[route]
}

// Add adds a new route to the manager
func (rm *RouteManager) Add(route string, server *RouteServer) {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	rm.Routes[route] = server
	log.Debug().Str("route", server.RouteName).Str("fqdn", route).Str("backend", server.Backend).Msg("Added route to manager")
}

// Count returns the number of routes
func (rm *RouteManager) Count() int {
	rm.mutex.RLock()
	defer rm.mutex.RUnlock()

	return len(rm.Routes)
}

// Close closes all Tailscale servers in the manager
func (rm *RouteManager) Close() error {
	rm.mutex.Lock()
	defer rm.mutex.Unlock()

	log.Info().Int("routes", len(rm.Routes)).Msg("Closing all Tailscale servers")

	var errs []error
	for route, routeServer := range rm.Routes {
		if routeServer.Server != nil {
			log.Debug().Str("route", routeServer.RouteName).Str("fqdn", route).Msg("Closing Tailscale server")
			if err := routeServer.Server.Close(); err != nil {
				log.Error().Err(err).Str("route", routeServer.RouteName).Msg("Error closing Tailscale server")
				errs = append(errs, fmt.Errorf("failed to close server for route %s: %w", routeServer.RouteName, err))
			}
		}
	}

	// Clear the routes map
	rm.Routes = make(map[string]*RouteServer)

	if len(errs) > 0 {
		return fmt.Errorf("errors occurred while closing servers: %v", errs)
	}

	log.Info().Msg("All Tailscale servers closed successfully")
	return nil
}

func createRequestHandler(routeManager *RouteManager, config *Config) echo.HandlerFunc {
	return func(c echo.Context) error {
		host := c.Request().Host

		if routeServer := routeManager.Get(host); routeServer != nil {
			log.Debug().Str("host", host).Str("route", routeServer.RouteName).Str("backend", routeServer.Backend).Msg("Route found, proxying request")
			return proxyRequest(c, routeServer.Backend, config.SkipTLSVerify, config.RequestTimeout)
		}

		log.Warn().Str("host", host).Msg("Host not found in routes")
		return c.String(http.StatusNotFound, "Host not found")
	}
}

// setupEchoServer creates and configures the Echo server with routing
func setupEchoServer(config *Config, routeManager *RouteManager, otel *OpenTelemetry) *echo.Echo {
	// Create Echo instance
	e := echo.New()
	e.HideBanner = true

	// Configure lecho logger with zerolog
	lechoLogger := lecho.From(log.Logger)
	lechoLogger.SetLevel(gommon.INFO)

	// Set lecho as the logger for Echo
	e.Logger = lechoLogger
	e.Use(
		lecho.Middleware(
			lecho.Config{
				Logger: lechoLogger,
			},
		),
	)

	// Add OpenTelemetry middleware if enabled
	if config.OpenTelemetry.Enabled {
		e.Use(otelecho.Middleware(config.OpenTelemetry.ServiceName))
		log.Info().Msg("OpenTelemetry Echo middleware enabled")
	}

	// Create and set up the main request handler
	requestHandler := createRequestHandler(routeManager, config)

	// Catch-all route for proxying
	e.Any("/*", requestHandler)

	return e
}

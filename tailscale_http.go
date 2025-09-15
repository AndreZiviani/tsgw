package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
	gommon "github.com/labstack/gommon/log"
	"github.com/rs/zerolog/log"
	lecho "github.com/ziflex/lecho/v3"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"tailscale.com/tsnet"
)

type RouteServer struct {
	RouteName string
	Server    *tsnet.Server
	Backend   string

	config *Config
	otel   *OpenTelemetry

	echo *echo.Echo
}

// RouteProxy holds the pre-configured proxy for a route
type RouteProxy struct {
	Proxy          *httputil.ReverseProxy
	RouteName      string
	BackendURL     string
	RequestTimeout time.Duration
	TargetURL      *url.URL // Pre-parsed target URL
}

func NewRouteServer(routeName string, server *tsnet.Server, backend string, config *Config, otel *OpenTelemetry) (*RouteServer, error) {
	rs := &RouteServer{
		RouteName: routeName,
		Server:    server,
		Backend:   backend,
		config:    config,
		otel:      otel,
	}

	err := rs.initEcho()
	if err != nil {
		return nil, err
	}

	return rs, nil
}

// newEcho creates a dedicated Echo instance for a single route
func (rs *RouteServer) initEcho() error {
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
	if rs.config.OpenTelemetry.Enabled {
		e.Use(otelecho.Middleware(rs.config.OpenTelemetry.ServiceName))
		log.Info().Str("route", rs.RouteName).Msg("OpenTelemetry Echo middleware enabled")
	}

	// Create pre-configured proxy during initialization
	routeProxy, err := rs.newRouteProxy()
	if err != nil {
		log.Error().Err(err).Str("route", rs.RouteName).Msg("Failed to create route proxy")
		return err
	}

	// Catch-all route for proxying
	e.Any("/*", routeProxy.handler)

	rs.echo = e

	return nil
}

// newRouteProxy creates a pre-configured proxy for a route during initialization
func (rs *RouteServer) newRouteProxy() (*RouteProxy, error) {
	// Parse backend URL once during initialization
	target, err := url.Parse(rs.Backend)
	if err != nil {
		log.Error().Err(err).Str("backendURL", rs.Backend).Msg("Failed to parse backend URL")
		return nil, err
	}

	// Create reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Configure transport for HTTPS backends once during initialization
	if target.Scheme == "https" {
		proxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: rs.config.SkipTLSVerify,
			},
		}
		log.Debug().Str("route", rs.RouteName).Str("backend", target.String()).Bool("skip_tls_verify", rs.config.SkipTLSVerify).Msg("Configured HTTPS proxy")
	}

	return &RouteProxy{
		Proxy:          proxy,
		RouteName:      rs.RouteName,
		BackendURL:     rs.Backend,
		RequestTimeout: rs.config.RequestTimeout,
		TargetURL:      target,
	}, nil
}

// handler serves a proxy request using a pre-configured proxy
func (rp *RouteProxy) handler(c echo.Context) error {
	// Create context with timeout for the request
	ctx, cancel := context.WithTimeout(c.Request().Context(), rp.RequestTimeout)
	defer cancel()

	// Replace request context
	c.SetRequest(c.Request().WithContext(ctx))

	// Store original request state
	originalURL := c.Request().URL
	originalHost := c.Request().Host

	// Modify request for proxying using pre-parsed target URL
	c.Request().URL.Scheme = rp.TargetURL.Scheme
	c.Request().URL.Host = rp.TargetURL.Host
	c.Request().Host = rp.TargetURL.Host

	// Remove hop-by-hop headers
	for _, h := range []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailers", "Transfer-Encoding", "Upgrade"} {
		c.Request().Header.Del(h)
	}

	log.Debug().Str("route", rp.RouteName).Str("backend", rp.BackendURL).Str("path", c.Request().URL.Path).Msg("Proxying request")

	// Serve via pre-configured proxy
	rp.Proxy.ServeHTTP(c.Response(), c.Request())

	// Restore original request state (though this won't be reached after ServeHTTP)
	c.Request().URL = originalURL
	c.Request().Host = originalHost

	return nil
}

func (rs *RouteServer) Start(ctx context.Context) error {
	// Listen on tailnet with TLS for this route
	portStr := fmt.Sprintf(":%d", rs.config.Port)

	ln, err := rs.Server.ListenTLS("tcp", portStr)
	if err != nil {
		log.Error().Err(err).Str("route", rs.RouteName).Msg("Failed to listen on Tailscale TLS")
		return fmt.Errorf("failed to listen on TLS for route %s: %w", rs.RouteName, err)
	}
	defer ln.Close()

	log.Info().Str("route", rs.RouteName).Str("fqdn", rs.RouteName+"."+rs.config.TailscaleDomain).Int("port", rs.config.Port).Str("address", ln.Addr().String()).Msg("Tailscale HTTPS server listening for route")
	server := &http.Server{Handler: rs.echo}

	// Start server in a goroutine so we can listen for context cancellation
	serverErrChan := make(chan error, 1)
	go func() {
		serverErrChan <- server.Serve(ln)
	}()

	// Wait for either server error or context cancellation
	select {
	case err := <-serverErrChan:
		if err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Str("route", rs.RouteName).Msg("Failed to start Tailscale server")
			return fmt.Errorf("failed to start server for route %s: %w", rs.RouteName, err)
		}
	case <-ctx.Done():
		log.Info().Str("route", rs.RouteName).Msg("Shutting down Tailscale server due to context cancellation")
		if err := server.Shutdown(context.Background()); err != nil {
			log.Error().Err(err).Str("route", rs.RouteName).Msg("Error shutting down Tailscale server")
		}
	}

	return nil
}

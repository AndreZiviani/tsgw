package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	gommon "github.com/labstack/gommon/log"
	"github.com/rs/zerolog/log"
	lecho "github.com/ziflex/lecho/v3"
	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
	"tailscale.com/tsnet"
)

type proxyBufferPool struct {
	size int
	pool sync.Pool
}

func newProxyBufferPool(size int) *proxyBufferPool {
	bp := &proxyBufferPool{size: size}
	bp.pool.New = func() any {
		return make([]byte, size)
	}
	return bp
}

func (bp *proxyBufferPool) Get() []byte {
	return bp.pool.Get().([]byte)
}

func (bp *proxyBufferPool) Put(b []byte) {
	if bp == nil {
		return
	}
	// Avoid retaining unusually large buffers.
	if cap(b) < bp.size {
		return
	}
	bp.pool.Put(b[:bp.size])
}

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
	lechoLogger.SetLevel(gommonLevelFromString(rs.config.LogLevel))

	// Set lecho as the logger for Echo
	e.Logger = lechoLogger
	e.Pre(middleware.HTTPSRedirect())
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

func gommonLevelFromString(level string) gommon.Lvl {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace", "debug":
		return gommon.DEBUG
	case "info", "":
		return gommon.INFO
	case "warn", "warning":
		return gommon.WARN
	case "error", "fatal", "panic":
		return gommon.ERROR
	default:
		return gommon.INFO
	}
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
	proxy.Transport = rs.newProxyTransport(target)
	proxy.BufferPool = newProxyBufferPool(32 * 1024)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Warn().
			Err(err).
			Str("route", rs.RouteName).
			Str("backend", target.String()).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Msg("Proxy error")
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
	}

	log.Debug().Str("route", rs.RouteName).Str("backend", target.String()).Bool("skip_tls_verify", rs.config.SkipTLSVerify).Msg("Configured proxy transport")

	return &RouteProxy{
		Proxy:          proxy,
		RouteName:      rs.RouteName,
		BackendURL:     rs.Backend,
		RequestTimeout: rs.config.RequestTimeout,
		TargetURL:      target,
	}, nil
}

func (rs *RouteServer) newProxyTransport(target *url.URL) http.RoundTripper {
	// Clone the default transport so we keep sane defaults (proxy env vars, HTTP/2,
	// dialer behavior, etc) while tuning pooling for reverse-proxy workloads.
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}

	tr := base.Clone()

	// Connection pooling: the Go defaults are conservative (MaxIdleConnsPerHost=2),
	// which can cause connection churn and TLS handshakes under load.
	tr.MaxIdleConns = 256
	tr.MaxIdleConnsPerHost = 64
	tr.IdleConnTimeout = 90 * time.Second

	// Good general-purpose timeouts for proxying.
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ExpectContinueTimeout = 1 * time.Second
	tr.ResponseHeaderTimeout = 30 * time.Second
	tr.ForceAttemptHTTP2 = true

	// Use an explicit dialer (still compatible with connection pooling).
	dialTimeout := 30 * time.Second
	if rs.config != nil && rs.config.ConnectTimeout > 0 {
		dialTimeout = rs.config.ConnectTimeout
	}
	tr.DialContext = (&net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
	}).DialContext

	if target != nil && target.Scheme == "https" {
		// Clone any existing TLS config rather than mutating shared pointers.
		var tlsCfg *tls.Config
		if tr.TLSClientConfig != nil {
			tlsCfg = tr.TLSClientConfig.Clone()
		} else {
			tlsCfg = &tls.Config{}
		}
		tlsCfg.InsecureSkipVerify = rs.config.SkipTLSVerify
		tr.TLSClientConfig = tlsCfg
	}

	return tr
}

// handler serves a proxy request using a pre-configured proxy
func (rp *RouteProxy) handler(c echo.Context) error {
	// Optional request timeout (0 disables; recommended for long-lived streams).
	if rp.RequestTimeout > 0 {
		ctx, cancel := context.WithTimeout(c.Request().Context(), rp.RequestTimeout)
		defer cancel()
		c.SetRequest(c.Request().WithContext(ctx))
	}

	log.Debug().Str("route", rp.RouteName).Str("backend", rp.BackendURL).Str("path", c.Request().URL.Path).Msg("Proxying request")

	// Serve via pre-configured proxy
	rp.Proxy.ServeHTTP(c.Response(), c.Request())

	return nil
}

func (rs *RouteServer) Start(ctx context.Context) error {
	lnHTTP, err := rs.Server.Listen("tcp", fmt.Sprintf(":%d", rs.config.HTTPPort))
	if err != nil {
		log.Error().Err(err).Str("route", rs.RouteName).Msg("Failed to listen on Tailscale HTTP")
		return fmt.Errorf("failed to listen on HTTP for route %s: %w", rs.RouteName, err)
	}
	defer lnHTTP.Close()

	lnHTTPS, err := rs.Server.ListenTLS("tcp", fmt.Sprintf(":%d", rs.config.HTTPSPort))
	if err != nil {
		log.Error().Err(err).Str("route", rs.RouteName).Msg("Failed to listen on Tailscale TLS")
		return fmt.Errorf("failed to listen on TLS for route %s: %w", rs.RouteName, err)
	}
	defer lnHTTPS.Close()

	log.Info().Str("route", rs.RouteName).Str("fqdn", rs.RouteName+"."+rs.config.TailscaleDomain).Int("http-port", rs.config.HTTPPort).Int("https-port", rs.config.HTTPSPort).Msg("Tailscale servers listening for route")

	// Keep separate server instances per listener (avoid calling Serve twice on the same http.Server).
	httpsServer := &http.Server{
		Handler:           rs.echo,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	httpServer := &http.Server{
		Handler:           rs.echo,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	// Start server in a goroutine so we can listen for context cancellation
	serverErrChan := make(chan error, 2)
	go func() {
		serverErrChan <- httpServer.Serve(lnHTTP)
	}()
	go func() {
		serverErrChan <- httpsServer.Serve(lnHTTPS)
	}()

	// Wait for either server error or context cancellation.
	// IMPORTANT: use a bounded timeout for shutdown; long-lived connections (e.g. WebSockets/streams)
	// can block Shutdown forever otherwise.
	select {
	case err := <-serverErrChan:
		// If a listener stops unexpectedly while the context is still active, treat it as an error.
		if err == http.ErrServerClosed {
			if ctx.Err() != nil {
				return nil
			}
			_ = httpServer.Close()
			_ = httpsServer.Close()
			return fmt.Errorf("route %s server stopped unexpectedly", rs.RouteName)
		}
		if err != nil {
			log.Error().Err(err).Str("route", rs.RouteName).Msg("Failed to start Tailscale server")
			_ = httpServer.Close()
			_ = httpsServer.Close()
			return fmt.Errorf("failed to start server for route %s: %w", rs.RouteName, err)
		}
	case <-ctx.Done():
		log.Info().Str("route", rs.RouteName).Msg("Shutdown requested; stopping HTTP servers")

		httpServer.SetKeepAlivesEnabled(false)
		httpsServer.SetKeepAlivesEnabled(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Warn().Err(err).Str("route", rs.RouteName).Msg("HTTP graceful shutdown timed out; forcing close")
			_ = httpServer.Close()
		}
		if err := httpsServer.Shutdown(shutdownCtx); err != nil {
			log.Warn().Err(err).Str("route", rs.RouteName).Msg("HTTPS graceful shutdown timed out; forcing close")
			_ = httpsServer.Close()
		}

		// Drain both Serve goroutines to avoid leaks.
		for i := 0; i < 2; i++ {
			err := <-serverErrChan
			if err != nil && err != http.ErrServerClosed {
				log.Debug().Err(err).Str("route", rs.RouteName).Msg("Serve loop exited")
			}
		}
	}

	return nil
}

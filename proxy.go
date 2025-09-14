package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/rs/zerolog/log"
)

// parseBackendURL parses and validates the backend URL
func parseBackendURL(backendURL string) (*url.URL, error) {
	target, err := url.Parse(backendURL)
	if err != nil {
		log.Error().Err(err).Str("backendURL", backendURL).Msg("Failed to parse backend URL")
		return nil, err
	}
	return target, nil
}

// setupProxyTransport configures the proxy transport for HTTPS backends
func setupProxyTransport(target *url.URL, skipTLSVerify bool) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(target)

	// Configure transport for HTTPS backends
	if target.Scheme == "https" {
		proxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: skipTLSVerify,
			},
		}
		log.Debug().Str("backend", target.String()).Bool("skip_tls_verify", skipTLSVerify).Msg("Configured HTTPS proxy")
	}

	return proxy
}

// modifyRequest modifies the request for proxying
func modifyRequest(c echo.Context, target *url.URL) (originalURL *url.URL, originalHost string) {
	originalURL = c.Request().URL
	originalHost = c.Request().Host

	c.Request().URL.Scheme = target.Scheme
	c.Request().URL.Host = target.Host
	c.Request().Host = target.Host

	// Remove hop-by-hop headers
	for _, h := range []string{"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "TE", "Trailers", "Transfer-Encoding", "Upgrade"} {
		c.Request().Header.Del(h)
	}

	return originalURL, originalHost
}

// restoreRequest restores the original request state
func restoreRequest(c echo.Context, originalURL *url.URL, originalHost string) {
	c.Request().URL = originalURL
	c.Request().Host = originalHost
}

// proxyRequest proxies the request to the specified backend
func proxyRequest(c echo.Context, backendURL string, skipTLSVerify bool, requestTimeout time.Duration) error {
	// Parse backend URL
	target, err := parseBackendURL(backendURL)
	if err != nil {
		return err
	}

	// Setup proxy transport
	proxy := setupProxyTransport(target, skipTLSVerify)

	// Create context with timeout for the request
	ctx, cancel := context.WithTimeout(c.Request().Context(), requestTimeout)
	defer cancel()

	// Replace request context
	c.SetRequest(c.Request().WithContext(ctx))

	// Modify request for proxying
	originalURL, originalHost := modifyRequest(c, target)

	// Restore original URL and Host for Echo routing
	defer restoreRequest(c, originalURL, originalHost)

	log.Debug().Str("host", c.Request().Host).Str("backend", backendURL).Str("path", c.Request().URL.Path).Msg("Proxying request")

	// Serve via proxy
	proxy.ServeHTTP(c.Response(), c.Request())
	return nil
}

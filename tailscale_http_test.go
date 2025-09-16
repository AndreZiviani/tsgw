package main

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"tailscale.com/tsnet"
)

func TestNewRouteServer(t *testing.T) {
	tests := []struct {
		name        string
		routeName   string
		backend     string
		expectError bool
	}{
		{
			name:        "valid route server",
			routeName:   "app",
			backend:     "http://app.internal:8080",
			expectError: false,
		},
		{
			name:        "invalid backend URL",
			routeName:   "app",
			backend:     "http://[::1:80/", // Invalid IPv6 URL
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock dependencies
			config := &Config{
				HTTPPort:       80,
				HTTPSPort:      443,
				SkipTLSVerify:  false,
				RequestTimeout: 30 * time.Second,
				OpenTelemetry: OpenTelemetryConfig{
					Enabled: false,
				},
			}

			otel := &OpenTelemetry{
				TracerProvider: nil,
				MeterProvider:  nil,
				Tracer:         nil,
				Meter:          nil,
			}

			// Create a mock tsnet.Server
			tsServer := &tsnet.Server{}

			routeServer, err := NewRouteServer(tt.routeName, tsServer, tt.backend, config, otel)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, routeServer)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, routeServer)
				assert.Equal(t, tt.routeName, routeServer.RouteName)
				assert.Equal(t, tt.backend, routeServer.Backend)
				assert.NotNil(t, routeServer.echo)
			}
		})
	}
}

func TestRouteProxy_Handler(t *testing.T) {
	tests := []struct {
		name          string
		backendURL    string
		requestPath   string
		expectStatus  int
		expectHeaders map[string]string
	}{
		{
			name:         "basic proxy request",
			backendURL:   "http://example.com",
			requestPath:  "/test",
			expectStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse the backend URL
			target, err := url.Parse(tt.backendURL)
			assert.NoError(t, err)

			// Create a RouteProxy
			routeProxy := &RouteProxy{
				Proxy:          httputil.NewSingleHostReverseProxy(target),
				RouteName:      "test",
				BackendURL:     tt.backendURL,
				RequestTimeout: 30 * time.Second,
				TargetURL:      target,
			}

			// Create a test Echo context
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, tt.requestPath, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			// Test that the handler doesn't panic
			assert.NotPanics(t, func() {
				err := routeProxy.handler(c)
				// The handler will return nil because ServeHTTP doesn't return an error
				// but the actual response is written to the recorder
				assert.NoError(t, err)
			})
		})
	}
}

func TestRouteServer_NewRouteProxy(t *testing.T) {
	tests := []struct {
		name        string
		backend     string
		skipTLS     bool
		expectError bool
	}{
		{
			name:        "HTTP backend",
			backend:     "http://app.internal:8080",
			skipTLS:     false,
			expectError: false,
		},
		{
			name:        "HTTPS backend",
			backend:     "https://api.internal:8443",
			skipTLS:     true,
			expectError: false,
		},
		{
			name:        "invalid URL",
			backend:     "http://[::1:80/", // Invalid IPv6 URL
			skipTLS:     false,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs := &RouteServer{
				RouteName: "test",
				Backend:   tt.backend,
				config: &Config{
					SkipTLSVerify:  tt.skipTLS,
					RequestTimeout: 30 * time.Second,
				},
			}

			routeProxy, err := rs.newRouteProxy()

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, routeProxy)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, routeProxy)
				assert.Equal(t, "test", routeProxy.RouteName)
				assert.Equal(t, tt.backend, routeProxy.BackendURL)
				assert.NotNil(t, routeProxy.Proxy)
				assert.NotNil(t, routeProxy.TargetURL)
			}
		})
	}
}

func TestRouteServer_InitEcho(t *testing.T) {
	t.Run("successful echo initialization", func(t *testing.T) {
		rs := &RouteServer{
			RouteName: "test",
			Backend:   "http://app.internal:8080",
			config: &Config{
				SkipTLSVerify:  false,
				RequestTimeout: 30 * time.Second,
				OpenTelemetry: OpenTelemetryConfig{
					Enabled: false,
				},
			},
			otel: &OpenTelemetry{
				TracerProvider: nil,
				MeterProvider:  nil,
				Tracer:         nil,
				Meter:          nil,
			},
		}

		err := rs.initEcho()

		assert.NoError(t, err)
		assert.NotNil(t, rs.echo)
		assert.NotNil(t, rs.echo.Router())
	})
}

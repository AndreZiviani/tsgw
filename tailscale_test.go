package main

import (
	"context"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func TestSetupAndStartServers(t *testing.T) {
	t.Run("missing_oauth_client_id", func(t *testing.T) {
		config := &Config{
			Routes: map[string]string{
				"test": "http://example.com",
			},
			OAuth: OAuthConfig{
				ClientID:     "",
				ClientSecret: "test-secret",
			},
		}

		routeManager := NewRouteManager()
		e := echo.New()
		e.HideBanner = true

		// This should return an error due to missing OAuth client ID
		err := setupAndStartServers(context.Background(), config, e, routeManager, &OpenTelemetry{
			TracerProvider: tracenoop.NewTracerProvider(),
			MeterProvider:  noop.NewMeterProvider(),
			Tracer:         tracenoop.NewTracerProvider().Tracer("test"),
			Meter:          noop.NewMeterProvider().Meter("test"),
			Metrics:        nil,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth client_id and client_secret are required")
	})

	t.Run("missing_oauth_client_secret", func(t *testing.T) {
		config := &Config{
			Routes: map[string]string{
				"test": "http://example.com",
			},
			OAuth: OAuthConfig{
				ClientID:     "test-id",
				ClientSecret: "",
			},
		}

		routeManager := NewRouteManager()
		e := echo.New()
		e.HideBanner = true

		// This should return an error due to missing OAuth client secret
		err := setupAndStartServers(context.Background(), config, e, routeManager, &OpenTelemetry{
			TracerProvider: tracenoop.NewTracerProvider(),
			MeterProvider:  noop.NewMeterProvider(),
			Tracer:         tracenoop.NewTracerProvider().Tracer("test"),
			Meter:          noop.NewMeterProvider().Meter("test"),
			Metrics:        nil,
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth client_id and client_secret are required")
	})

	t.Run("valid_oauth_config", func(t *testing.T) {
		config := &Config{
			Routes: map[string]string{
				"test": "http://example.com",
			},
			OAuth: OAuthConfig{
				ClientID:     "test-id",
				ClientSecret: "test-secret",
			},
		}

		routeManager := NewRouteManager()
		e := echo.New()
		e.HideBanner = true

		// This would normally try to connect to Tailscale, but since we don't have
		// valid credentials, it will fail. For now, we'll just test that it doesn't
		// panic on the OAuth validation part.
		// In a real test environment, you'd mock the Tailscale client
		t.Skip("Requires Tailscale API mocking - tests OAuth validation passes but connection would fail")
		_ = config       // Avoid unused variable error
		_ = routeManager // Avoid unused variable error
	})

	t.Run("empty_routes", func(t *testing.T) {
		config := &Config{
			Routes: map[string]string{},
			OAuth: OAuthConfig{
				ClientID:     "test-id",
				ClientSecret: "test-secret",
			},
		}

		routeManager := NewRouteManager()
		e := echo.New()
		e.HideBanner = true

		// Test with empty routes - should not panic on OAuth validation
		// but would fail on actual Tailscale connection
		t.Skip("Requires Tailscale API mocking - tests empty routes handling")
		_ = config       // Avoid unused variable error
		_ = routeManager // Avoid unused variable error
		_ = e            // Avoid unused variable error
	})
}

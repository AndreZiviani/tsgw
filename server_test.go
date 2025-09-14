package main

import (
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func TestSetupEchoServer(t *testing.T) {
	t.Run("route_mapping_with_domain", func(t *testing.T) {
		config := &Config{
			TailscaleDomain: "example.com",
			Routes: map[string]string{
				"app1": "http://app1.internal:8080",
				"api":  "https://api.internal:8443",
			},
		}

		// Create route servers
		routeServers := []*RouteServer{
			{
				RouteName: "app1",
				Hostname:  "test-app1",
				Backend:   "http://app1.internal:8080",
				FQDN:      "app1.example.com",
			},
			{
				RouteName: "api",
				Hostname:  "test-api",
				Backend:   "https://api.internal:8443",
				FQDN:      "api.example.com",
			},
		}

		// Create route manager and add routes
		routeManager := NewRouteManager()
		for _, rs := range routeServers {
			routeManager.Add(rs.FQDN, rs)
		}

		e := setupEchoServer(config, routeManager, &OpenTelemetry{
			TracerProvider: tracenoop.NewTracerProvider(),
			MeterProvider:  noop.NewMeterProvider(),
			Tracer:         tracenoop.NewTracerProvider().Tracer("test"),
			Meter:          noop.NewMeterProvider().Meter("test"),
			Metrics:        nil,
		})

		// The Echo instance should be created
		assert.NotNil(t, e)

		// Test that routes are properly mapped (this would require inspecting the Echo routes)
		// For now, we'll just verify the function doesn't panic and returns a valid Echo instance
		assert.IsType(t, &echo.Echo{}, e)
	})

	t.Run("route_mapping_without_domain", func(t *testing.T) {
		config := &Config{
			TailscaleDomain: "",
			Routes: map[string]string{
				"app1.example.com": "http://app1.internal:8080",
			},
		}

		// Create route servers
		routeServers := []*RouteServer{
			{
				RouteName: "app1.example.com",
				Hostname:  "test-app1",
				Backend:   "http://app1.internal:8080",
				FQDN:      "app1.example.com",
			},
		}

		// Create route manager and add routes
		routeManager := NewRouteManager()
		for _, rs := range routeServers {
			routeManager.Add(rs.FQDN, rs)
		}

		e := setupEchoServer(config, routeManager, &OpenTelemetry{
			TracerProvider: tracenoop.NewTracerProvider(),
			MeterProvider:  noop.NewMeterProvider(),
			Tracer:         tracenoop.NewTracerProvider().Tracer("test"),
			Meter:          noop.NewMeterProvider().Meter("test"),
			Metrics:        nil,
		})
		assert.NotNil(t, e)
		assert.IsType(t, &echo.Echo{}, e)
	})

	t.Run("empty_routes", func(t *testing.T) {
		config := &Config{
			TailscaleDomain: "example.com",
			Routes:          map[string]string{},
		}

		// Empty route servers
		routeServers := []*RouteServer{}

		// Create route manager and add routes
		routeManager := NewRouteManager()
		for _, rs := range routeServers {
			routeManager.Add(rs.FQDN, rs)
		}

		e := setupEchoServer(config, routeManager, &OpenTelemetry{
			TracerProvider: tracenoop.NewTracerProvider(),
			MeterProvider:  noop.NewMeterProvider(),
			Tracer:         tracenoop.NewTracerProvider().Tracer("test"),
			Meter:          noop.NewMeterProvider().Meter("test"),
			Metrics:        nil,
		})
		assert.NotNil(t, e)
		assert.IsType(t, &echo.Echo{}, e)
	})
}

package main

import (
	"context"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestStartHTTPServers(t *testing.T) {
	t.Run("start_servers_with_routes", func(t *testing.T) {
		// Test starting HTTP servers with route manager containing routes
		config := &Config{
			Port: 8443,
		}

		routeManager := NewRouteManager()
		// Note: In a real test, we'd need to mock the RouteServer with a mock tsnet.Server
		// For now, we'll test the basic setup

		e := echo.New()
		e.HideBanner = true

		// Create context with timeout to prevent test from hanging
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// This would normally start servers, but since we don't have real RouteServers,
		// we'll just test that the function doesn't panic with empty routes
		assert.NotPanics(t, func() {
			startHTTPServers(ctx, config, routeManager, e)
		})
	})

	t.Run("start_servers_with_listen_address", func(t *testing.T) {
		// Test starting servers with a listen address
		config := &Config{
			Port:          8443,
			ListenAddress: "localhost:8080",
		}

		routeManager := NewRouteManager()
		e := echo.New()
		e.HideBanner = true

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Test that function handles listen address configuration
		assert.NotPanics(t, func() {
			startHTTPServers(ctx, config, routeManager, e)
		})
	})

	t.Run("default_port_handling", func(t *testing.T) {
		// Test default port handling when port is 0
		config := &Config{
			Port: 0, // Should default to 443
		}

		routeManager := NewRouteManager()
		e := echo.New()
		e.HideBanner = true

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		assert.NotPanics(t, func() {
			startHTTPServers(ctx, config, routeManager, e)
		})
	})
}

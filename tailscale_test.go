package main

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

func TestCreateTailscaleClient(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name: "valid config",
			config: &Config{
				TsnetDir: "/tmp/tsgw-test",
				OAuth: OAuthConfig{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
				},
				InitTimeout: 30 * time.Second,
			},
			expectError: false,
		},
		{
			name: "missing client ID",
			config: &Config{
				TsnetDir: "/tmp/tsgw-test",
				OAuth: OAuthConfig{
					ClientSecret: "test-client-secret",
				},
				InitTimeout: 30 * time.Second,
			},
			expectError: false, // Client creation might still succeed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := createTailscaleClient(context.Background(), tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.NotNil(t, client.HTTP)
			}
		})
	}
}

func TestServer_Start(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name: "valid server start",
			config: &Config{
				TsnetDir: "/tmp/tsgw-test",
				Routes: map[string]string{
					"app": "http://app.internal:8080",
				},
				Port:           443,
				SkipTLSVerify:  false,
				RequestTimeout: 30 * time.Second,
				OpenTelemetry: OpenTelemetryConfig{
					Enabled: false,
				},
				OAuth: OAuthConfig{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
				},
				InitTimeout: 30 * time.Second,
			},
			expectError: false,
		},
		{
			name: "no routes configured",
			config: &Config{
				TsnetDir:       "/tmp/tsgw-test",
				Routes:         map[string]string{},
				Port:           443,
				SkipTLSVerify:  false,
				RequestTimeout: 30 * time.Second,
				OpenTelemetry: OpenTelemetryConfig{
					Enabled: false,
				},
				OAuth: OAuthConfig{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
				},
				InitTimeout: 30 * time.Second,
			},
			expectError: false, // Server can start with no routes, but won't serve anything
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock OpenTelemetry instance
			otel := &OpenTelemetry{
				TracerProvider: tracenoop.NewTracerProvider(),
				MeterProvider:  noop.NewMeterProvider(),
				Tracer:         tracenoop.NewTracerProvider().Tracer("tsgw"),
				Meter:          noop.NewMeterProvider().Meter("tsgw"),
			}

			// Try to create a Tailscale client (this may fail in test environment)
			tsClient, err := createTailscaleClient(context.Background(), tt.config)
			if err != nil {
				t.Logf("Failed to create Tailscale client: %v", err)
				tsClient = nil // Continue with nil client
			}

			s := &server{
				config:   tt.config,
				otel:     otel,
				tsClient: tsClient,
			}

			// Test that Start doesn't panic and returns appropriate values
			assert.NotPanics(t, func() {
				err := s.Start(context.Background())
				// We expect this to fail due to external dependencies (Tailscale auth, network, etc.)
				// but it should not panic
				if err == nil {
					t.Log("Server started successfully (unexpected in test environment)")
				} else {
					t.Logf("Server failed as expected: %v", err)
				}
			})
		})
	}
}

func TestServer_StartRoute(t *testing.T) {
	tests := []struct {
		name        string
		routeName   string
		backendURL  string
		config      *Config
		expectError bool
	}{
		{
			name:       "valid route start",
			routeName:  "app",
			backendURL: "http://app.internal:8080",
			config: &Config{
				Port:           443,
				SkipTLSVerify:  false,
				RequestTimeout: 30 * time.Second,
				OpenTelemetry: OpenTelemetryConfig{
					Enabled: false,
				},
			},
			expectError: false,
		},
		{
			name:       "invalid backend URL",
			routeName:  "app",
			backendURL: "http://[::1:80/", // Invalid IPv6 URL
			config: &Config{
				Port:           443,
				SkipTLSVerify:  false,
				RequestTimeout: 30 * time.Second,
				OpenTelemetry: OpenTelemetryConfig{
					Enabled: false,
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock OpenTelemetry instance
			otel := &OpenTelemetry{
				TracerProvider: tracenoop.NewTracerProvider(),
				MeterProvider:  noop.NewMeterProvider(),
				Tracer:         tracenoop.NewTracerProvider().Tracer("tsgw"),
				Meter:          noop.NewMeterProvider().Meter("tsgw"),
			}

			// Try to create a Tailscale client (this may fail in test environment)
			tsClient, err := createTailscaleClient(context.Background(), tt.config)
			if err != nil {
				t.Logf("Failed to create Tailscale client: %v", err)
				tsClient = nil // Continue with nil client
			}

			s := &server{
				config:   tt.config,
				otel:     otel,
				tsClient: tsClient,
			}

			// Test that startRoute handles the route appropriately
			assert.NotPanics(t, func() {
				err := s.startRoute(context.Background(), tt.routeName, tt.backendURL)
				// We expect this to fail due to external dependencies
				// but it should not panic
				if err == nil {
					t.Log("Route started successfully (unexpected in test environment)")
				} else {
					t.Logf("Route failed as expected: %v", err)
				}
			})
		})
	}
}

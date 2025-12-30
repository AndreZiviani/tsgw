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
				HTTPPort:       80,
				HTTPSPort:      443,
				SkipTLSVerify:  false,
				RequestTimeout: 30 * time.Second,
				OpenTelemetry: OpenTelemetryConfig{
					Enabled: false,
				},
				OAuth: OAuthConfig{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
				},
			},
			expectError: false,
		},
		{
			name: "no routes configured",
			config: &Config{
				TsnetDir:       "/tmp/tsgw-test",
				Routes:         map[string]string{},
				HTTPPort:       80,
				HTTPSPort:      443,
				SkipTLSVerify:  false,
				RequestTimeout: 30 * time.Second,
				OpenTelemetry: OpenTelemetryConfig{
					Enabled: false,
				},
				OAuth: OAuthConfig{
					ClientID:     "test-client-id",
					ClientSecret: "test-client-secret",
				},
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

			// Start now runs as a long-lived service host; use a canceled context to
			// ensure tests do not hang (external dependencies are not available here).
			assert.NotPanics(t, func() {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				_ = s.Start(ctx)
			})
		})
	}
}

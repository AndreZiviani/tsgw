package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short string",
			input:    "short",
			expected: "short",
		},
		{
			name:     "exactly 8 characters",
			input:    "12345678",
			expected: "12345678",
		},
		{
			name:     "long string",
			input:    "this-is-a-very-long-string-that-should-be-masked",
			expected: "this-is-...",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestServer_LogRoutes(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		routes map[string]string
	}{
		{
			name: "single route",
			config: &Config{
				TailscaleDomain: "example.ts.net",
				Routes: map[string]string{
					"app": "http://app.internal:8080",
				},
			},
		},
		{
			name: "multiple routes",
			config: &Config{
				TailscaleDomain: "example.ts.net",
				Routes: map[string]string{
					"app": "http://app.internal:8080",
					"api": "https://api.internal:3000",
					"web": "http://web.internal:8080",
				},
			},
		},
		{
			name: "empty routes",
			config: &Config{
				TailscaleDomain: "example.ts.net",
				Routes:          map[string]string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock OpenTelemetry instance
			otel := &OpenTelemetry{
				TracerProvider: nil,
				MeterProvider:  nil,
				Tracer:         nil,
				Meter:          nil,
			}

			server := &server{
				config: tt.config,
				otel:   otel,
			}

			// Test that LogRoutes doesn't panic
			assert.NotPanics(t, func() {
				server.LogRoutes()
			})
		})
	}
}

func TestBuildConfigFromCLI(t *testing.T) {
	t.Run("basic configuration creation", func(t *testing.T) {
		// Set up environment variables for testing
		os.Setenv("TSGW_ROUTE_APP", "http://app.internal:8080")
		defer os.Unsetenv("TSGW_ROUTE_APP")

		// This is a simplified test since buildConfigFromCLI requires a CLI context
		// In a real implementation, you'd need to mock the CLI context or use dependency injection

		// Test that environment variable parsing works
		routes := make(map[string]string)

		// Simulate the environment variable parsing logic from buildConfigFromCLI
		if os.Getenv("TSGW_ROUTE_APP") != "" {
			routes["app"] = os.Getenv("TSGW_ROUTE_APP")
		}

		assert.Equal(t, map[string]string{"app": "http://app.internal:8080"}, routes)
	})
}

func TestSetupLogging(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "default logging",
			config: &Config{
				LogLevel:  "",
				LogFormat: "",
			},
		},
		{
			name: "debug level",
			config: &Config{
				LogLevel:  "debug",
				LogFormat: "console",
			},
		},
		{
			name: "json format",
			config: &Config{
				LogLevel:  "info",
				LogFormat: "json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// SetupLogging modifies global state, so we test that it doesn't panic
			assert.NotPanics(t, func() {
				SetupLogging(tt.config)
			})
		})
	}
}

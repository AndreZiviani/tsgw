package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"
)

func TestConfigLoader_LoadFromFile(t *testing.T) {
	// Create a temporary config file
	validYAML := `
hostname: "test-gateway"
tailscale_domain: "example.com"
oauth:
  client_id: "test-client-id"
  client_secret: "test-client-secret"
port: 8443
log_level: "debug"
skip_tls_verify: true
listen_address: ":8080"
routes:
  "app1": "http://app1.internal:8080"
  "app2": "https://app2.internal:8443"
`

	tmpFile, err := os.CreateTemp("", "config-*.yaml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(validYAML)
	assert.NoError(t, err)
	tmpFile.Close()

	// Create a mock CLI command
	cmd := &cli.Command{}
	cmd.Flags = []cli.Flag{
		&cli.StringFlag{Name: "config", Value: tmpFile.Name()},
	}

	loader := NewConfigLoader(cmd)
	config, err := loader.Load()
	assert.NoError(t, err)
	assert.NotNil(t, config)

	assert.Equal(t, "test-gateway", config.Hostname)
	assert.Equal(t, "example.com", config.TailscaleDomain)
	assert.Equal(t, "test-client-id", config.OAuth.ClientID)
	assert.Equal(t, "test-client-secret", config.OAuth.ClientSecret)
	assert.Equal(t, 8443, config.Port)
	assert.Equal(t, "debug", config.LogLevel)
	assert.True(t, config.SkipTLSVerify)
	assert.Equal(t, ":8080", config.ListenAddress)

	expectedRoutes := map[string]string{
		"app1": "http://app1.internal:8080",
		"app2": "https://app2.internal:8443",
	}
	assert.Equal(t, expectedRoutes, config.Routes)
}

func TestConfigLoader_LoadWithCLIFlags(t *testing.T) {
	// Create a mock CLI command with flags set
	cmd := &cli.Command{}
	cmd.Flags = []cli.Flag{
		&cli.StringFlag{Name: "config", Value: "non-existent.yaml"},
		&cli.StringFlag{Name: "hostname", Value: "cli-hostname"},
		&cli.StringFlag{Name: "tailscale-domain", Value: "cli-domain.com"},
		&cli.StringFlag{Name: "oauth-client-id", Value: "cli-client-id"},
		&cli.StringFlag{Name: "oauth-client-secret", Value: "cli-client-secret"},
		&cli.IntFlag{Name: "port", Value: 9443},
		&cli.BoolFlag{Name: "skip-tls-verify", Value: true},
		&cli.StringSliceFlag{Name: "route"},
	}

	// Manually set the flags as if they were provided via CLI
	cmd.Set("hostname", "cli-hostname")
	cmd.Set("tailscale-domain", "cli-domain.com")
	cmd.Set("oauth-client-id", "cli-client-id")
	cmd.Set("oauth-client-secret", "cli-client-secret")
	cmd.Set("port", "9443")
	cmd.Set("skip-tls-verify", "true")
	cmd.Set("route", "app1=http://cli-app1:8080")

	loader := NewConfigLoader(cmd)
	config, err := loader.Load()
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// CLI flags should override defaults
	assert.Equal(t, "cli-hostname", config.Hostname)
	assert.Equal(t, "cli-domain.com", config.TailscaleDomain)
	assert.Equal(t, "cli-client-id", config.OAuth.ClientID)
	assert.Equal(t, "cli-client-secret", config.OAuth.ClientSecret)
	assert.Equal(t, 9443, config.Port)
	assert.True(t, config.SkipTLSVerify)

	expectedRoutes := map[string]string{
		"app1": "http://cli-app1:8080",
	}
	assert.Equal(t, expectedRoutes, config.Routes)
}

func TestConfigLoader_LoadWithEnvironmentVariables(t *testing.T) {
	// Set environment variables
	envVars := map[string]string{
		"TSGW_HOSTNAME":            "env-hostname",
		"TSGW_TAILSCALE_DOMAIN":    "env-domain.com",
		"TSGW_OAUTH_CLIENT_ID":     "env-client-id",
		"TSGW_OAUTH_CLIENT_SECRET": "env-client-secret",
		"TSGW_PORT":                "9555",
		"TSGW_LOG_LEVEL":           "warn",
		"TSGW_SKIP_TLS_VERIFY":     "true",
		"TSGW_LISTEN_ADDRESS":      ":9555",
	}

	for key, value := range envVars {
		os.Setenv(key, value)
		defer os.Unsetenv(key)
	}

	// Create a mock CLI command
	cmd := &cli.Command{}
	cmd.Flags = []cli.Flag{
		&cli.StringFlag{Name: "config", Value: "non-existent.yaml"},
		&cli.StringSliceFlag{Name: "route"},
	}

	// Set routes via CLI flag
	cmd.Set("route", "app1=http://env-app1:8080")

	loader := NewConfigLoader(cmd)
	config, err := loader.Load()
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Environment variables should override defaults
	assert.Equal(t, "env-hostname", config.Hostname)
	assert.Equal(t, "env-domain.com", config.TailscaleDomain)
	assert.Equal(t, "env-client-id", config.OAuth.ClientID)
	assert.Equal(t, "env-client-secret", config.OAuth.ClientSecret)
	assert.Equal(t, 9555, config.Port)
	assert.Equal(t, "warn", config.LogLevel)
	assert.True(t, config.SkipTLSVerify)
	assert.Equal(t, ":9555", config.ListenAddress)
}

func TestConfigLoader_PriorityOrder(t *testing.T) {
	// Set environment variables (highest priority)
	os.Setenv("TSGW_HOSTNAME", "env-hostname")
	os.Setenv("TSGW_PORT", "9666")
	os.Setenv("TSGW_OAUTH_CLIENT_ID", "env-client-id")
	os.Setenv("TSGW_OAUTH_CLIENT_SECRET", "env-client-secret")
	os.Setenv("TSGW_TAILSCALE_DOMAIN", "env-domain.com")
	defer func() {
		os.Unsetenv("TSGW_HOSTNAME")
		os.Unsetenv("TSGW_PORT")
		os.Unsetenv("TSGW_OAUTH_CLIENT_ID")
		os.Unsetenv("TSGW_OAUTH_CLIENT_SECRET")
		os.Unsetenv("TSGW_TAILSCALE_DOMAIN")
	}()

	// Create a temporary config file (medium priority)
	configYAML := `
hostname: "file-hostname"
port: 8777
oauth:
  client_id: "file-client-id"
  client_secret: "file-client-secret"
tailscale_domain: "file-domain.com"
routes:
  app1: "http://file-app1:8080"
`
	tmpFile, err := os.CreateTemp("", "priority-config-*.yaml")
	assert.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(configYAML)
	assert.NoError(t, err)
	tmpFile.Close()

	// Create a mock CLI command with flags (lowest priority among the three)
	cmd := &cli.Command{}
	cmd.Flags = []cli.Flag{
		&cli.StringFlag{Name: "config", Value: tmpFile.Name()},
		&cli.StringFlag{Name: "hostname", Value: "cli-hostname"},
		&cli.IntFlag{Name: "port", Value: 9888},
		&cli.StringFlag{Name: "oauth-client-id", Value: "cli-client-id"},
		&cli.StringFlag{Name: "oauth-client-secret", Value: "cli-client-secret"},
		&cli.StringFlag{Name: "tailscale-domain", Value: "cli-domain.com"},
		&cli.StringSliceFlag{Name: "route"},
	}

	// Set CLI flags
	cmd.Set("hostname", "cli-hostname")
	cmd.Set("port", "9888")
	cmd.Set("oauth-client-id", "cli-client-id")
	cmd.Set("oauth-client-secret", "cli-client-secret")
	cmd.Set("tailscale-domain", "cli-domain.com")
	cmd.Set("route", "app1=http://cli-app1:8080")

	loader := NewConfigLoader(cmd)
	config, err := loader.Load()
	assert.NoError(t, err)
	assert.NotNil(t, config)

	// Environment variables should have highest priority
	assert.Equal(t, "env-hostname", config.Hostname)
	assert.Equal(t, 9666, config.Port)
}

func TestConfigLoader_Validation(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectError bool
	}{
		{
			name: "valid config",
			config: &Config{
				Hostname:        "test",
				TailscaleDomain: "example.com",
				OAuth: OAuthConfig{
					ClientID:     "client-id",
					ClientSecret: "client-secret",
				},
				Routes: map[string]string{"app": "http://app"},
			},
			expectError: false,
		},
		{
			name: "missing oauth client id",
			config: &Config{
				Hostname:        "test",
				TailscaleDomain: "example.com",
				OAuth: OAuthConfig{
					ClientSecret: "client-secret",
				},
				Routes: map[string]string{"app": "http://app"},
			},
			expectError: true,
		},
		{
			name: "missing oauth client secret",
			config: &Config{
				Hostname:        "test",
				TailscaleDomain: "example.com",
				OAuth: OAuthConfig{
					ClientID: "client-id",
				},
				Routes: map[string]string{"app": "http://app"},
			},
			expectError: true,
		},
		{
			name: "missing tailscale domain",
			config: &Config{
				Hostname: "test",
				OAuth: OAuthConfig{
					ClientID:     "client-id",
					ClientSecret: "client-secret",
				},
				Routes: map[string]string{"app": "http://app"},
			},
			expectError: true,
		},
		{
			name: "missing routes",
			config: &Config{
				Hostname:        "test",
				TailscaleDomain: "example.com",
				OAuth: OAuthConfig{
					ClientID:     "client-id",
					ClientSecret: "client-secret",
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cli.Command{}
			cmd.Flags = []cli.Flag{
				&cli.StringFlag{Name: "config", Value: "non-existent.yaml"},
			}

			loader := NewConfigLoader(cmd)
			err := loader.validate(tt.config)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigLoader_ParseRoutes(t *testing.T) {
	cmd := &cli.Command{}
	loader := NewConfigLoader(cmd)

	tests := []struct {
		name         string
		routeStrings []string
		expected     map[string]string
		expectError  bool
	}{
		{
			name:         "valid routes",
			routeStrings: []string{"app1=http://app1:8080", "app2=https://app2:8443"},
			expected:     map[string]string{"app1": "http://app1:8080", "app2": "https://app2:8443"},
			expectError:  false,
		},
		{
			name:         "invalid route format",
			routeStrings: []string{"invalid-route-format"},
			expected:     map[string]string{},
			expectError:  true,
		},
		{
			name:         "mixed valid and invalid",
			routeStrings: []string{"valid=http://valid", "invalid"},
			expected:     map[string]string{"valid": "http://valid"},
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes, err := loader.parseRoutes(tt.routeStrings)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, routes)
			}
		})
	}
}

func TestMaskString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"a", "a"},
		{"12345678", "12345678"},
		{"123456789", "12345678..."},
		{"very-long-string-that-should-be-masked", "very-lon..."},
	}

	for _, test := range tests {
		result := maskString(test.input)
		assert.Equal(t, test.expected, result, "maskString(%q) = %q, expected %q", test.input, result, test.expected)
	}
}

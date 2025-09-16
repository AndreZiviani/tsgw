package main

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
)

type Config struct {
	Hostname        string
	OAuth           OAuthConfig
	OpenTelemetry   OpenTelemetryConfig
	Port            int
	LogLevel        string
	LogFormat       string
	SkipTLSVerify   bool
	TailscaleDomain string
	TsnetDir        string
	ForceCleanup    bool
	Routes          map[string]string // name -> backend URL

	// Timeouts and limits
	ConnectTimeout time.Duration
	RequestTimeout time.Duration
}

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	Issuer       string // OAuth issuer URL, defaults to Tailscale
}

type OpenTelemetryConfig struct {
	Enabled     bool
	ServiceName string
	Endpoint    string            // OTLP endpoint (e.g., "localhost:4317")
	Protocol    string            // "grpc" or "http"
	Insecure    bool              // Skip TLS verification for OTLP endpoint
	Headers     map[string]string // Additional headers for OTLP requests
}

// buildConfigFromCLI builds a Config struct directly from CLI flag values
func buildConfigFromCLI(cmd *cli.Command) *Config {
	config := &Config{
		Hostname:        cmd.String("hostname"),
		TailscaleDomain: cmd.String("tailscale-domain"),
		Port:            cmd.Int("port"),
		LogLevel:        cmd.String("log-level"),
		LogFormat:       cmd.String("log-format"),
		SkipTLSVerify:   cmd.Bool("skip-tls-verify"),
		TsnetDir:        cmd.String("tsnet-dir"),
		ForceCleanup:    cmd.Bool("force-cleanup"),

		OAuth: OAuthConfig{
			ClientID:     cmd.String("oauth-client-id"),
			ClientSecret: cmd.String("oauth-client-secret"),
			Issuer:       cmd.String("oauth-issuer"),
		},

		OpenTelemetry: OpenTelemetryConfig{
			Enabled:     cmd.Bool("otel-enabled"),
			ServiceName: cmd.String("otel-service-name"),
			Endpoint:    cmd.String("otel-endpoint"),
			Protocol:    cmd.String("otel-protocol"),
			Insecure:    cmd.Bool("otel-insecure"),
		},

		ConnectTimeout: 2 * time.Minute,  // 2 minutes for connection
		RequestTimeout: 30 * time.Second, // 30 seconds for requests
	}

	// Parse routes from CLI flags and environment variables
	routeFlags := cmd.StringSlice("route")
	config.Routes = make(map[string]string)

	// Parse routes from CLI flags
	for _, route := range routeFlags {
		parts := strings.SplitN(route, "=", 2)
		if len(parts) == 2 {
			config.Routes[parts[0]] = parts[1]
		}
	}

	// Parse routes from TSGW_ROUTE_* environment variables
	for _, env := range os.Environ() {
		if strings.HasPrefix(env, "TSGW_ROUTE_") {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				routeName := strings.TrimPrefix(parts[0], "TSGW_ROUTE_")
				routeName = strings.ToLower(routeName) // Convert to lowercase for consistency
				config.Routes[routeName] = parts[1]
			}
		}
	}

	return config
}

// SetupLogging configures the logging level and format from the loaded configuration
func SetupLogging(config *Config) {
	// Configure log format
	logFormat := config.LogFormat
	if logFormat == "" {
		logFormat = "console" // default to console format
	}

	switch strings.ToLower(logFormat) {
	case "text", "console":
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	case "json":
		// Reset to default JSON format
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	default:
		log.Warn().Str("log_format", logFormat).Msg("Invalid log format, using console")
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	// Configure log level
	logLevel := config.LogLevel
	if logLevel == "" {
		logLevel = "info"
	}

	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		log.Warn().Err(err).Str("log_level", logLevel).Msg("Invalid log level, using info")
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	log.Info().Str("level", level.String()).Str("format", logFormat).Msg("Logging configured")
}

// maskString safely masks a string by showing only the first 8 characters
func maskString(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8] + "..."
}

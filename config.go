package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Hostname        string              `yaml:"hostname"`
	OAuth           OAuthConfig         `yaml:"oauth"`
	OpenTelemetry   OpenTelemetryConfig `yaml:"opentelemetry"`
	Port            int                 `yaml:"port"`
	LogLevel        string              `yaml:"log_level"`
	LogFormat       string              `yaml:"log_format"`
	SkipTLSVerify   bool                `yaml:"skip_tls_verify"`
	ListenAddress   string              `yaml:"listen_address"`
	TailscaleDomain string              `yaml:"tailscale_domain"`
	TsnetDir        string              `yaml:"tsnet_dir"`
	Routes          map[string]string   `yaml:"routes"` // name -> backend URL

	// Timeouts and limits
	InitTimeout    time.Duration `yaml:"init_timeout"`
	ConnectTimeout time.Duration `yaml:"connect_timeout"`
	RequestTimeout time.Duration `yaml:"request_timeout"`
}

type OAuthConfig struct {
	ClientID     string `yaml:"client_id"`
	ClientSecret string `yaml:"client_secret"`
	Issuer       string `yaml:"issuer,omitempty"` // OAuth issuer URL, defaults to Tailscale
}

type OpenTelemetryConfig struct {
	Enabled     bool              `yaml:"enabled"`
	ServiceName string            `yaml:"service_name"`
	Endpoint    string            `yaml:"endpoint"` // OTLP endpoint (e.g., "localhost:4317")
	Protocol    string            `yaml:"protocol"` // "grpc" or "http"
	Insecure    bool              `yaml:"insecure"` // Skip TLS verification for OTLP endpoint
	Headers     map[string]string `yaml:"headers"`  // Additional headers for OTLP requests
}

// ConfigLoader handles loading configuration from multiple sources with priority
type ConfigLoader struct {
	cmd *cli.Command
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader(cmd *cli.Command) *ConfigLoader {
	return &ConfigLoader{cmd: cmd}
}

// Load loads configuration with the following priority (highest to lowest):
// 1. Environment variables
// 2. CLI flags
// 3. Configuration file
// 4. Default values
func (cl *ConfigLoader) Load() (*Config, error) {
	config := &Config{}

	// Load from config file (lowest priority)
	if err := cl.loadFromFile(config); err != nil {
		log.Debug().Err(err).Msg("Could not load config file, using defaults")
	}

	// Apply CLI flags (medium priority)
	cl.applyCLIFlags(config)

	// Apply environment variables (highest priority)
	cl.applyEnvironmentOverrides(config)

	// Set defaults for missing values
	cl.setDefaults(config)

	// Validate configuration
	if err := cl.validate(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// loadFromFile loads configuration from YAML file
func (cl *ConfigLoader) loadFromFile(config *Config) error {
	configFile := cl.cmd.String("config")
	if configFile == "" {
		configFile = "config.yaml"
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return fmt.Errorf("failed to parse YAML: %w", err)
	}

	log.Debug().Str("file", configFile).Int("routes", len(config.Routes)).Msg("Loaded configuration from file")
	return nil
}

// applyCLIFlags applies CLI flag values to the configuration
func (cl *ConfigLoader) applyCLIFlags(config *Config) {
	cmd := cl.cmd

	// Apply string flags
	if cmd.IsSet("hostname") {
		config.Hostname = cmd.String("hostname")
	}
	if cmd.IsSet("tailscale-domain") {
		config.TailscaleDomain = cmd.String("tailscale-domain")
	}
	if cmd.IsSet("oauth-client-id") {
		config.OAuth.ClientID = cmd.String("oauth-client-id")
	}
	if cmd.IsSet("oauth-client-secret") {
		config.OAuth.ClientSecret = cmd.String("oauth-client-secret")
	}
	if cmd.IsSet("oauth-issuer") {
		config.OAuth.Issuer = cmd.String("oauth-issuer")
	}
	if cmd.IsSet("log-level") {
		config.LogLevel = cmd.String("log-level")
	}
	if cmd.IsSet("log-format") {
		config.LogFormat = cmd.String("log-format")
	}
	if cmd.IsSet("listen-address") {
		config.ListenAddress = cmd.String("listen-address")
	}
	if cmd.IsSet("tsnet-dir") {
		config.TsnetDir = cmd.String("tsnet-dir")
	}
	if cmd.IsSet("otel-service-name") {
		config.OpenTelemetry.ServiceName = cmd.String("otel-service-name")
	}
	if cmd.IsSet("otel-endpoint") {
		config.OpenTelemetry.Endpoint = cmd.String("otel-endpoint")
	}
	if cmd.IsSet("otel-protocol") {
		config.OpenTelemetry.Protocol = cmd.String("otel-protocol")
	}

	// Apply int flags
	if cmd.IsSet("port") {
		config.Port = cmd.Int("port")
	}

	// Apply bool flags
	if cmd.IsSet("skip-tls-verify") {
		config.SkipTLSVerify = cmd.Bool("skip-tls-verify")
	}
	if cmd.IsSet("otel-enabled") {
		config.OpenTelemetry.Enabled = cmd.Bool("otel-enabled")
	}
	if cmd.IsSet("otel-insecure") {
		config.OpenTelemetry.Insecure = cmd.Bool("otel-insecure")
	}

	// Apply routes (special handling for string slice)
	if cmd.IsSet("route") {
		routes, err := cl.parseRoutes(cmd.StringSlice("route"))
		if err != nil {
			log.Warn().Err(err).Msg("Failed to parse some routes from CLI")
		}
		config.Routes = routes
	}
}

// applyEnvironmentOverrides applies environment variable overrides
func (cl *ConfigLoader) applyEnvironmentOverrides(config *Config) {
	// String environment variables
	cl.applyEnvString("TSGW_HOSTNAME", &config.Hostname)
	cl.applyEnvString("TSGW_TAILSCALE_DOMAIN", &config.TailscaleDomain)
	cl.applyEnvString("TSGW_OAUTH_CLIENT_ID", &config.OAuth.ClientID)
	cl.applyEnvString("TSGW_OAUTH_CLIENT_SECRET", &config.OAuth.ClientSecret)
	cl.applyEnvString("TSGW_OAUTH_ISSUER", &config.OAuth.Issuer)
	cl.applyEnvString("TSGW_LOG_LEVEL", &config.LogLevel)
	cl.applyEnvString("TSGW_LOG_FORMAT", &config.LogFormat)
	cl.applyEnvString("TSGW_LISTEN_ADDRESS", &config.ListenAddress)
	cl.applyEnvString("TSGW_TSNET_DIR", &config.TsnetDir)
	cl.applyEnvString("TSGW_OTEL_SERVICE_NAME", &config.OpenTelemetry.ServiceName)
	cl.applyEnvString("TSGW_OTEL_ENDPOINT", &config.OpenTelemetry.Endpoint)
	cl.applyEnvString("TSGW_OTEL_PROTOCOL", &config.OpenTelemetry.Protocol)

	// Int environment variables
	cl.applyEnvInt("TSGW_PORT", &config.Port)

	// Bool environment variables
	cl.applyEnvBool("TSGW_SKIP_TLS_VERIFY", &config.SkipTLSVerify)
	cl.applyEnvBool("TSGW_OTEL_ENABLED", &config.OpenTelemetry.Enabled)
	cl.applyEnvBool("TSGW_OTEL_INSECURE", &config.OpenTelemetry.Insecure)
}

// setDefaults sets default values for any missing configuration
func (cl *ConfigLoader) setDefaults(config *Config) {
	if config.Hostname == "" {
		config.Hostname = "tsgw"
	}
	if config.Port == 0 {
		config.Port = 443
	}
	if config.LogLevel == "" {
		config.LogLevel = "info"
	}
	if config.LogFormat == "" {
		config.LogFormat = "console"
	}
	if config.OAuth.Issuer == "" {
		config.OAuth.Issuer = "https://login.tailscale.com"
	}
	if config.TsnetDir == "" {
		config.TsnetDir = "./tsnet"
	}
	if config.Routes == nil {
		config.Routes = make(map[string]string)
	}

	// Set OpenTelemetry defaults
	if config.OpenTelemetry.ServiceName == "" {
		config.OpenTelemetry.ServiceName = "tsgw"
	}
	if config.OpenTelemetry.Protocol == "" {
		config.OpenTelemetry.Protocol = "grpc"
	}

	// Set default timeouts
	if config.InitTimeout == 0 {
		config.InitTimeout = 5 * time.Minute // 5 minutes for initialization
	}
	if config.ConnectTimeout == 0 {
		config.ConnectTimeout = 2 * time.Minute // 2 minutes for connection
	}
	if config.RequestTimeout == 0 {
		config.RequestTimeout = 30 * time.Second // 30 seconds for requests
	}
}

// validate checks the configuration for required values and consistency
func (cl *ConfigLoader) validate(config *Config) error {
	if config.OAuth.ClientID == "" {
		return fmt.Errorf("OAuth client ID is required (set via --oauth-client-id, TSGW_OAUTH_CLIENT_ID, or config file)")
	}
	if config.OAuth.ClientSecret == "" {
		return fmt.Errorf("OAuth client secret is required (set via --oauth-client-secret, TSGW_OAUTH_CLIENT_SECRET, or config file)")
	}
	if config.TailscaleDomain == "" {
		return fmt.Errorf("Tailscale domain is required (set via --tailscale-domain, TSGW_TAILSCALE_DOMAIN, or config file)")
	}
	if len(config.Routes) == 0 {
		return fmt.Errorf("at least one route is required (set via --route flags or config file)")
	}
	return nil
}

// Helper methods for environment variable application

func (cl *ConfigLoader) applyEnvString(envKey string, target *string) {
	if value := os.Getenv(envKey); value != "" {
		log.Debug().Str("key", envKey).Str("value", maskString(value)).Msg("Applied environment variable")
		*target = value
	}
}

func (cl *ConfigLoader) applyEnvInt(envKey string, target *int) {
	if value := os.Getenv(envKey); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			log.Debug().Str("key", envKey).Int("value", intValue).Msg("Applied environment variable")
			*target = intValue
		} else {
			log.Warn().Err(err).Str("key", envKey).Str("value", value).Msg("Invalid integer value for environment variable")
		}
	}
}

func (cl *ConfigLoader) applyEnvBool(envKey string, target *bool) {
	if value := os.Getenv(envKey); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			log.Debug().Str("key", envKey).Bool("value", boolValue).Msg("Applied environment variable")
			*target = boolValue
		} else {
			log.Warn().Err(err).Str("key", envKey).Str("value", value).Msg("Invalid boolean value for environment variable")
		}
	}
}

// parseRoutes parses route strings in format "name=backend_url"
func (cl *ConfigLoader) parseRoutes(routeStrings []string) (map[string]string, error) {
	routes := make(map[string]string)
	var errors []string

	for _, routeStr := range routeStrings {
		parts := strings.SplitN(routeStr, "=", 2)
		if len(parts) != 2 {
			errors = append(errors, fmt.Sprintf("invalid route format '%s', expected 'name=backend_url'", routeStr))
			continue
		}
		routes[parts[0]] = parts[1]
	}

	if len(errors) > 0 {
		return routes, fmt.Errorf("route parsing errors: %s", strings.Join(errors, "; "))
	}

	return routes, nil
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

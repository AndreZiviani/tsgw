package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2/clientcredentials"
	tailscale "tailscale.com/client/tailscale/v2"
	"tailscale.com/ipn"
	"tailscale.com/tsnet"
)

// createTailscaleClient creates and returns a Tailscale API client
func createTailscaleClient(ctx context.Context, config *Config) (*tailscale.Client, error) {
	log.Info().Str("client_id", maskString(config.OAuth.ClientID)).Msg("Creating Tailscale API client for auth key management")

	const tokenURLPath = "/api/v2/oauth/token"
	tokenURL := fmt.Sprintf("%s%s", ipn.DefaultControlURL, tokenURLPath)
	if config.OAuth.Issuer != "" {
		tokenURL = fmt.Sprintf("%s%s", config.OAuth.Issuer, tokenURLPath)
		log.Info().Str("issuer", config.OAuth.Issuer).Msg("Using custom OAuth issuer")
	}

	credentials := clientcredentials.Config{
		ClientID:     config.OAuth.ClientID,
		ClientSecret: config.OAuth.ClientSecret,
		TokenURL:     tokenURL,
	}

	// Create Tailscale API client
	tsClient := &tailscale.Client{
		Tailnet:   "-", // "-" indicates default tailnet
		HTTP:      credentials.Client(ctx),
		UserAgent: "tsgw",
	}
	if config.OAuth.Issuer != "" {
		baseURL, err := url.Parse(config.OAuth.Issuer)
		if err != nil {
			return nil, fmt.Errorf("failed to parse OAuth issuer URL: %w", err)
		}
		tsClient.BaseURL = baseURL
	}

	log.Info().Msg("Tailscale API client created successfully")
	return tsClient, nil
}

// tryReuseExistingState attempts to reuse existing Tailscale state for a route
func tryReuseExistingState(routeName, hostname, fqdn, backendURL, tsnetDir string) (*RouteServer, error) {
	stateFile := filepath.Join(tsnetDir, "tailscaled.state")
	if _, err := os.Stat(stateFile); err == nil {
		log.Debug().Str("route", routeName).Str("stateFile", stateFile).Str("backend", backendURL).Msg("Found existing Tailscale state, attempting to reuse")

		// Try to start without auth key first
		s := &tsnet.Server{
			Hostname: hostname,
			Dir:      tsnetDir,
			UserLogf: func(format string, args ...interface{}) {
				log.Debug().Str("route", routeName).Msgf(format, args...)
			},
			Logf: func(format string, args ...interface{}) {
				log.Trace().Str("route", routeName).Msgf(format, args...)
			},
		}

		if err := s.Start(); err == nil {
			log.Debug().Str("route", routeName).Msg("Successfully started Tailscale server with existing state")
			// Try to connect
			status, connectErr := s.Up(context.Background())
			if connectErr == nil {
				// Get Tailscale IP addresses
				ip4, ip6 := s.TailscaleIPs()
				log.Info().Str("route", routeName).Str("state", status.BackendState).Str("ip4", ip4.String()).Str("ip6", ip6.String()).Str("fqdn", fqdn).Msg("Tailscale server connected (reused)")
				routeServer := createRouteServer(routeName, hostname, fqdn, backendURL, s)
				return routeServer, nil // Successfully reused
			} else {
				log.Warn().Err(connectErr).Str("route", routeName).Msg("Failed to connect with existing state, will create new auth key")
				s.Close()
				return nil, connectErr
			}
		} else {
			log.Warn().Err(err).Str("route", routeName).Msg("Failed to start with existing state, will create new auth key")
			return nil, err
		}
	} else {
		log.Debug().Str("route", routeName).Str("stateFile", stateFile).Msg("No existing state file found")
	}
	return nil, nil // Could not reuse, but no error
}

// createNewAuthKey creates a new auth key for the given hostname
func createNewAuthKey(ctx context.Context, tsClient *tailscale.Client, routeName, hostname string) (string, error) {
	log.Info().Str("route", routeName).Msg("Creating auth key programmatically")

	caps := tailscale.KeyCapabilities{
		Devices: struct {
			Create struct {
				Reusable      bool     `json:"reusable"`
				Ephemeral     bool     `json:"ephemeral"`
				Tags          []string `json:"tags"`
				Preauthorized bool     `json:"preauthorized"`
			} `json:"create"`
		}{
			Create: struct {
				Reusable      bool     `json:"reusable"`
				Ephemeral     bool     `json:"ephemeral"`
				Tags          []string `json:"tags"`
				Preauthorized bool     `json:"preauthorized"`
			}{
				Reusable:      false,
				Preauthorized: true,
				Tags:          []string{"tag:tsgw"}, // Tag for our gateway nodes
			},
		},
	}

	// Sanitize description to only contain valid characters
	description := fmt.Sprintf("Auth key for TSGW route: %s", hostname)
	// Replace invalid characters with underscores, keep only alphanumeric, spaces, hyphens, and underscores
	sanitizedDesc := ""
	for _, r := range description {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' || r == '-' || r == '_' {
			sanitizedDesc += string(r)
		} else {
			sanitizedDesc += "_"
		}
	}

	request := tailscale.CreateKeyRequest{
		Capabilities: caps,
		Description:  sanitizedDesc,
	}

	key, err := tsClient.Keys().CreateAuthKey(ctx, request)
	if err != nil {
		log.Error().Err(err).Str("route", routeName).Msg("Failed to create auth key programmatically")
		return "", err
	}
	log.Info().Str("route", routeName).Msg("Auth key created successfully")
	return key.Key, nil
}

// startAndConnectServer starts a tsnet server and connects it to Tailscale
func startAndConnectServer(ctx context.Context, routeName, hostname, fqdn, tsnetDir, authKey string, connectTimeout time.Duration, otel *OpenTelemetry) (*tsnet.Server, error) {
	ctx, span := otel.Tracer.Start(ctx, "startAndConnectServer",
		trace.WithAttributes(
			attribute.String("route.name", routeName),
			attribute.String("route.hostname", hostname),
			attribute.String("route.fqdn", fqdn),
		))
	defer span.End()

	// Create tsnet server for this route
	s := &tsnet.Server{
		Hostname: hostname,
		AuthKey:  authKey,
		Dir:      tsnetDir,
	}

	log.Debug().Str("route", routeName).Str("hostname", hostname).Str("fqdn", fqdn).Str("tsnetDir", tsnetDir).Msg("Creating Tailscale server for route")

	// Start Tailscale server
	log.Info().Str("route", routeName).Msg("Starting Tailscale server")
	if err := s.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Tailscale server for route %s: %w", routeName, err)
	}

	// Ensure server is closed if we return early due to context cancellation or error
	defer func() {
		if ctx.Err() != nil {
			log.Info().Str("route", routeName).Msg("Context cancelled, closing Tailscale server")
			s.Close()
		}
	}()

	// Wait for connection with timeout
	log.Info().Str("route", routeName).Msg("Connecting Tailscale server to network")
	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	status, err := s.Up(connectCtx)
	if err != nil {
		// Close the server since connection failed
		s.Close()
		// Provide more helpful error message for OAuth permission issues
		if strings.Contains(err.Error(), "key cannot be used for node auth") {
			return nil, fmt.Errorf("OAuth client does not have permission to authenticate nodes for route %s. Please ensure the OAuth client has 'Device' scope enabled in the Tailscale admin console: %w", routeName, err)
		}
		return nil, fmt.Errorf("failed to connect Tailscale server for route %s: %w", routeName, err)
	}

	// Get Tailscale IP addresses
	ip4, ip6 := s.TailscaleIPs()
	log.Info().Str("route", routeName).Str("state", status.BackendState).Str("ip4", ip4.String()).Str("ip6", ip6.String()).Str("fqdn", fqdn).Msg("Tailscale server connected")

	return s, nil
}

// createRouteServer creates a RouteServer struct
func createRouteServer(routeName, hostname, fqdn, backendURL string, server *tsnet.Server) *RouteServer {
	return &RouteServer{
		RouteName: routeName,
		Hostname:  hostname,
		Server:    server,
		Backend:   backendURL,
		FQDN:      fqdn,
	}
}

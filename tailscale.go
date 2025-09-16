package main

import (
	"context"
	"fmt"
	"net/url"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2/clientcredentials"
	"golang.org/x/sync/errgroup"
	"tailscale.com/client/tailscale/v2"
	"tailscale.com/ipn"
)

func (s *server) Start(ctx context.Context) error {
	ctx, span := s.otel.Tracer.Start(ctx, "setupAndStartServers")
	defer span.End()

	log.Info().Int("routes", len(s.config.Routes)).Msg("Starting parallel initialization of Tailscale servers")

	// Create errgroup for managing all goroutines
	g, gctx := errgroup.WithContext(ctx)

	// Start independent goroutines for each route
	for routeName, backendURL := range s.config.Routes {
		routeName, backendURL := routeName, backendURL // capture loop variables
		g.Go(func() error {
			return s.startRoute(gctx, routeName, backendURL)
		})
	}

	// Wait for all goroutines to complete
	return g.Wait()
}

// startRoute handles the complete lifecycle of a single route:
// 1. Initializes the route server using shared Tailscale client
// 2. Creates a dedicated Echo instance for this route
// 3. Starts the HTTP server
// 4. Listens for shutdown signals
func (s *server) startRoute(ctx context.Context, routeName, backendURL string) error {
	ctx, span := s.otel.Tracer.Start(ctx, "startRoute",
		trace.WithAttributes(
			attribute.String("route.name", routeName),
			attribute.String("route.backend", backendURL),
		))
	defer span.End()

	log.Info().Str("route", routeName).Str("backend", backendURL).Msg("Starting route")

	fqdn := routeName + "." + s.config.TailscaleDomain

	tsServer, err := s.startTailscaleInstance(ctx, routeName)
	if err != nil {
		return fmt.Errorf("failed to start Tailscale instance for %s: %w", routeName, err)
	}

	// Get Tailscale IP addresses
	ip4, ip6 := tsServer.TailscaleIPs()
	log.Info().Str("route", routeName).Str("ip4", ip4.String()).Str("ip6", ip6.String()).Str("fqdn", fqdn).Msg("Tailscale server connected")

	routeServer, err := NewRouteServer(routeName, tsServer, backendURL, s.config, s.otel)
	if err != nil {
		return fmt.Errorf("failed to create route server for %s: %w", routeName, err)
	}

	// Start the HTTP server for this route
	return routeServer.Start(ctx)
}

// createTailscaleClient creates and returns a Tailscale API client
func createTailscaleClient(ctx context.Context, config *Config) (*tailscale.Client, error) {
	log.Info().Str("client_id", maskString(config.OAuth.ClientID)).Msg("Creating Tailscale API client for auth key management")

	const tokenURLPath = "/api/v2/oauth/token"
	tokenURL := fmt.Sprintf("%s%s", ipn.DefaultControlURL, tokenURLPath)
	baseURL, err := url.Parse("https://api.tailscale.com")
	if config.OAuth.Issuer != "" {
		tokenURL = fmt.Sprintf("%s%s", config.OAuth.Issuer, tokenURLPath)
		baseURL, err = url.Parse(config.OAuth.Issuer)
		if err != nil {
			return nil, fmt.Errorf("failed to parse OAuth issuer URL: %w", err)
		}
		log.Info().Str("issuer", config.OAuth.Issuer).Msg("Using custom OAuth issuer")
	}

	credentials := clientcredentials.Config{
		ClientID:     config.OAuth.ClientID,
		ClientSecret: config.OAuth.ClientSecret,
		TokenURL:     tokenURL,
	}

	// Create Tailscale API client
	tsClient := &tailscale.Client{
		BaseURL:   baseURL,
		Tailnet:   "-", // "-" indicates default tailnet
		HTTP:      credentials.Client(ctx),
		UserAgent: "tsgw",
	}

	log.Info().Msg("Tailscale API client created successfully")
	return tsClient, nil
}

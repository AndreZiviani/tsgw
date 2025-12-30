package main

import (
	"context"
	"fmt"
	"net/url"

	"github.com/rs/zerolog/log"
	"golang.org/x/oauth2/clientcredentials"
	"tailscale.com/client/tailscale/v2"
	"tailscale.com/ipn"
)

// createTailscaleClient creates and returns a Tailscale API client.
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

	tsClient := &tailscale.Client{
		BaseURL:   baseURL,
		Tailnet:   "-", // "-" indicates default tailnet
		HTTP:      credentials.Client(ctx),
		UserAgent: "tsgw",
	}

	log.Info().Msg("Tailscale API client created successfully")
	return tsClient, nil
}

package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"tailscale.com/client/tailscale/v2"
)

func createNewAuthKey(ctx context.Context, tsClient *tailscale.Client, tsTag string, routeName string) (string, error) {
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
				Tags:          []string{tsTag},
			},
		},
	}

	description := fmt.Sprintf("Auth key for TSGW route: %s", routeName)
	var b strings.Builder
	b.Grow(len(description))
	for _, r := range description {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	sanitizedDesc := b.String()

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

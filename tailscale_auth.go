package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"tailscale.com/client/tailscale/v2"
	"tailscale.com/ipn"
	"tailscale.com/tsnet"
)

func (s *server) startTailscaleInstance(ctx context.Context, routeName string) (*tsnet.Server, error) {
	ctx, span := s.otel.Tracer.Start(ctx, "startTailscaleInstance",
		trace.WithAttributes(
			attribute.String("route.name", routeName),
			attribute.String("route.fqdn", routeName+"."+s.config.TailscaleDomain),
		))
	defer span.End()

	// Use configurable tsnet directory with route-specific subdirectory
	tsnetDir := filepath.Join(s.config.TsnetDir, routeName)
	if s.config.ForceCleanup {
		log.Warn().Str("route", routeName).Str("dir", tsnetDir).Msg("Force cleanup enabled; removing tsnet state directory")
		if err := os.RemoveAll(tsnetDir); err != nil {
			return nil, fmt.Errorf("failed to remove tsnet dir %s: %w", tsnetDir, err)
		}
	}

	// Try to start without auth key first
	tsServer := &tsnet.Server{
		Hostname: routeName,
		Dir:      tsnetDir,
		UserLogf: func(format string, args ...interface{}) {
			log.Debug().Str("route", routeName).Msgf(format, args...)
		},
		Logf: func(format string, args ...interface{}) {
			log.Trace().Str("route", routeName).Msgf(format, args...)
		},
	}

	if err := tsServer.Start(); err != nil {
		log.Warn().Err(err).Str("route", routeName).Msg("Failed to start with existing state")
		return nil, err
	}

	log.Debug().Str("route", routeName).Msg("Successfully started Tailscale server with existing state")
	lc, err := tsServer.LocalClient()
	if err != nil {
		log.Warn().Err(err).Str("route", routeName).Msg("Failed to create local client for existing state")
		tsServer.Close()
		//TODO: cleanup state files?
		return nil, err
	}

	loginDone := false
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
waitOnline:
	for {
		st, err := lc.StatusWithoutPeers(ctx)
		if err != nil {
			log.Warn().Err(err).Str("route", routeName).Msg("Failed to get status from local client")
			tsServer.Close()
			return nil, err
		}

		switch st.BackendState {
		case "Running":
			log.Debug().Str("route", routeName).Msg("Tailscale server is already running")
			break waitOnline
		case "NeedsLogin":
			if loginDone {
				break
			}

			key, err := createNewAuthKey(ctx, s.tsClient, s.config.TailscaleTag, routeName)
			if err != nil {
				tsServer.Close()
				return nil, err
			}

			log.Info().Str("route", routeName).Msg("Logging in with new auth key")
			if err := lc.Start(ctx, ipn.Options{AuthKey: key}); err != nil {
				log.Warn().Err(err).Str("route", routeName).Msg("Failed to authenticate with new auth key")
				tsServer.Close()
				return nil, err
			}

			if err := lc.StartLoginInteractive(ctx); err != nil {
				log.Warn().Err(err).Str("route", routeName).Msg("Failed to start interactive login")
				tsServer.Close()
				return nil, err
			}
			loginDone = true
		}
		select {
		case <-ctx.Done():
			tsServer.Close()
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}

	// Try to connect. Use ctx so shutdown can interrupt this.
	upCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, connectErr := tsServer.Up(upCtx)
	if connectErr != nil {
		log.Warn().Err(connectErr).Str("route", routeName).Msg("Failed to connect")
		tsServer.Close()
		return nil, connectErr
	}

	return tsServer, nil
}

// createNewAuthKey creates a new auth key for the given hostname
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
				Tags:          []string{tsTag}, // Tag for our gateway nodes
			},
		},
	}

	// Sanitize description to only contain valid characters
	description := fmt.Sprintf("Auth key for TSGW route: %s", routeName)
	// Replace invalid characters with underscores, keep only alphanumeric, spaces, hyphens, and underscores
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

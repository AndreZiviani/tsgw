package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"tailscale.com/ipn"
	"tailscale.com/tsnet"
)

func (s *server) startTailscaleServiceHost(ctx context.Context) (*tsnet.Server, error) {
	ctx, span := s.otel.Tracer.Start(ctx, "startTailscaleInstance",
		trace.WithAttributes(
			attribute.String("host.name", "tsgw"),
		))
	defer span.End()

	tsnetDir := s.config.TsnetDir
	if s.config.ForceCleanup {
		log.Warn().Str("dir", tsnetDir).Msg("Force cleanup enabled; removing tsnet state directory")
		if err := os.RemoveAll(tsnetDir); err != nil {
			return nil, fmt.Errorf("failed to remove tsnet dir %s: %w", tsnetDir, err)
		}
	}

	tsServer := &tsnet.Server{Hostname: "tsgw", Dir: tsnetDir}
	tsServer.UserLogf = func(format string, args ...interface{}) {
		log.Debug().Msgf(format, args...)
	}
	tsServer.Logf = func(format string, args ...interface{}) {
		log.Trace().Msgf(format, args...)
	}

	if err := tsServer.Start(); err != nil {
		log.Warn().Err(err).Msg("Failed to start with existing state")
		return nil, err
	}

	log.Debug().Str("host", "tsgw").Msg("Successfully started Tailscale server with existing state")
	lc, err := tsServer.LocalClient()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to create local client")
		tsServer.Close()
		return nil, err
	}

	loginDone := false
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
waitOnline:
	for {
		st, err := lc.StatusWithoutPeers(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get status from local client")
			tsServer.Close()
			return nil, err
		}

		switch st.BackendState {
		case "Running":
			log.Debug().Msg("Tailscale server is already running")
			break waitOnline
		case "NeedsLogin":
			if loginDone {
				break
			}

			key, err := createNewAuthKey(ctx, s.tsClient, s.config.TailscaleTag, "tsgw")
			if err != nil {
				tsServer.Close()
				return nil, err
			}

			log.Info().Msg("Logging in with new auth key")
			if err := lc.Start(ctx, ipn.Options{AuthKey: key}); err != nil {
				log.Warn().Err(err).Msg("Failed to authenticate with new auth key")
				tsServer.Close()
				return nil, err
			}

			if err := lc.StartLoginInteractive(ctx); err != nil {
				log.Warn().Err(err).Msg("Failed to start interactive login")
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

	upCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, connectErr := tsServer.Up(upCtx)
	if connectErr != nil {
		log.Warn().Err(connectErr).Msg("Failed to connect")
		tsServer.Close()
		return nil, connectErr
	}

	return tsServer, nil
}

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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

	// Best-effort: try to create an auth key for automatic registration. If
	// this fails, continue and let tsnet handle interactive login or use any
	// existing state.
	// Reuse a single in-memory auth key for all services started in this
	// process. Create it once (best-effort) and keep it on `s.tsAuthKey`.
	if s.tsAuthKey == "" {
		if key, err := createNewAuthKey(ctx, s.tsClient, s.config.TailscaleTag, "tsgw"); err == nil {
			s.tsAuthKey = key
			tsServer.AuthKey = key
			log.Info().Msg("Using freshly created Tailscale auth key")
		} else {
			log.Debug().Err(err).Msg("could not create auth key; will rely on tsnet interactive login or existing state")
		}
	} else {
		tsServer.AuthKey = s.tsAuthKey
	}

	// Let tsnet start and wait until the backend is Running. tsnet.Up takes
	// care of starting the local backend, handling auth keys and interactive
	// login when needed, and returns the status including Tailscale IPs.
	upCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := tsServer.Up(upCtx); err != nil {
		log.Warn().Err(err).Msg("Failed to bring tsnet server up")
		tsServer.Close()
		return nil, err
	}

	log.Debug().Msg("Tailscale server is up and running")
	return tsServer, nil
}

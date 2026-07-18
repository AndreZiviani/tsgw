package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"tailscale.com/tsnet"
)

func (s *server) Start(ctx context.Context) error {
	ctx, span := s.otel.Tracer.Start(ctx, "setupAndStartServers")
	defer span.End()

	if err := ctx.Err(); err != nil {
		return err
	}

	log.Info().Int("routes", len(s.config.Routes)).Msg("Starting Tailscale service host")

	tsServer, err := s.startTailscaleServiceHost(ctx)
	if err != nil {
		return fmt.Errorf("start tailscale service host: %w", err)
	}
	defer tsServer.Close()

	// errgroup and listener collection for idiomatic lifecycle management
	g, gctx := errgroup.WithContext(ctx)
	listeners := make([]net.Listener, 0, len(s.config.Routes)*2)

	for routeName, backend := range s.config.Routes {
		svcName := "svc:" + routeName

		// If HTTP port is configured, serve a redirect handler that points to HTTPS
		if s.config.HTTPPort != 0 {
			ln, err := tsServer.ListenService(svcName, tsnet.ServiceModeHTTP{Port: uint16(s.config.HTTPPort), HTTPS: false})
			if err != nil {
				for _, l := range listeners {
					_ = l.Close()
				}
				return fmt.Errorf("ListenService (http) for %s: %w", svcName, err)
			}
			listeners = append(listeners, ln)

			g.Go(func() error {
				log.Info().Str("service", svcName).Str("route", routeName).Msg("Starting HTTP redirect listener")
				err := http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					host := r.Host
					if host == "" {
						host = routeName
					}
					location := "https://" + host + r.URL.RequestURI()
					w.Header().Set("Location", location)
					w.WriteHeader(http.StatusPermanentRedirect)
				}))
				if err == nil || errors.Is(err, net.ErrClosed) {
					return nil
				}
				return err
			})
		}

		// If HTTPS port is configured, serve the reverse proxy directly
		if s.config.HTTPSPort != 0 {
			ln, err := tsServer.ListenService(svcName, tsnet.ServiceModeHTTP{Port: uint16(s.config.HTTPSPort), HTTPS: true})
			if err != nil {
				for _, l := range listeners {
					_ = l.Close()
				}
				return fmt.Errorf("ListenService (https) for %s: %w", svcName, err)
			}
			listeners = append(listeners, ln)

			proxy, err := NewRouteProxy(routeName, backend, s.config)
			if err != nil {
				_ = ln.Close()
				for _, l := range listeners {
					_ = l.Close()
				}
				return fmt.Errorf("create proxy for %s: %w", routeName, err)
			}

			g.Go(func() error {
				log.Info().Str("service", svcName).Str("route", routeName).Msg("Starting HTTPS proxy listener")
				err := http.Serve(ln, proxy)
				if err == nil || errors.Is(err, net.ErrClosed) {
					return nil
				}
				return err
			})
		}
	}

	// Close listeners when group context is done (on cancel or error)
	go func() {
		<-gctx.Done()
		for _, l := range listeners {
			_ = l.Close()
		}
	}()

	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}

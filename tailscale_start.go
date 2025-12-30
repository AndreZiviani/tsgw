package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
	"tailscale.com/tailcfg"
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

	lc, err := tsServer.LocalClient()
	if err != nil {
		return fmt.Errorf("local client: %w", err)
	}

	magicSuffix, err := s.magicDNSSuffix(ctx, lc)
	if err != nil {
		return err
	}

	redirectLn, redirectSrv, redirectURL, err := newRedirectServer()
	if err != nil {
		return err
	}

	runtimes, routePorts, serviceNames, err := buildRouteRuntimes(s.config)
	if err != nil {
		_ = redirectLn.Close()
		_ = redirectSrv.Close()
		return err
	}

	errCh := startLocalServers(ctx, redirectLn, redirectSrv, runtimes)

	if err := applyTailscaleServeConfig(ctx, lc, serviceNames, routePorts, magicSuffix, redirectURL, uint16(s.config.HTTPPort), uint16(s.config.HTTPSPort)); err != nil {
		return err
	}

	for _, rt := range runtimes {
		fqdn := rt.name + "." + magicSuffix
		log.Info().
			Str("service", rt.svc.String()).
			Str("fqdn", fqdn).
			Uint16("http-port", uint16(s.config.HTTPPort)).
			Uint16("https-port", uint16(s.config.HTTPSPort)).
			Str("backend", s.config.Routes[rt.name]).
			Msg("Service configured")
	}

	select {
	case <-ctx.Done():
		// Proceed to shutdown.
	case err := <-errCh:
		return err
	}

	log.Info().Msg("Shutdown requested; stopping")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	bestEffortCleanupServeConfig(shutdownCtx, lc, serviceNames)
	shutdownLocalServers(shutdownCtx, redirectSrv, runtimes)

	select {
	case err := <-errCh:
		return err
	case <-shutdownCtx.Done():
		return shutdownCtx.Err()
	}
}

type routeRuntime struct {
	name string
	ln   net.Listener
	srv  *http.Server
	port int
	svc  tailcfg.ServiceName
}

func (s *server) magicDNSSuffix(ctx context.Context, lc localClient) (string, error) {
	st, err := lc.StatusWithoutPeers(ctx)
	if err != nil {
		return "", fmt.Errorf("status: %w", err)
	}

	magicSuffix := ""
	if st.CurrentTailnet != nil {
		magicSuffix = st.CurrentTailnet.MagicDNSSuffix
	}

	configuredDomain := strings.TrimSpace(s.config.TailscaleDomain)
	configuredDomain = strings.TrimPrefix(configuredDomain, ".")
	if configuredDomain != "" && magicSuffix != "" && configuredDomain != magicSuffix {
		log.Warn().
			Str("configured", configuredDomain).
			Str("magic_dns_suffix", magicSuffix).
			Msg("Configured tailscale-domain does not match MagicDNSSuffix")
	}

	if magicSuffix == "" {
		return "", fmt.Errorf("tailscale MagicDNSSuffix is empty; cannot configure services (is MagicDNS enabled and is this node fully connected?)")
	}

	return magicSuffix, nil
}

func newRedirectServer() (net.Listener, *http.Server, string, error) {
	redirectLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, "", fmt.Errorf("listen redirect server: %w", err)
	}
	redirectPort := redirectLn.Addr().(*net.TCPAddr).Port
	redirectURL := fmt.Sprintf("http://127.0.0.1:%d", redirectPort)

	redirectSrv := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       30 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := strings.TrimSpace(r.Host)
			if host == "" {
				host = "localhost"
			}
			location := "https://" + host + r.URL.RequestURI()
			w.Header().Set("Location", location)
			w.WriteHeader(http.StatusPermanentRedirect)
		}),
	}

	return redirectLn, redirectSrv, redirectURL, nil
}

func buildRouteRuntimes(cfg *Config) ([]*routeRuntime, map[string]int, []tailcfg.ServiceName, error) {
	runtimes := make([]*routeRuntime, 0, len(cfg.Routes))
	routePorts := make(map[string]int, len(cfg.Routes))
	serviceNames := make([]tailcfg.ServiceName, 0, len(cfg.Routes))

	for routeName, backendURL := range cfg.Routes {
		proxy, err := NewRouteProxy(routeName, backendURL, cfg)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("route %s: create proxy: %w", routeName, err)
		}

		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, nil, nil, fmt.Errorf("route %s: listen localhost: %w", routeName, err)
		}
		tcpAddr, ok := ln.Addr().(*net.TCPAddr)
		if !ok {
			_ = ln.Close()
			return nil, nil, nil, fmt.Errorf("route %s: unexpected listener addr type %T", routeName, ln.Addr())
		}
		port := tcpAddr.Port

		srv := &http.Server{
			Handler:           proxy,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       2 * time.Minute,
		}

		rt := &routeRuntime{
			name: routeName,
			ln:   ln,
			srv:  srv,
			port: port,
			svc:  serviceNameForRoute(routeName),
		}
		runtimes = append(runtimes, rt)
		routePorts[routeName] = port
		serviceNames = append(serviceNames, rt.svc)
	}

	sort.Slice(serviceNames, func(i, j int) bool { return serviceNames[i] < serviceNames[j] })

	return runtimes, routePorts, serviceNames, nil
}

func startLocalServers(ctx context.Context, redirectLn net.Listener, redirectSrv *http.Server, runtimes []*routeRuntime) <-chan error {
	g, _ := errgroup.WithContext(ctx)

	g.Go(func() error {
		err := redirectSrv.Serve(redirectLn)
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	})

	for _, rt := range runtimes {
		rt := rt
		g.Go(func() error {
			err := rt.srv.Serve(rt.ln)
			if err == http.ErrServerClosed {
				return nil
			}
			return err
		})
	}

	errCh := make(chan error, 1)
	go func() { errCh <- g.Wait() }()
	return errCh
}

func applyTailscaleServeConfig(
	ctx context.Context,
	lc localClient,
	serviceNames []tailcfg.ServiceName,
	routePorts map[string]int,
	magicSuffix string,
	redirectURL string,
	httpPort, httpsPort uint16,
) error {
	if err := ensureAdvertiseServices(ctx, lc, serviceNames); err != nil {
		return err
	}

	newSC := buildServicesServeConfig(routePorts, magicSuffix, redirectURL, httpPort, httpsPort)
	if cur, err := lc.GetServeConfig(ctx); err == nil && cur != nil {
		newSC.ETag = cur.ETag
	}
	if err := lc.SetServeConfig(ctx, newSC); err != nil {
		return fmt.Errorf("set serve config: %w", err)
	}

	return nil
}

func bestEffortCleanupServeConfig(ctx context.Context, lc localClient, serviceNames []tailcfg.ServiceName) {
	_ = removeAdvertiseServices(ctx, lc, serviceNames)

	cur, err := lc.GetServeConfig(ctx)
	if err != nil || cur == nil {
		return
	}

	changed := false
	for _, sn := range serviceNames {
		if cur.Services != nil {
			if _, ok := cur.Services[sn]; ok {
				delete(cur.Services, sn)
				changed = true
			}
		}
	}
	if changed {
		_ = lc.SetServeConfig(ctx, cur)
	}
}

func shutdownLocalServers(ctx context.Context, redirectSrv *http.Server, runtimes []*routeRuntime) {
	for _, rt := range runtimes {
		rt.srv.SetKeepAlivesEnabled(false)
		_ = rt.srv.Shutdown(ctx)
		_ = rt.srv.Close()
	}

	redirectSrv.SetKeepAlivesEnabled(false)
	_ = redirectSrv.Shutdown(ctx)
	_ = redirectSrv.Close()
}

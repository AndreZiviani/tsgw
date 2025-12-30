package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
)

type RouteProxy struct {
	routeName      string
	backendURL     string
	targetURL      *url.URL
	proxy          *httputil.ReverseProxy
	requestTimeout time.Duration
}

func NewRouteProxy(routeName, backendURL string, cfg *Config) (*RouteProxy, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}

	target, err := url.Parse(backendURL)
	if err != nil {
		return nil, fmt.Errorf("parse backend URL: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	baseDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		baseDirector(r)
		// Many backends (virtual hosts, CDNs, ingress controllers) route based on
		// the Host header. Default ReverseProxy preserves the incoming Host, which
		// in our case is the Tailscale service FQDN, not the backend host.
		r.Host = target.Host
	}
	proxy.Transport = newProxyTransport(cfg, target)
	proxy.BufferPool = newProxyBufferPool(32 * 1024)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Warn().
			Err(err).
			Str("route", routeName).
			Str("backend", target.String()).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Msg("Proxy error")
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
	}

	return &RouteProxy{
		routeName:      routeName,
		backendURL:     backendURL,
		targetURL:      target,
		proxy:          proxy,
		requestTimeout: cfg.RequestTimeout,
	}, nil
}

func (rp *RouteProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &responseRecorder{w: w}

	if rp.requestTimeout > 0 {
		ctx, cancel := context.WithTimeout(r.Context(), rp.requestTimeout)
		defer cancel()
		r = r.WithContext(ctx)
	}

	rp.proxy.ServeHTTP(rec, r)

	dur := time.Since(start)
	status := rec.statusCode
	if status == 0 {
		status = http.StatusOK
	}

	// Avoid logging full query strings by default; they may contain secrets.
	log.Info().
		Str("route", rp.routeName).
		Str("backend", rp.backendURL).
		Str("method", r.Method).
		Str("host", r.Host).
		Str("path", r.URL.Path).
		Int("status", status).
		Int64("bytes", rec.bytes).
		Dur("duration", dur).
		Str("remote", r.RemoteAddr).
		Msg("request")
}

func (rp *RouteProxy) LocalTarget() *url.URL {
	return rp.targetURL
}

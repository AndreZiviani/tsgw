package main

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type proxyBufferPool struct {
	size int
	pool sync.Pool
}

func newProxyBufferPool(size int) *proxyBufferPool {
	bp := &proxyBufferPool{size: size}
	bp.pool.New = func() any {
		return make([]byte, size)
	}
	return bp
}

func (bp *proxyBufferPool) Get() []byte {
	return bp.pool.Get().([]byte)
}

func (bp *proxyBufferPool) Put(b []byte) {
	if bp == nil {
		return
	}
	if cap(b) < bp.size {
		return
	}
	bp.pool.Put(b[:bp.size])
}

func newProxyTransport(cfg *Config, target *url.URL) http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return http.DefaultTransport
	}

	tr := base.Clone()
	tr.MaxIdleConns = 256
	tr.MaxIdleConnsPerHost = 64
	tr.IdleConnTimeout = 90 * time.Second
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ExpectContinueTimeout = 1 * time.Second
	tr.ResponseHeaderTimeout = 30 * time.Second
	tr.ForceAttemptHTTP2 = true

	dialTimeout := 30 * time.Second
	if cfg != nil && cfg.ConnectTimeout > 0 {
		dialTimeout = cfg.ConnectTimeout
	}
	tr.DialContext = (&net.Dialer{Timeout: dialTimeout, KeepAlive: 30 * time.Second}).DialContext

	if cfg != nil && target != nil && target.Scheme == "https" {
		var tlsCfg *tls.Config
		if tr.TLSClientConfig != nil {
			tlsCfg = tr.TLSClientConfig.Clone()
		} else {
			tlsCfg = &tls.Config{}
		}
		tlsCfg.InsecureSkipVerify = cfg.SkipTLSVerify
		tr.TLSClientConfig = tlsCfg
	}

	return tr
}

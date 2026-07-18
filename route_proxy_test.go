package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewRouteProxy(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Backend", "ok")
		w.Header().Set("X-Seen-Host", r.Host)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer backend.Close()

	backendURL, err := url.Parse(backend.URL)
	assert.NoError(t, err)

	cfg := &Config{RequestTimeout: 2 * time.Second}

	rp, err := NewRouteProxy("app", backend.URL, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, rp)

	req := httptest.NewRequest(http.MethodGet, "http://example.test/test", nil)
	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "ok", rec.Header().Get("X-Backend"))
	assert.Equal(t, backendURL.Host, rec.Header().Get("X-Seen-Host"))
}

func TestNewRouteProxy_InvalidBackend(t *testing.T) {
	cfg := &Config{}
	rp, err := NewRouteProxy("app", "http://[::1:80/", cfg)
	assert.Error(t, err)
	assert.Nil(t, rp)
}

func TestNewProxyTransport_ResponseHeaderTimeout(t *testing.T) {
	tests := []struct {
		name           string
		requestTimeout time.Duration
		wantHeaderTO   time.Duration
	}{
		{
			name:           "request-timeout=0 disables ResponseHeaderTimeout",
			requestTimeout: 0,
			wantHeaderTO:   0,
		},
		{
			name:           "custom request-timeout is honored",
			requestTimeout: 5 * time.Minute,
			wantHeaderTO:   5 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := url.Parse("http://app.internal:8080")
			assert.NoError(t, err)

			rt := newProxyTransport(&Config{RequestTimeout: tt.requestTimeout}, target)
			tr, ok := rt.(*http.Transport)
			assert.True(t, ok, "expected *http.Transport")
			assert.Equal(t, tt.wantHeaderTO, tr.ResponseHeaderTimeout)
		})
	}
}

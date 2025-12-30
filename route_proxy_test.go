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

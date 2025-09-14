package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
)

func TestProxyRequest(t *testing.T) {
	// Create a test backend server
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from backend"))
	}))
	defer backend.Close()

	// Create Echo context
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Request().Host = "test.example.com"

	// Test proxy
	err := proxyRequest(c, backend.URL, false, 30*time.Second)
	assert.NoError(t, err)

	assert.Equal(t, http.StatusOK, rec.Code)

	expected := "Hello from backend"
	assert.Equal(t, expected, rec.Body.String())
}

func TestProxyRequestInvalidURL(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Use a URL that will definitely fail to parse
	err := proxyRequest(c, "http://[::1:80/", false, 30*time.Second) // Invalid IPv6 URL
	assert.Error(t, err, "Expected error for invalid URL, but got none")
}

func TestRoutingWithConfig(t *testing.T) {
	// Create test backend servers
	backend1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Response from backend1"))
	}))
	defer backend1.Close()

	backend2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Response from backend2"))
	}))
	defer backend2.Close()

	// Create route servers
	routeServers := []*RouteServer{
		{
			RouteName: "app1",
			Hostname:  "test-app1",
			Backend:   backend1.URL,
			FQDN:      "app1.example.com",
		},
		{
			RouteName: "app2",
			Hostname:  "test-app2",
			Backend:   backend2.URL,
			FQDN:      "app2.example.com",
		},
	}

	// Test routing logic
	testCases := []struct {
		host     string
		expected string
		found    bool
	}{
		{"app1.example.com", "Response from backend1", true},
		{"app2.example.com", "Response from backend2", true},
		{"unknown.example.com", "", false},
	}

	for _, tc := range testCases {
		t.Run(tc.host, func(t *testing.T) {
			// Find the route server for this host
			var routeServer *RouteServer
			for _, rs := range routeServers {
				if rs.FQDN == tc.host {
					routeServer = rs
					break
				}
			}

			found := routeServer != nil
			assert.Equal(t, tc.found, found, "Expected route found=%v for host %s, got found=%v", tc.found, tc.host, found)

			if found {
				// Test the actual proxy request
				e := echo.New()
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)
				c.Request().Host = tc.host

				err := proxyRequest(c, routeServer.Backend, false, 30*time.Second)
				assert.NoError(t, err)

				assert.Contains(t, rec.Body.String(), tc.expected, "Expected response to contain '%s', got '%s'", tc.expected, rec.Body.String())
			}
		})
	}
}

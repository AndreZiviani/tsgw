package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildServicesServeConfig(t *testing.T) {
	ports := map[string]int{"app": 12345, "api": 23456}
	sc := buildServicesServeConfig(ports, "test.ts.net", "http://127.0.0.1:9999", 80, 443)

	assert.NotNil(t, sc)
	assert.Len(t, sc.Services, 2)

	appSvc := sc.Services[serviceNameForRoute("app")]
	assert.NotNil(t, appSvc)
	assert.NotNil(t, appSvc.TCP[80])
	assert.True(t, appSvc.TCP[80].HTTP)
	assert.NotNil(t, appSvc.TCP[443])
	assert.True(t, appSvc.TCP[443].HTTPS)

	// HostPort key includes route + magic suffix.
	appWeb80 := appSvc.Web["app.test.ts.net:80"]
	assert.NotNil(t, appWeb80)
	assert.Equal(t, "http://127.0.0.1:9999", appWeb80.Handlers["/"].Proxy)

	appWeb443 := appSvc.Web["app.test.ts.net:443"]
	assert.NotNil(t, appWeb443)
	assert.Equal(t, "http://127.0.0.1:12345", appWeb443.Handlers["/"].Proxy)
}

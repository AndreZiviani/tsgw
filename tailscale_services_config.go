package main

import (
	"fmt"

	"tailscale.com/ipn"
	"tailscale.com/tailcfg"
)

func serviceNameForRoute(routeName string) tailcfg.ServiceName {
	return tailcfg.ServiceName("svc:" + routeName)
}

func buildServicesServeConfig(routeToLocalPort map[string]int, magicDNSSuffix string, redirectProxyURL string, httpPort, httpsPort uint16) *ipn.ServeConfig {
	sc := &ipn.ServeConfig{}

	for route, localPort := range routeToLocalPort {
		dnsName := serviceNameForRoute(route).String()
		proxyURL := fmt.Sprintf("http://127.0.0.1:%d", localPort)

		if httpPort != 0 {
			sc.SetWebHandler(&ipn.HTTPHandler{Proxy: redirectProxyURL}, dnsName, httpPort, "/", false, magicDNSSuffix)
		}
		if httpsPort != 0 {
			sc.SetWebHandler(&ipn.HTTPHandler{Proxy: proxyURL}, dnsName, httpsPort, "/", true, magicDNSSuffix)
		}
	}

	return sc
}

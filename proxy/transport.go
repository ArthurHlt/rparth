package proxy

import (
	"net"
	"net/http"
	"time"
)

func DefaultProxyTransport() http.RoundTripper {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		// this is a transport for a proxy
		// so i enlarge connection pooling and lower down when connetion idle will timeout
		// to release faster than default, and it will be coherent with keepalive above
		MaxIdleConns:        1000,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     30 * time.Second,

		ResponseHeaderTimeout: 30 * time.Second,
		// we do not let transport using compression to ensure
		// we can pipe directly response to our own response
		DisableCompression:  true,
		TLSHandshakeTimeout: 10 * time.Second,
	}
}

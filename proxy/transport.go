package proxy

import (
	"net"
	"net/http"

	"github.com/ArthurHlt/rparth/config"
)

//go:generate mockgen -destination=mocks/mock_transport.go -package=mocks net/http RoundTripper

func DefaultProxyTransport(transConfig config.Transport) http.RoundTripper {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   transConfig.Timeout,
			KeepAlive: transConfig.KeepAlive,
		}).DialContext,
		MaxIdleConns:          transConfig.MaxIdleConns,
		MaxIdleConnsPerHost:   transConfig.MaxIdleConnsPerHost,
		IdleConnTimeout:       transConfig.IdleConnTimeout,
		ResponseHeaderTimeout: transConfig.ResponseHeaderTimeout,
		// we do not let transport using compression to ensure
		// we can pipe directly response to our own response
		DisableCompression:  true,
		TLSHandshakeTimeout: transConfig.TLSHandshakeTimeout,
	}
}

package main

import (
	"log/slog"
	"net/http"
	"net/url"
	"os"

	"github.com/ArthurHlt/rparth/models"
	"github.com/ArthurHlt/rparth/proxy"
)

func urlMustParse(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	rts := models.RPRoutes{
		{
			Name:   "example-route-host",
			Host:   "httpbin.local",
			Target: urlMustParse("http://httpbin.org/"),
		},
		{
			Name:   "example-route-prefix",
			Prefix: "/httpbin",
			Target: urlMustParse("http://httpbin.org/"),
		},
	}
	err := rts.Validate()
	if err != nil {
		panic(err)
	}
	rparthProxy := proxy.NewProxy(proxy.DefaultProxyTransport(), rts)
	slog.Info("starting rparth proxy at http://127.0.0.1:8080")
	panic(http.ListenAndServe("127.0.0.1:8080", rparthProxy))
}

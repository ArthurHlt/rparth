package app

import (
	"log/slog"
	"net/http"

	"github.com/ArthurHlt/rparth/proxy"
)

func (a *App) RunServer() error {
	proxyHandler := proxy.NewProxy(proxy.DefaultProxyTransport(), a.cnf.Routes)
	slog.Info("Starting http server", "addr", a.cnf.Server.ListenAddr)
	return http.ListenAndServe(a.cnf.Server.ListenAddr, proxyHandler)
}

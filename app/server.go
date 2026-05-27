package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/ArthurHlt/rparth/middlewares"
	"github.com/ArthurHlt/rparth/proxy"
)

func (a *App) RunServer(stopCtx, forceCtx context.Context) error {
	proxyHandler := proxy.NewProxy(proxy.DefaultProxyTransport(), a.cnf.Routes)
	handler := middlewares.Chain(a.middlewares...)(proxyHandler)

	servConf := a.cnf.Server
	server, ln, err := a.httpServerBuilder(servConf.ListenAddr)
	if err != nil {
		return err
	}
	server.Handler = handler
	serverType := "http"
	if servConf.Tls != nil {
		serverType = "https"
	}

	slog.Info("Starting server ...", "type", serverType, "addr", ln.Addr().String())

	errChan := make(chan error, 1)
	// Run the server in a goroutine so the main goroutine can wait on stopCtx.
	go func() {
		var err error
		serverTls := servConf.Tls
		if serverTls == nil {
			err = server.Serve(ln)
		} else {
			err = server.ServeTLS(ln, serverTls.CertFile, serverTls.KeyFile)
		}
		if err != nil {
			if !errors.Is(err, http.ErrServerClosed) {
				slog.Error("Error on server", "err", err)
			}
			errChan <- err
			return
		}
	}()

	select {
	case err := <-errChan:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-stopCtx.Done():
	}

	// we wait for the server to stop gracefully
	waitShutdown := make(chan struct{})

	// we run the server shutdown in a goroutine
	// to be able to listen for a forced to stop context
	// useful in dev mode to be able to stop the server directly
	go func() {
		defer close(waitShutdown)
		ctxTimeout, cancelFunc := context.WithTimeout(context.Background(), 1*time.Minute)
		defer cancelFunc()
		slog.Debug("Stopping server ...")
		err := server.Shutdown(ctxTimeout)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Server error during shutdown", "err", err)
			return
		}
		slog.Info("Server stopped.")

	}()

	select {
	case <-waitShutdown:
		return nil
	case <-forceCtx.Done():
		// Forced exit: the shutdown goroutine may still be running and will be
		// abandoned. Acceptable here since this path is meant for dev "press
		// Ctrl+C twice" semantics where the process is about to exit anyway.
		slog.Info("Force shutdown")
	}

	return nil
}

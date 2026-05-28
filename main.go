package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ArthurHlt/rparth/app"
	"github.com/ArthurHlt/rparth/config"
	"github.com/alecthomas/kong"
)

var (
	version = "0.0.1-dev"
	commit  = "none"
	date    = "unknown"
)

type ServeCmd struct {
	ConfigPath string `short:"c" help:"path to the configuration file" type:"path" default:"./config.yml"`
}

func (r *ServeCmd) Run() error {
	cnf, err := config.ReadConfig(r.ConfigPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	appRun, err := app.NewApp(cnf)
	if err != nil {
		return fmt.Errorf("init app: %w", err)
	}
	defer appRun.Close() // nolint: errcheck

	// listen for signals SIGINT and SIGTERM
	// in fact SIGINT is for getting ctrl+c when developing
	// sigterm is for getting any orchestrator stop signal (like docker/k8s stop)
	stopCtx, stopCancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stopCancel()

	forceCtx, forceCancel := context.WithCancel(context.Background())
	defer forceCancel()

	go func() {
		<-stopCtx.Done()
		slog.Info("Stop signal received, gracefully shutting down ...")
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		defer signal.Stop(sig)
		// When developing you may not want to wait for the server to fully
		// stop: a second SIGINT or SIGTERM forces an immediate exit.
		s := <-sig
		slog.Info("Second signal received, forcing stop.", "signal", s)
		forceCancel()
	}()

	return appRun.RunServer(stopCtx, forceCtx)
}

type VersionCmd struct {
}

func (l *VersionCmd) Run() error {
	fmt.Printf("rparth %s, commit %s, built at %s\n", version, commit, date)
	return nil
}

var cli struct {
	Serve   ServeCmd   `cmd:"" help:"Run server."`
	Version VersionCmd `cmd:"" help:"Show version."`
}

func main() {
	metricBuildInfo.WithLabelValues(version, commit, date).Set(1)

	ctx := kong.Parse(&cli)
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}

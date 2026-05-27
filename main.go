package main

import (
	"fmt"

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
	appRun := app.NewApp(cnf)
	return appRun.RunServer()
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
	ctx := kong.Parse(&cli)
	err := ctx.Run()
	ctx.FatalIfErrorf(err)
}

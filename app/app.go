package app

import (
	"log/slog"
	"os"
	"time"

	"github.com/ArthurHlt/rparth/config"
	"github.com/lmittmann/tint"
)

type App struct {
	cnf *config.Config
}

func NewApp(cnf *config.Config) *App {
	app := &App{cnf: cnf}
	app.preInit()
	return app
}

func (a *App) preInit() {
	handler := tint.NewHandler(os.Stderr, &tint.Options{
		Level:      a.cnf.Log.Level,
		TimeFormat: time.Kitchen,
		NoColor:    a.cnf.Log.NoColor,
	})
	if a.cnf.Log.InJson {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
			Level: a.cnf.Log.Level,
		})
	}
	slog.SetDefault(slog.New(handler))
}

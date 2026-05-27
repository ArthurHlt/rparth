package app

import (
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/ArthurHlt/rparth/config"
	"github.com/ArthurHlt/rparth/middlewares"
	"github.com/lmittmann/tint"
)

// HttpServerBuilder builds the *http.Server and its bound listener.
// Returning a pre-bound listener lets RunServer call Serve(ln) instead of
// ListenAndServe, so tests can inject an httptest-style listener without
// racing on a free port.
type HttpServerBuilder func(listenAddr string) (*http.Server, net.Listener, error)

func defaultHttpServerBuilder(listenAddr string) (*http.Server, net.Listener, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, nil, err
	}
	return &http.Server{Addr: listenAddr}, ln, nil
}

type App struct {
	cnf               *config.Config
	httpServerBuilder HttpServerBuilder
	middlewares       []middlewares.Middleware
}

func NewApp(cnf *config.Config) *App {
	app := NewAppBare(cnf)
	app.middlewares = []middlewares.Middleware{
		middlewares.AccessLog(cnf.Log.Level),
		middlewares.Prometheus(),
		middlewares.MetricsHttp(),
	}
	return app
}

// NewAppBare returns an App with no middlewares.
// useful for testing.
func NewAppBare(cnf *config.Config) *App {
	app := &App{cnf: cnf, httpServerBuilder: defaultHttpServerBuilder}
	app.preInit()
	return app
}

func (a *App) SetMiddlewares(middlewares []middlewares.Middleware) {
	a.middlewares = middlewares
}

func (a *App) SetServerBuilder(builder HttpServerBuilder) {
	if builder == nil {
		panic("builder cannot be nil")
	}
	a.httpServerBuilder = builder
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

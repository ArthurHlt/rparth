package app

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/ArthurHlt/rparth/caches"
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
	cacheStore        caches.Cache
}

func NewApp(cnf *config.Config) (*App, error) {
	app, err := NewAppBare(cnf)
	if err != nil {
		return nil, err
	}
	app.middlewares = app.makeMiddlewares()
	return app, nil
}

// NewAppBare returns an App with no middlewares.
// useful for testing.
func NewAppBare(cnf *config.Config) (*App, error) {
	app := &App{cnf: cnf, httpServerBuilder: defaultHttpServerBuilder}
	app.preInit()
	err := app.loadCacheStore()
	if err != nil {
		return nil, err
	}
	return app, nil
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

func (a *App) makeMiddlewares() []middlewares.Middleware {
	mdws := []middlewares.Middleware{
		middlewares.AccessLog(a.cnf.Log.Level),
		middlewares.Prometheus(),
		middlewares.MetricsHttp(),
	}
	cacheHandler := a.makeCacheHandler()
	if cacheHandler != nil {
		mdws = append(mdws, middlewares.Cache(cacheHandler))
	} else {
		slog.Warn("cache is disabled")
	}
	return mdws
}

func (a *App) loadCacheStore() error {
	if a.cnf.Cache.Lru == nil && a.cnf.Cache.Redis == nil {
		return nil
	}
	var cacheStore caches.Cache
	switch {
	case a.cnf.Cache.Lru != nil:
		lruCacheConfig := a.cnf.Cache.Lru
		cacheStore = caches.NewLRUExpirable(
			int(lruCacheConfig.Size),
			nil,
			lruCacheConfig.Ttl,
		)
		slog.Info("cache configured", "type", "lru", "size", lruCacheConfig.Size, "ttl", lruCacheConfig.Ttl)
	case a.cnf.Cache.Redis != nil:
		var err error
		redisConfig := a.cnf.Cache.Redis
		cacheStore, err = caches.NewRedisCache(redisConfig.URL, redisConfig.Ttl)
		if err != nil {
			return fmt.Errorf("init redis cache: %w", err)
		}
		slog.Info("cache configured", "type", "redis", "ttl", redisConfig.Ttl)
	}
	a.cacheStore = caches.NewCacheMetrics(cacheStore)
	return nil
}

func (a *App) makeCacheHandler() *middlewares.CacheHandler {
	if a.cacheStore == nil {
		return nil
	}
	slog.Info("cache enabled", "max_size_item", a.cnf.Cache.MaxSizeItem)
	return middlewares.NewCacheHandler(a.cacheStore, int(a.cnf.Cache.MaxSizeItem))
}

func (a *App) Close() error {
	if a.cacheStore == nil {
		return nil
	}
	return a.cacheStore.Close()
}

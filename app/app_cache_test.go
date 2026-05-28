package app_test

import (
	"log/slog"
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/app"
	"github.com/ArthurHlt/rparth/config"
	"github.com/ArthurHlt/rparth/models"
	"github.com/ArthurHlt/rparth/testutils"
)

// configWithRoute returns the smallest valid Config we can pass to NewAppBare:
// one route, a listen address, and whatever cache block the caller wants.
func configWithRoute(cache config.Cache) *config.Config {
	return &config.Config{
		Routes: models.RPRoutes{
			{
				Name:   "api",
				Prefix: "/",
				Target: testutils.MustYamlParseURL("http://backend:8080"),
			},
		},
		Server: &config.Server{ListenAddr: ":0"},
		Cache:  cache,
	}
}

var _ = Describe("App cache wiring", func() {
	// NewAppBare calls preInit, which swaps slog.Default(). Snapshot and
	// restore so we don't bleed log state to other suites in this binary.
	var originalLogger *slog.Logger
	BeforeEach(func() { originalLogger = slog.Default() })
	AfterEach(func() { slog.SetDefault(originalLogger) })

	Describe("loadCacheStore via NewAppBare", func() {
		It("wires the LRU backend when cache.lru is configured", func() {
			cfg := configWithRoute(config.Cache{
				MaxSizeItem: 1024,
				Lru:         &config.LruCache{Size: 16, Ttl: time.Minute},
			})

			a, err := app.NewAppBare(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(a).NotTo(BeNil())

			// The LRU has no resources to release, so Close just returns nil.
			Expect(a.Close()).To(Succeed())
		})

		It("wires the Redis backend when cache.redis is configured", func() {
			mr := miniredis.RunT(GinkgoT())
			cfg := configWithRoute(config.Cache{
				MaxSizeItem: 1024,
				Redis:       &config.RedisCache{URL: "redis://" + mr.Addr(), Ttl: time.Minute},
			})

			a, err := app.NewAppBare(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(a).NotTo(BeNil())

			Expect(a.Close()).To(Succeed())
		})

		It("returns an error from NewAppBare when the Redis server is unreachable", func() {
			mr := miniredis.RunT(GinkgoT())
			addr := mr.Addr()
			mr.Close()

			cfg := configWithRoute(config.Cache{
				Redis: &config.RedisCache{URL: "redis://" + addr, Ttl: time.Minute},
			})

			_, err := app.NewAppBare(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("init redis cache"))
		})

		It("returns an error from NewAppBare when the Redis URL is malformed", func() {
			cfg := configWithRoute(config.Cache{
				Redis: &config.RedisCache{URL: "not a url", Ttl: time.Minute},
			})

			_, err := app.NewAppBare(cfg)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("init redis cache"))
		})

		It("succeeds with no cache block (caching disabled)", func() {
			a, err := app.NewAppBare(configWithRoute(config.Cache{}))
			Expect(err).NotTo(HaveOccurred())
			Expect(a).NotTo(BeNil())

			// Close on a cache-less app is a no-op.
			Expect(a.Close()).To(Succeed())
		})
	})

	Describe("App.Close", func() {
		It("is a no-op when no cache backend is configured", func() {
			a, err := app.NewAppBare(configWithRoute(config.Cache{}))
			Expect(err).NotTo(HaveOccurred())

			Expect(a.Close()).To(Succeed())
			// Calling Close twice should still not panic — exercises the
			// nil-cacheStore guard.
			Expect(func() { _ = a.Close() }).NotTo(Panic())
		})

		It("on a Redis-backed app, the first Close succeeds and a second Close reports the client is already closed", func() {
			mr := miniredis.RunT(GinkgoT())
			cfg := configWithRoute(config.Cache{
				Redis: &config.RedisCache{URL: "redis://" + mr.Addr(), Ttl: time.Minute},
			})

			a, err := app.NewAppBare(cfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(a.Close()).To(Succeed())
			// go-redis returns an error wrapping "client is closed" when
			// Close is invoked on an already-closed client. That's the only
			// observable proof we have that the first Close actually shut
			// the client down — the Cache interface doesn't expose state.
			err = a.Close()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("closed"))
		})
	})

	Describe("NewApp + cache wiring", func() {
		It("succeeds end-to-end when Redis is reachable", func() {
			mr := miniredis.RunT(GinkgoT())
			cfg := configWithRoute(config.Cache{
				Redis: &config.RedisCache{URL: "redis://" + mr.Addr(), Ttl: time.Minute},
			})

			a, err := app.NewApp(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(a).NotTo(BeNil())
			Expect(a.Close()).To(Succeed())
		})
	})
})

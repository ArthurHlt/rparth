package caches

import (
	"time"

	"github.com/ArthurHlt/rparth/models"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	cacheHits = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Number of cache hits",
		},
	)

	cacheMisses = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Number of cache misses",
		},
	)

	cacheLatency = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "cache_lookup_latency_seconds",
			Help:    "Cache lookup latency",
			Buckets: prometheus.DefBuckets,
		},
	)

	cacheSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "cache_size",
			Help: "Number of items in cache",
		},
	)
)

type CacheMetrics struct {
	next Cache
}

func NewCacheMetrics(cache Cache) *CacheMetrics {
	return &CacheMetrics{next: cache}
}

func (c *CacheMetrics) Get(key string) (*models.CacheData, bool) {
	startTime := time.Now()
	contains := c.next.Contains(key)
	data, ok := c.next.Get(key)
	if ok {
		cacheHits.Inc()
		cacheLatency.Observe(time.Since(startTime).Seconds())
	} else {
		cacheMisses.Inc()
	}
	// only decrement cache size if the key was not found and existed before
	if !ok && contains {
		cacheSize.Dec()
	}
	return data, ok
}

func (c *CacheMetrics) Set(key string, data *models.CacheData) {
	c.next.Set(key, data)
	cacheSize.Inc()
}

func (c *CacheMetrics) Contains(key string) bool {
	return c.next.Contains(key)
}

func (c *CacheMetrics) Close() error {
	return c.next.Close()
}

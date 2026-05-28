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
)

type CacheMetrics struct {
	next          Cache
	cacheSizeProm prometheus.GaugeFunc
}

func NewCacheMetrics(cache Cache) *CacheMetrics {
	cm := &CacheMetrics{next: cache}
	cm.cacheSizeProm = promauto.NewGaugeFunc(
		prometheus.GaugeOpts{
			Name: "cache_size",
			Help: "Number of items in cache",
		},
		func() float64 {
			return float64(cm.next.Len())
		},
	)
	return cm
}

func (c *CacheMetrics) Get(key string) (*models.CacheData, bool) {
	startTime := time.Now()

	data, ok := c.next.Get(key)
	if ok {
		cacheHits.Inc()
		cacheLatency.Observe(time.Since(startTime).Seconds())
	} else {
		cacheMisses.Inc()
	}
	return data, ok
}

func (c *CacheMetrics) Set(key string, data *models.CacheData) {
	c.next.Set(key, data)
}

func (c *CacheMetrics) Close() error {
	prometheus.DefaultRegisterer.Unregister(c.cacheSizeProm)
	return c.next.Close()
}

func (c *CacheMetrics) Len() int {
	return c.next.Len()
}

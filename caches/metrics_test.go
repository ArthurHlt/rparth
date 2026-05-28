package caches_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/ArthurHlt/rparth/caches"
	"github.com/ArthurHlt/rparth/models"
)

// fakeStore is a minimal caches.Cache implementation that lets us script
// Get/Contains/Len independently so we can assert how CacheMetrics maps the
// underlying store length onto the cache_size gauge.
type fakeStore struct {
	getReturns  *models.CacheData
	getOk       bool
	lenReturns  int
	lastSetKey  string
	lastSetData *models.CacheData
}

func (f *fakeStore) Get(string) (*models.CacheData, bool) {
	return f.getReturns, f.getOk
}

func (f *fakeStore) Set(key string, data *models.CacheData) {
	f.lastSetKey = key
	f.lastSetData = data
}

func (f *fakeStore) Close() error { return nil }

func (f *fakeStore) Len() int { return f.lenReturns }

// metricValue reads a single-cell counter or gauge from the default
// Prometheus registry by name. The caches/metrics.go vars are registered on
// the default registry via promauto, and the suite has no labels on these
// metric families so each gathers as exactly one cell.
func metricValue(name string) float64 {
	mfs, err := prometheus.DefaultGatherer.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		Expect(mf.GetMetric()).To(HaveLen(1))
		m := mf.GetMetric()[0]
		if m.Counter != nil {
			return m.GetCounter().GetValue()
		}
		if m.Gauge != nil {
			return m.GetGauge().GetValue()
		}
		Fail("metric " + name + " is neither counter nor gauge")
	}
	Fail("metric not found: " + name)
	return 0
}

// observationCount reads the sample count of a single-cell histogram.
func observationCount(name string) uint64 {
	mfs, err := prometheus.DefaultGatherer.Gather()
	Expect(err).NotTo(HaveOccurred())
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		Expect(mf.GetMetric()).To(HaveLen(1))
		h := mf.GetMetric()[0].GetHistogram()
		Expect(h).NotTo(BeNil())
		return h.GetSampleCount()
	}
	Fail("histogram not found: " + name)
	return 0
}

var _ = Describe("CacheMetrics", func() {
	var (
		store *fakeStore
		cache *caches.CacheMetrics
	)

	BeforeEach(func() {
		store = &fakeStore{}
		cache = caches.NewCacheMetrics(store)
	})
	
	AfterEach(func() {
		err := cache.Close()
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Get", func() {
		It("delegates to the underlying store and returns the payload on a hit", func() {
			payload := &models.CacheData{Status: 200, Body: []byte("hi")}
			store.getReturns = payload
			store.getOk = true

			data, ok := cache.Get("a")
			Expect(ok).To(BeTrue())
			Expect(data).To(BeIdenticalTo(payload))
		})

		It("returns (nil, false) when the store reports a miss", func() {
			store.getOk = false

			data, ok := cache.Get("absent")
			Expect(ok).To(BeFalse())
			Expect(data).To(BeNil())
		})

		It("increments cache_hits_total and records latency on a hit", func() {
			beforeHits := metricValue("cache_hits_total")
			beforeLatency := observationCount("cache_lookup_latency_seconds")

			store.getReturns = &models.CacheData{Status: 200}
			store.getOk = true
			cache.Get("a")

			Expect(metricValue("cache_hits_total") - beforeHits).To(Equal(float64(1)))
			Expect(observationCount("cache_lookup_latency_seconds") - beforeLatency).To(Equal(uint64(1)))
		})

		It("increments cache_misses_total on a miss", func() {
			before := metricValue("cache_misses_total")

			store.getOk = false
			cache.Get("absent")

			Expect(metricValue("cache_misses_total") - before).To(Equal(float64(1)))
		})

		It("updates cache_size to the underlying store length", func() {
			store.getOk = false
			store.lenReturns = 4
			cache.Get("absent")

			Expect(metricValue("cache_size")).To(Equal(float64(4)))
		})
	})

	Describe("Set", func() {
		It("delegates to the underlying store", func() {
			payload := &models.CacheData{Status: 201, Body: []byte("bye")}
			cache.Set("k", payload)

			Expect(store.lastSetKey).To(Equal("k"))
			Expect(store.lastSetData).To(BeIdenticalTo(payload))
		})

		It("updates cache_size to the underlying store length", func() {
			store.lenReturns = 3
			cache.Set("k", &models.CacheData{Status: 200})
			Expect(metricValue("cache_size")).To(Equal(float64(3)))
		})
	})
})

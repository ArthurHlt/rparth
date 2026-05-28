package caches_test

import (
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/caches"
	"github.com/ArthurHlt/rparth/models"
)

var _ caches.Cache = (*caches.LRUExpirable)(nil)

var _ = Describe("LRUExpirable", func() {
	var (
		cache    *caches.LRUExpirable
		evicted  map[string]*models.CacheData
		evictMu  sync.Mutex
		onEvict  caches.EvictionCallback
		sampleA  *models.CacheData
		sampleB  *models.CacheData
	)

	BeforeEach(func() {
		evicted = map[string]*models.CacheData{}
		onEvict = func(key string, value *models.CacheData) {
			evictMu.Lock()
			defer evictMu.Unlock()
			evicted[key] = value
		}
		sampleA = &models.CacheData{Status: 200, Body: []byte("alpha"), Headers: map[string][]string{"X-A": {"1"}}}
		sampleB = &models.CacheData{Status: 404, Body: []byte("beta"), Headers: map[string][]string{"X-B": {"2"}}}
	})

	readEvicted := func() map[string]*models.CacheData {
		evictMu.Lock()
		defer evictMu.Unlock()
		out := make(map[string]*models.CacheData, len(evicted))
		for k, v := range evicted {
			out[k] = v
		}
		return out
	}

	Describe("Get", func() {
		BeforeEach(func() {
			cache = caches.NewLRUExpirable(8, onEvict, time.Minute)
		})

		It("returns (nil, false) for a missing key", func() {
			data, ok := cache.Get("absent")
			Expect(ok).To(BeFalse())
			Expect(data).To(BeNil())
		})

		It("returns the value previously stored under the key", func() {
			cache.Set("a", sampleA)
			data, ok := cache.Get("a")
			Expect(ok).To(BeTrue())
			Expect(data).To(BeIdenticalTo(sampleA))
		})
	})

	Describe("Set", func() {
		BeforeEach(func() {
			cache = caches.NewLRUExpirable(8, onEvict, time.Minute)
		})

		It("overwrites the value when the key is set twice", func() {
			cache.Set("a", sampleA)
			cache.Set("a", sampleB)

			data, ok := cache.Get("a")
			Expect(ok).To(BeTrue())
			Expect(data).To(BeIdenticalTo(sampleB))
		})
	})

	Describe("Delete", func() {
		BeforeEach(func() {
			cache = caches.NewLRUExpirable(8, onEvict, time.Minute)
		})

		It("removes the key from the cache", func() {
			cache.Set("a", sampleA)
			cache.Delete("a")

			data, ok := cache.Get("a")
			Expect(ok).To(BeFalse())
			Expect(data).To(BeNil())
		})

		It("is a no-op when the key is absent", func() {
			Expect(func() { cache.Delete("missing") }).NotTo(Panic())
		})
	})

	Describe("LRU size eviction", func() {
		It("evicts the oldest entry when capacity is exceeded and invokes the callback", func() {
			cache = caches.NewLRUExpirable(2, onEvict, time.Minute)

			cache.Set("a", sampleA)
			cache.Set("b", sampleB)
			third := &models.CacheData{Status: 500, Body: []byte("gamma")}
			cache.Set("c", third)

			_, okA := cache.Get("a")
			Expect(okA).To(BeFalse())

			Expect(readEvicted()).To(HaveKeyWithValue("a", sampleA))

			dataB, okB := cache.Get("b")
			Expect(okB).To(BeTrue())
			Expect(dataB).To(BeIdenticalTo(sampleB))

			dataC, okC := cache.Get("c")
			Expect(okC).To(BeTrue())
			Expect(dataC).To(BeIdenticalTo(third))
		})
	})

	Describe("TTL expiration", func() {
		It("returns (nil, false) once the TTL has elapsed and invokes the eviction callback", func() {
			cache = caches.NewLRUExpirable(8, onEvict, 50*time.Millisecond)
			cache.Set("a", sampleA)

			Eventually(func() bool {
				_, ok := cache.Get("a")
				return ok
			}, "500ms", "10ms").Should(BeFalse())

			Eventually(readEvicted, "500ms", "10ms").Should(HaveKeyWithValue("a", sampleA))
		})
	})

	Describe("constructor", func() {
		It("tolerates a nil eviction callback", func() {
			c := caches.NewLRUExpirable(2, nil, time.Minute)
			c.Set("a", sampleA)
			c.Set("b", sampleB)
			Expect(func() {
				c.Set("c", &models.CacheData{Status: 500, Body: []byte("gamma")})
			}).NotTo(Panic())
		})
	})
})

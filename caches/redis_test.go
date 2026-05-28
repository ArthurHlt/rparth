package caches_test

import (
	"time"

	"github.com/alicebob/miniredis/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/caches"
	"github.com/ArthurHlt/rparth/models"
)

var _ caches.Cache = (*caches.RedisCache)(nil)

var _ = Describe("RedisCache", func() {
	var (
		mr      *miniredis.Miniredis
		cache   *caches.RedisCache
		sampleA *models.CacheData
	)

	BeforeEach(func() {
		mr = miniredis.RunT(GinkgoT())
		var err error
		cache, err = caches.NewRedisCache("redis://"+mr.Addr(), time.Minute)
		Expect(err).NotTo(HaveOccurred())
		sampleA = &models.CacheData{
			Status:  200,
			Body:    []byte("alpha"),
			Headers: map[string][]string{"Content-Type": {"text/plain"}},
		}
	})

	Describe("Get/Set", func() {
		It("returns (nil, false) for a missing key", func() {
			data, ok := cache.Get("absent")
			Expect(ok).To(BeFalse())
			Expect(data).To(BeNil())
		})

		It("round-trips a CacheData value", func() {
			cache.Set("a", sampleA)
			data, ok := cache.Get("a")
			Expect(ok).To(BeTrue())
			Expect(data.Status).To(Equal(sampleA.Status))
			Expect(data.Body).To(Equal(sampleA.Body))
			Expect(data.Headers).To(Equal(sampleA.Headers))
		})
	})

	Describe("TTL", func() {
		It("returns miss once the TTL has elapsed", func() {
			cache.Set("a", sampleA)
			mr.FastForward(time.Minute + time.Second)
			_, ok := cache.Get("a")
			Expect(ok).To(BeFalse())
		})
	})

	Describe("key prefix", func() {
		It("stores keys under the 'rparth:' prefix", func() {
			cache.Set("a", sampleA)
			Expect(mr.Keys()).To(ConsistOf("rparth:a"))
		})
	})

	Describe("Len", func() {
		It("returns 0 when the cache is empty", func() {
			Expect(cache.Len()).To(Equal(0))
		})

		It("counts only keys under the 'rparth:' prefix", func() {
			cache.Set("a", sampleA)
			cache.Set("b", sampleA)
			Expect(mr.Set("other", "x")).To(Succeed())
			Expect(cache.Len()).To(Equal(2))
		})
	})

	Describe("constructor", func() {
		It("errors on a malformed URL", func() {
			_, err := caches.NewRedisCache("not a url", time.Minute)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("parse redis url"))
		})

		It("errors when the server is unreachable", func() {
			addr := mr.Addr()
			mr.Close()
			_, err := caches.NewRedisCache("redis://"+addr, time.Minute)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("redis ping"))
		})
	})
})

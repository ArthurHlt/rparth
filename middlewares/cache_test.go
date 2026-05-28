package middlewares_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/ArthurHlt/rparth/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/contexts"
	"github.com/ArthurHlt/rparth/middlewares"
	"github.com/ArthurHlt/rparth/models"
)

type fakeCache struct {
	mu       sync.Mutex
	data     map[string]*models.CacheData
	getCalls []string
	setCalls []string
}

func (f *fakeCache) Close() error {
	return nil
}

func newFakeCache() *fakeCache {
	return &fakeCache{data: map[string]*models.CacheData{}}
}

func (f *fakeCache) Get(key string) (*models.CacheData, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls = append(f.getCalls, key)
	d, ok := f.data[key]
	return d, ok
}

func (f *fakeCache) Set(key string, data *models.CacheData) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.setCalls = append(f.setCalls, key)
	f.data[key] = data
}

func (f *fakeCache) Delete(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, key)
}

func (f *fakeCache) Contains(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.data[key]
	return ok
}

func (f *fakeCache) onlyEntry() *models.CacheData {
	f.mu.Lock()
	defer f.mu.Unlock()
	Expect(f.data).To(HaveLen(1))
	for _, v := range f.data {
		return v
	}
	return nil
}

type flushRecorder struct {
	*httptest.ResponseRecorder
	flushes int
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (f *flushRecorder) Flush() { f.flushes++ }

var _ = Describe("Cache middleware", func() {
	var (
		store      *fakeCache
		nextCalled int
	)

	BeforeEach(func() {
		store = newFakeCache()
		nextCalled = 0
	})

	build := func(maxBodySize int, next http.Handler) http.Handler {
		return middlewares.Cache(middlewares.NewCacheHandler(store, maxBodySize))(next)
	}

	Describe("non-GET requests", func() {
		It("passes through without touching the cache", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled++
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte("created"))
			})
			h := build(1024, next)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/foo", strings.NewReader("body")))

			Expect(nextCalled).To(Equal(1))
			Expect(rec.Code).To(Equal(http.StatusCreated))
			Expect(rec.Body.String()).To(Equal("created"))
			Expect(store.getCalls).To(BeEmpty())
			Expect(store.setCalls).To(BeEmpty())
		})
	})

	Describe("GET miss", func() {
		It("invokes next, forwards the response, and stores the entry", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled++
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("hello"))
			})
			h := build(1024, next)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/foo", "r1"))

			Expect(nextCalled).To(Equal(1))
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).To(Equal("hello"))
			Expect(rec.Header().Get("X-Cache")).To(BeEmpty())

			cached := store.onlyEntry()
			Expect(cached.Status).To(Equal(http.StatusOK))
			Expect(string(cached.Body)).To(Equal("hello"))
			Expect(http.Header(cached.Headers).Get("Content-Type")).To(Equal("text/plain"))
		})

		It("defaults the cached status to 200 when the handler never calls WriteHeader", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("body"))
			})
			h := build(1024, next)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/foo", "r1"))

			cached := store.onlyEntry()
			Expect(cached.Status).To(Equal(http.StatusOK))
			Expect(string(cached.Body)).To(Equal("body"))
		})

		It("preserves the cached body across multiple Write calls", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("alpha-"))
				w.Write([]byte("beta-"))
				w.Write([]byte("gamma"))
			})
			h := build(1024, next)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/foo", "r1"))

			Expect(rec.Body.String()).To(Equal("alpha-beta-gamma"))
			Expect(string(store.onlyEntry().Body)).To(Equal("alpha-beta-gamma"))
		})

		It("does not cache responses that carry Set-Cookie, to avoid replaying one client's session to another", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Set-Cookie", "session=abc; Path=/")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("hi"))
			})
			h := build(1024, next)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/foo", "r1"))

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).To(Equal("hi"))
			Expect(rec.Header().Get("Set-Cookie")).To(Equal("session=abc; Path=/"))
			Expect(store.setCalls).To(BeEmpty())
		})
	})

	Describe("GET hit", func() {
		It("serves from the cache, sets X-Cache: HIT, restores cached headers, and does not call next", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"v":1}`))
			})
			h := build(1024, next)

			h.ServeHTTP(httptest.NewRecorder(), testutils.RequestWithRoute(http.MethodGet, "/foo", "r1"))
			Expect(nextCalled).To(Equal(1))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/foo", "r1"))

			Expect(nextCalled).To(Equal(1))
			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).To(Equal(`{"v":1}`))
			Expect(rec.Header().Get("X-Cache")).To(Equal("HIT"))
			Expect(rec.Header().Get("Content-Type")).To(Equal("application/json"))
		})
	})

	Describe("body size cutoff", func() {
		It("does not store the entry when the body exceeds maxBodySize, but still forwards the full response", func() {
			payload := strings.Repeat("x", 2048)
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(payload))
			})
			h := build(1024, next)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/big", "r1"))

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).To(Equal(payload))
			Expect(store.setCalls).To(BeEmpty())
		})

		It("treats maxBodySize <= 0 as unlimited", func() {
			payload := strings.Repeat("y", 4096)
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(payload))
			})
			h := build(0, next)

			h.ServeHTTP(httptest.NewRecorder(), testutils.RequestWithRoute(http.MethodGet, "/big", "r1"))

			Expect(string(store.onlyEntry().Body)).To(Equal(payload))
		})

		It("triggers the cutoff across chunked writes that together exceed the limit", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(strings.Repeat("a", 600)))
				w.Write([]byte(strings.Repeat("b", 600)))
			})
			h := build(1024, next)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/chunked", "r1"))

			Expect(rec.Body.Len()).To(Equal(1200))
			Expect(store.setCalls).To(BeEmpty())
		})
	})

	Describe("status filtering", func() {
		writeStatus := func(status int) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				w.Write([]byte("body"))
			})
		}

		It("does not cache 4xx responses", func() {
			h := build(1024, writeStatus(http.StatusNotFound))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/missing", "r1"))

			Expect(rec.Code).To(Equal(http.StatusNotFound))
			Expect(rec.Body.String()).To(Equal("body"))
			Expect(store.setCalls).To(BeEmpty())
		})

		It("does not cache 5xx responses", func() {
			h := build(1024, writeStatus(http.StatusInternalServerError))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/boom", "r1"))

			Expect(rec.Code).To(Equal(http.StatusInternalServerError))
			Expect(store.setCalls).To(BeEmpty())
		})

		It("does not cache 1xx responses", func() {
			h := build(1024, writeStatus(http.StatusProcessing))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/early", "r1"))

			Expect(store.setCalls).To(BeEmpty())
		})

		It("caches 3xx redirects", func() {
			h := build(1024, writeStatus(http.StatusMovedPermanently))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/redir", "r1"))

			Expect(rec.Code).To(Equal(http.StatusMovedPermanently))
			Expect(store.setCalls).To(HaveLen(1))
			Expect(store.onlyEntry().Status).To(Equal(http.StatusMovedPermanently))
		})
	})

	Describe("request Cache-Control", func() {
		It("bypasses the cache (and does not store) when the request carries Cache-Control: no-store", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled++
				w.Write([]byte("body"))
			})
			h := build(1024, next)

			req := testutils.RequestWithRoute(http.MethodGet, "/foo", "r1")
			req.Header.Set("Cache-Control", "no-store")
			h.ServeHTTP(httptest.NewRecorder(), req)

			Expect(nextCalled).To(Equal(1))
			Expect(store.getCalls).To(BeEmpty())
			Expect(store.setCalls).To(BeEmpty())
		})

		It("bypasses the cache when the request carries Cache-Control: no-cache", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("body"))
			})
			h := build(1024, next)

			req := testutils.RequestWithRoute(http.MethodGet, "/foo", "r1")
			req.Header.Set("Cache-Control", "no-cache")
			h.ServeHTTP(httptest.NewRecorder(), req)

			Expect(store.getCalls).To(BeEmpty())
			Expect(store.setCalls).To(BeEmpty())
		})

		It("parses comma-separated request directives", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("body"))
			})
			h := build(1024, next)

			req := testutils.RequestWithRoute(http.MethodGet, "/foo", "r1")
			req.Header.Set("Cache-Control", "max-age=0, no-store")
			h.ServeHTTP(httptest.NewRecorder(), req)

			Expect(store.setCalls).To(BeEmpty())
		})
	})

	Describe("response Cache-Control and Vary", func() {
		writeWithHeaders := func(headers map[string]string) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("body"))
			})
		}

		DescribeTable("does not cache when the response carries a disallowed directive",
			func(headerName, headerValue string) {
				h := build(1024, writeWithHeaders(map[string]string{headerName: headerValue}))

				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/foo", "r1"))

				Expect(rec.Code).To(Equal(http.StatusOK))
				Expect(rec.Body.String()).To(Equal("body"))
				Expect(store.setCalls).To(BeEmpty())
			},
			Entry("Cache-Control: no-store", "Cache-Control", "no-store"),
			Entry("Cache-Control: no-cache", "Cache-Control", "no-cache"),
			Entry("Cache-Control: private", "Cache-Control", "private"),
			Entry("Cache-Control with mixed directives", "Cache-Control", "public, no-store"),
			Entry("Vary: Accept-Encoding", "Vary", "Accept-Encoding"),
			Entry("Vary: *", "Vary", "*"),
		)

		It("still caches when the response Cache-Control is a benign directive like public", func() {
			h := build(1024, writeWithHeaders(map[string]string{"Cache-Control": "public, max-age=60"}))

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/foo", "r1"))

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(store.setCalls).To(HaveLen(1))
		})
	})

	Describe("cache key", func() {
		It("differs across route names so two routes hitting the same URL don't collide", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				route := contexts.GetRPRoute(r)
				w.Write([]byte(route.Name))
			})
			h := build(1024, next)

			h.ServeHTTP(httptest.NewRecorder(), testutils.RequestWithRoute(http.MethodGet, "/same", "r1"))
			h.ServeHTTP(httptest.NewRecorder(), testutils.RequestWithRoute(http.MethodGet, "/same", "r2"))

			Expect(store.setCalls).To(HaveLen(2))
			Expect(store.setCalls[0]).NotTo(Equal(store.setCalls[1]))
		})

		It("differs across URLs for the same route", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("body"))
			})
			h := build(1024, next)

			h.ServeHTTP(httptest.NewRecorder(), testutils.RequestWithRoute(http.MethodGet, "/a", "r1"))
			h.ServeHTTP(httptest.NewRecorder(), testutils.RequestWithRoute(http.MethodGet, "/b", "r1"))

			Expect(store.setCalls).To(HaveLen(2))
			Expect(store.setCalls[0]).NotTo(Equal(store.setCalls[1]))
		})

		It("differs by Authorization header so two users don't share a cached response", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled++
				w.Write([]byte("body"))
			})
			h := build(1024, next)

			req1 := testutils.RequestWithRoute(http.MethodGet, "/same", "r1")
			req1.Header.Set("Authorization", "Bearer user-1")
			h.ServeHTTP(httptest.NewRecorder(), req1)

			req2 := testutils.RequestWithRoute(http.MethodGet, "/same", "r1")
			req2.Header.Set("Authorization", "Bearer user-2")
			h.ServeHTTP(httptest.NewRecorder(), req2)

			Expect(nextCalled).To(Equal(2))
			Expect(store.setCalls).To(HaveLen(2))
			Expect(store.setCalls[0]).NotTo(Equal(store.setCalls[1]))
		})

		It("reuses the cache entry when Authorization repeats", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled++
				w.Write([]byte("body"))
			})
			h := build(1024, next)

			req1 := testutils.RequestWithRoute(http.MethodGet, "/same", "r1")
			req1.Header.Set("Authorization", "Bearer user-1")
			h.ServeHTTP(httptest.NewRecorder(), req1)

			req2 := testutils.RequestWithRoute(http.MethodGet, "/same", "r1")
			req2.Header.Set("Authorization", "Bearer user-1")
			rec2 := httptest.NewRecorder()
			h.ServeHTTP(rec2, req2)

			Expect(nextCalled).To(Equal(1))
			Expect(rec2.Header().Get("X-Cache")).To(Equal("HIT"))
		})

		It("differs by Cookie header so two clients with distinct cookies don't share a cached response", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled++
				w.Write([]byte("body"))
			})
			h := build(1024, next)

			req1 := testutils.RequestWithRoute(http.MethodGet, "/same", "r1")
			req1.Header.Set("Cookie", "session=a")
			h.ServeHTTP(httptest.NewRecorder(), req1)

			req2 := testutils.RequestWithRoute(http.MethodGet, "/same", "r1")
			req2.Header.Set("Cookie", "session=b")
			h.ServeHTTP(httptest.NewRecorder(), req2)

			Expect(nextCalled).To(Equal(2))
			Expect(store.setCalls).To(HaveLen(2))
			Expect(store.setCalls[0]).NotTo(Equal(store.setCalls[1]))
		})

		It("treats a request with no Authorization as distinct from one carrying any value", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled++
				w.Write([]byte("body"))
			})
			h := build(1024, next)

			h.ServeHTTP(httptest.NewRecorder(), testutils.RequestWithRoute(http.MethodGet, "/same", "r1"))
			req2 := testutils.RequestWithRoute(http.MethodGet, "/same", "r1")
			req2.Header.Set("Authorization", "Bearer x")
			h.ServeHTTP(httptest.NewRecorder(), req2)

			Expect(nextCalled).To(Equal(2))
			Expect(store.setCalls).To(HaveLen(2))
			Expect(store.setCalls[0]).NotTo(Equal(store.setCalls[1]))
		})
	})

	Describe("Flush", func() {
		It("forwards Flush to an underlying http.Flusher", func() {
			rec := newFlushRecorder()
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("part-"))
				w.(http.Flusher).Flush()
				w.Write([]byte("rest"))
			})
			h := build(1024, next)

			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/stream", "r1"))

			Expect(rec.flushes).To(Equal(1))
			Expect(rec.Body.String()).To(Equal("part-rest"))
			Expect(string(store.onlyEntry().Body)).To(Equal("part-rest"))
		})

		It("is a no-op when the underlying writer does not implement http.Flusher", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte("body"))
				// httptest.ResponseRecorder does not implement http.Flusher; the
				// cacheResponseWriter must not panic when its parent lacks Flush.
				w.(http.Flusher).Flush()
			})
			h := build(1024, next)

			Expect(func() {
				h.ServeHTTP(httptest.NewRecorder(), testutils.RequestWithRoute(http.MethodGet, "/no-flush", "r1"))
			}).NotTo(Panic())
		})
	})

	Describe("ETag and conditional requests", func() {
		It("sets an ETag on the forwarded response on cache miss", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("payload"))
			})
			h := build(1024, next)

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/etag", "r1"))

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Header().Get("ETag")).NotTo(BeEmpty())
		})

		It("returns 304 Not Modified when If-None-Match matches the cached entry's ETag", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("payload"))
			})
			h := build(1024, next)

			firstRec := httptest.NewRecorder()
			h.ServeHTTP(firstRec, testutils.RequestWithRoute(http.MethodGet, "/etag", "r1"))
			etag := firstRec.Header().Get("ETag")
			Expect(etag).NotTo(BeEmpty())

			req := testutils.RequestWithRoute(http.MethodGet, "/etag", "r1")
			req.Header.Set("If-None-Match", etag)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusNotModified))
			Expect(rec.Body.Len()).To(Equal(0))
		})

		It("serves the full response when If-None-Match does not match", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("payload"))
			})
			h := build(1024, next)

			// Warm the cache.
			h.ServeHTTP(httptest.NewRecorder(), testutils.RequestWithRoute(http.MethodGet, "/etag", "r1"))

			req := testutils.RequestWithRoute(http.MethodGet, "/etag", "r1")
			req.Header.Set("If-None-Match", "deadbeef-does-not-match")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			Expect(rec.Code).To(Equal(http.StatusOK))
			Expect(rec.Body.String()).To(Equal("payload"))
			Expect(rec.Header().Get("X-Cache")).To(Equal("HIT"))
		})
	})

	Describe("route.NoCache", func() {
		It("bypasses the cache entirely when the matched route has NoCache=true", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled++
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("fresh"))
			})
			h := build(1024, next)

			req := httptest.NewRequest(http.MethodGet, "/uncached", nil)
			req = contexts.SetRPRoute(req, &models.RPRoute{Name: "uncached-route", NoCache: true})

			h.ServeHTTP(httptest.NewRecorder(), req)
			h.ServeHTTP(httptest.NewRecorder(), req)

			Expect(nextCalled).To(Equal(2), "next should be called every time for NoCache routes")
			Expect(store.getCalls).To(BeEmpty(), "cache.Get should never be consulted")
			Expect(store.setCalls).To(BeEmpty(), "responses must not be stored")
		})

		It("still caches when NoCache is false (default)", func() {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				nextCalled++
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("body"))
			})
			h := build(1024, next)

			req := httptest.NewRequest(http.MethodGet, "/cached", nil)
			req = contexts.SetRPRoute(req, &models.RPRoute{Name: "cached-route", NoCache: false})

			h.ServeHTTP(httptest.NewRecorder(), req)
			Expect(store.setCalls).To(HaveLen(1))
		})
	})
})

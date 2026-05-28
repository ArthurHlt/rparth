package middlewares

import (
	"bytes"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/ArthurHlt/rparth/caches"
	"github.com/ArthurHlt/rparth/contexts"
	"github.com/ArthurHlt/rparth/models"
	"github.com/cespare/xxhash/v2"
)

func Cache(handler *CacheHandler) Middleware {
	return func(next http.Handler) http.Handler {
		return handler.serveMiddleware(next)
	}
}

type cacheResponseWriter struct {
	w             http.ResponseWriter
	cacheData     *models.CacheData
	buf           *bytes.Buffer
	maxBodySize   int
	shouldCache   bool
	noCacheReason cacheSkipReason
	key           string
	headerWritten bool
}

func newCacheResponseWriter(key string, w http.ResponseWriter, maxBodySize int) *cacheResponseWriter {
	return &cacheResponseWriter{
		w:           w,
		cacheData:   &models.CacheData{Status: http.StatusOK},
		buf:         &bytes.Buffer{},
		maxBodySize: maxBodySize,
		shouldCache: true,
		key:         key,
	}
}

func (c *cacheResponseWriter) Header() http.Header {
	return c.w.Header()
}

func (c *cacheResponseWriter) Write(p []byte) (int, error) {
	if c.shouldCache {
		if c.maxBodySize > 0 && c.buf.Len()+len(p) > c.maxBodySize {
			c.noCacheReason = cacheSkipReasonTooLarge
			c.shouldCache = false
			c.buf.Reset()
		} else {
			c.buf.Write(p)
		}
	}
	return c.w.Write(p)
}

func (c *cacheResponseWriter) allowCachingFromHeaders(h http.Header) bool {
	// Set-Cookie is per-client so we should not cache
	if h.Get("Set-Cookie") != "" {
		c.noCacheReason = cacheSkipReasonSetCookie
		return false
	}
	// if response can vary we should not cache
	if h.Get("Vary") != "" {
		c.noCacheReason = cacheSkipReasonVary
		return false
	}
	// if response has cache control directives we follow where we should not cache
	if hasCacheControl(h, "no-store", "no-cache", "private") {
		c.noCacheReason = cacheSkipReasonCacheControl
		return false
	}
	return true
}

func (c *cacheResponseWriter) WriteHeader(statusCode int) {
	c.headerWritten = true
	c.injectCacheHttpHeader()
	c.cacheData.Status = statusCode
	if statusCode >= 400 || statusCode < 200 {
		c.noCacheReason = cacheSkipReasonStatusCode
		c.shouldCache = false
	}
	if c.shouldCache {
		c.shouldCache = c.allowCachingFromHeaders(c.w.Header())
	}
	c.w.WriteHeader(statusCode)
}

func (c *cacheResponseWriter) injectCacheHttpHeader() {
	c.w.Header().Set("ETag", c.key)
}

func (c *cacheResponseWriter) Unwrap() http.ResponseWriter {
	return c.w
}

func (c *cacheResponseWriter) finalize() {
	if !c.shouldCache {
		return
	}
	if !c.headerWritten {
		c.injectCacheHttpHeader()
	}
	c.cacheData.Body = c.buf.Bytes()
	c.cacheData.Headers = c.Header().Clone()
}

type CacheHandler struct {
	store       caches.Cache
	maxBodySize int
}

func NewCacheHandler(store caches.Cache, maxBodySize int) *CacheHandler {
	return &CacheHandler{store: store, maxBodySize: maxBodySize}
}

func (c *CacheHandler) makeHash(r *http.Request) string {
	h := xxhash.New()
	h.WriteString(r.Method)
	h.WriteString(r.URL.String())
	h.WriteString(contexts.GetRouteName(r))
	// user can set Authorization and Cookie headers
	// which means that user is authenticated, to avoid that another user get the content of another
	// we hash the Authorization and Cookie headers too if there is ones
	authorization := r.Header.Get("Authorization")
	if authorization != "" {
		h.WriteString(authorization)
	}
	cookie := r.Header.Get("Cookie")
	if cookie != "" {
		h.WriteString(cookie)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (c *CacheHandler) allowCaching(r *http.Request) (bool, cacheSkipReason) {
	route := contexts.GetRPRoute(r)
	if route != nil && route.NoCache {
		return false, cacheSkipReasonDisabled
	}
	if r.Method != http.MethodGet {
		return false, cacheSkipReasonMethodNotAllowed
	}
	if hasCacheControl(r.Header, "no-store", "no-cache") {
		return false, cacheSkipReasonCacheControl
	}
	return true, ""
}

func hasCacheControl(h http.Header, directives ...string) bool {
	cc := strings.ToLower(h.Get("Cache-Control"))
	if cc == "" {
		return false
	}
	for _, d := range directives {
		if strings.Contains(cc, d) {
			return true
		}
	}
	return false
}

func (c *CacheHandler) serveFromCache(key string, item *models.CacheData, w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("If-None-Match") == key {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	newHeaders := http.Header(item.Headers).Clone()
	newHeaders.Set("X-Cache", "HIT")
	maps.Copy(w.Header(), newHeaders)
	w.WriteHeader(item.Status)
	w.Write(item.Body)
}

func (c *CacheHandler) serveMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routeName := contexts.GetRouteName(r)
		shouldCache, reason := c.allowCaching(r)
		if !shouldCache {
			cacheSkip.WithLabelValues(routeName, string(reason)).Inc()
			next.ServeHTTP(w, r)
			return
		}
		key := c.makeHash(r)
		ca, ok := c.store.Get(key)
		if ok {
			c.serveFromCache(key, ca, w, r)
			return
		}
		crw := newCacheResponseWriter(key, w, c.maxBodySize)
		next.ServeHTTP(crw, r)
		crw.finalize()
		if !crw.shouldCache {
			cacheSkip.WithLabelValues(routeName, string(crw.noCacheReason)).Inc()
			return
		}
		c.store.Set(key, crw.cacheData)
	})
}

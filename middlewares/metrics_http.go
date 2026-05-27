package middlewares

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "status"},
	)
)

// MetricsHttp records request count and duration labelled by method, path
// and status.
// pass a templating pathFn (e.g. "/users/123" → "/users/{id}") in front of unbounded paths
// to avoid a Prometheus cardinality explosion (and sad OOM).
// by default, pathFn is the request URL path if nil.
func MetricsHttp(pathFn func(*http.Request) string) Middleware {
	if pathFn == nil {
		pathFn = func(r *http.Request) string { return r.URL.Path }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := pathFn(r)
			start := time.Now()
			rw := newResponseWriter(w)

			next.ServeHTTP(rw, r)

			status := strconv.Itoa(rw.statusCode)
			duration := time.Since(start).Seconds()

			httpRequestsTotal.WithLabelValues(r.Method, path, status).Inc()
			httpRequestDuration.WithLabelValues(r.Method, path, status).Observe(duration)
		})
	}
}

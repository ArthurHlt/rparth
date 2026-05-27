package middlewares

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ArthurHlt/rparth/contexts"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"route_name", "method", "status"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"route_name", "method", "status"},
	)
)

func MetricsHttp() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			route := contexts.GetRPRoute(r)
			routeName := "unknown"
			if route != nil {
				routeName = route.Name
			}
			start := time.Now()
			rw := newResponseWriter(w)

			next.ServeHTTP(rw, r)

			status := strconv.Itoa(rw.statusCode)
			duration := time.Since(start).Seconds()

			httpRequestsTotal.WithLabelValues(routeName, r.Method, status).Inc()
			httpRequestDuration.WithLabelValues(routeName, r.Method, status).Observe(duration)
		})
	}
}

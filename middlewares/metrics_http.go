package middlewares

import (
	"net/http"
	"strconv"
	"time"

	"github.com/ArthurHlt/rparth/contexts"
)

func MetricsHttp() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			routeName := contexts.GetRouteName(r)

			httpRequestsInFlight.WithLabelValues(routeName).Inc()
			defer httpRequestsInFlight.WithLabelValues(routeName).Dec()

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

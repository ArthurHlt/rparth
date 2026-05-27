package middlewares

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Prometheus returns a middleware that exposes Prometheus metrics.
// this is the simplest way to expose metrics in our context where i only want chain middlewares
func Prometheus() Middleware {
	promHandler := promhttp.Handler()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/_metrics" {
				promHandler.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

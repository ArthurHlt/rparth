package middlewares

import (
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

	httpRequestsInFlight = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests in flight",
		},
		[]string{"route_name"},
	)

	cacheSkip = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "cache_skip_total",
			Help: "Number of responses not cached, by reason",
		},
		[]string{"route_name", "reason"},
	)
)

type cacheSkipReason string

const (
	cacheSkipReasonDisabled         cacheSkipReason = "disabled"
	cacheSkipReasonTooLarge         cacheSkipReason = "too_large"
	cacheSkipReasonStatusCode       cacheSkipReason = "status_code"
	cacheSkipReasonSetCookie        cacheSkipReason = "set_cookie"
	cacheSkipReasonVary             cacheSkipReason = "vary"
	cacheSkipReasonCacheControl     cacheSkipReason = "cache_control"
	cacheSkipReasonMethodNotAllowed cacheSkipReason = "method_not_allowed"
)

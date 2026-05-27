package proxy

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpProxyRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_proxy_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"route_name", "method", "status"},
	)

	httpProxyRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_proxy_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"route_name", "method", "status"},
	)
)

package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"syscall"

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

	httpProxyErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_proxy_errors_total",
			Help: "Total number of HTTP errors",
		},
		[]string{"route_name", "method", "reason"},
	)
)

type httpErrorReason string

const (
	httpErrorReasonTimeout           httpErrorReason = "timeout"
	httpErrorReasonCanceled          httpErrorReason = "canceled"
	httpErrorReasonDNS               httpErrorReason = "dns"
	httpErrorReasonConnectionRefused httpErrorReason = "connection_refused"
	httpErrorReasonTLS               httpErrorReason = "tls"
	httpErrorReasonConnection        httpErrorReason = "connection"
	httpErrorReasonUnknown           httpErrorReason = "unknown"
)

func proxyErrorReason(err error) httpErrorReason {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return httpErrorReasonTimeout
	case errors.Is(err, context.Canceled):
		return httpErrorReasonCanceled
	}

	if _, ok := errors.AsType[*net.DNSError](err); ok {
		return httpErrorReasonDNS
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return httpErrorReasonConnectionRefused
	}

	var recErr tls.RecordHeaderError
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok || errors.As(err, &recErr) {
		return httpErrorReasonTLS
	}

	if netErr, ok := errors.AsType[net.Error](err); ok && netErr.Timeout() {
		return httpErrorReasonTimeout
	}

	if _, ok := errors.AsType[*net.OpError](err); ok {
		return httpErrorReasonConnection
	}

	return httpErrorReasonUnknown
}

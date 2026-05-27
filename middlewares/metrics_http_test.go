package middlewares_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/middlewares"
)

// scrapeMetrics returns the prometheus exposition for the default registry
// via the Prometheus middleware. Asserting on the textual output lets the
// tests stay in the external _test package without poking at the unexported
// metric vectors.
func scrapeMetrics() string {
	promH := middlewares.Prometheus()(http.NotFoundHandler())
	rec := httptest.NewRecorder()
	promH.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_metrics", nil))
	return rec.Body.String()
}

var _ = Describe("MetricsHttp middleware", func() {
	It("records a request as a counter and a histogram sample", func() {
		// Unique path isolates this spec from any other test that touches
		// the global metric vectors.
		const path = "/middlewares-test-metrics-http-incr"

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})
		h := middlewares.MetricsHttp(nil)(next)

		Expect(scrapeMetrics()).NotTo(ContainSubstring(path),
			"unique path should not appear in registry before the request")

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		body := scrapeMetrics()
		Expect(body).To(ContainSubstring(fmt.Sprintf(
			`http_requests_total{method="GET",path="%s",status="201"} 1`, path)))
		Expect(body).To(ContainSubstring(fmt.Sprintf(
			`http_request_duration_seconds_count{method="GET",path="%s",status="201"} 1`, path)))
	})

	It("defaults status to 200 when the handler doesn't call WriteHeader", func() {
		const path = "/middlewares-test-metrics-http-default"

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, "ok")
		})
		h := middlewares.MetricsHttp(nil)(next)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))

		Expect(scrapeMetrics()).To(ContainSubstring(fmt.Sprintf(
			`http_requests_total{method="GET",path="%s",status="200"} 1`, path)))
	})

	It("labels by request method", func() {
		const path = "/middlewares-test-metrics-http-method"

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		h := middlewares.MetricsHttp(nil)(next)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, path, nil))

		body := scrapeMetrics()
		Expect(body).To(ContainSubstring(fmt.Sprintf(
			`http_requests_total{method="POST",path="%s",status="200"} 1`, path)))
		Expect(body).NotTo(ContainSubstring(fmt.Sprintf(
			`http_requests_total{method="GET",path="%s"`, path)))
	})
})

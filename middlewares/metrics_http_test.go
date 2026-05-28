package middlewares_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/ArthurHlt/rparth/testutils"
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
		// Unique route name isolates this spec from any other test that
		// touches the global metric vectors.
		const routeName = "middlewares-test-metrics-http-incr"

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})
		h := middlewares.MetricsHttp()(next)

		Expect(scrapeMetrics()).NotTo(ContainSubstring(routeName),
			"unique route name should not appear in registry before the request")

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/", routeName))

		body := scrapeMetrics()
		Expect(body).To(ContainSubstring(fmt.Sprintf(
			`http_requests_total{method="GET",route_name="%s",status="201"} 1`, routeName)))
		Expect(body).To(ContainSubstring(fmt.Sprintf(
			`http_request_duration_seconds_count{method="GET",route_name="%s",status="201"} 1`, routeName)))
	})

	It("defaults status to 200 when the handler doesn't call WriteHeader", func() {
		const routeName = "middlewares-test-metrics-http-default"

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprint(w, "ok")
		})
		h := middlewares.MetricsHttp()(next)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/", routeName))

		Expect(scrapeMetrics()).To(ContainSubstring(fmt.Sprintf(
			`http_requests_total{method="GET",route_name="%s",status="200"} 1`, routeName)))
	})

	It("labels by request method", func() {
		const routeName = "middlewares-test-metrics-http-method"

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		h := middlewares.MetricsHttp()(next)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodPost, "/", routeName))

		body := scrapeMetrics()
		Expect(body).To(ContainSubstring(fmt.Sprintf(
			`http_requests_total{method="POST",route_name="%s",status="200"} 1`, routeName)))
		Expect(body).NotTo(ContainSubstring(fmt.Sprintf(
			`http_requests_total{method="GET",route_name="%s"`, routeName)))
	})

	It("labels by route name pulled from the request context", func() {
		const routeName = "middlewares-test-metrics-http-route"

		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		h := middlewares.MetricsHttp()(next)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, testutils.RequestWithRoute(http.MethodGet, "/", routeName))

		Expect(scrapeMetrics()).To(ContainSubstring(fmt.Sprintf(
			`http_requests_total{method="GET",route_name="%s",status="200"} 1`, routeName)))
	})

	It("falls back to route_name=\"unknown\" when no route is in context", func() {
		// No contexts.SetRPRoute call — the middleware should label the
		// metric as "unknown" instead of leaking a per-path value (which
		// would blow up cardinality, the very reason for this refactor).
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		h := middlewares.MetricsHttp()(next)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/anything", nil))

		// "unknown" is shared across any spec that omits a route, so the
		// counter value is not pinned — just assert the labelled series exists.
		Expect(scrapeMetrics()).To(ContainSubstring(
			`http_requests_total{method="GET",route_name="unknown",status="200"}`))
	})
})

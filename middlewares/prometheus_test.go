package middlewares_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/middlewares"
)

var _ = Describe("Prometheus middleware", func() {
	It("serves the metrics exposition on /_metrics without calling next", func() {
		nextCalled := false
		next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
			nextCalled = true
		})

		h := middlewares.Prometheus()(next)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_metrics", nil))

		Expect(nextCalled).To(BeFalse())
		Expect(rec.Code).To(Equal(http.StatusOK))
		// promhttp.Handler exposes Go runtime metrics — assert on a known
		// always-present metric rather than the full payload so the test
		// stays stable across client_golang versions.
		Expect(rec.Body.String()).To(ContainSubstring("go_goroutines"))
	})

	It("passes through to next for any other path", func() {
		nextCalled := false
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusTeapot)
		})

		h := middlewares.Prometheus()(next)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/users", nil))

		Expect(nextCalled).To(BeTrue())
		Expect(rec.Code).To(Equal(http.StatusTeapot))
	})
})

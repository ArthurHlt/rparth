package middlewares_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/middlewares"
)

var _ = Describe("Chain", func() {
	// trace lets each middleware record an in/out marker so we can assert the
	// nesting order: the first middleware in the slice must be the outermost.
	tracer := func(trace *[]string, tag string) middlewares.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				*trace = append(*trace, fmt.Sprintf("%s:in", tag))
				next.ServeHTTP(w, r)
				*trace = append(*trace, fmt.Sprintf("%s:out", tag))
			})
		}
	}

	It("runs middlewares in declaration order (first is outermost)", func() {
		var trace []string
		final := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			trace = append(trace, "handler")
			w.WriteHeader(http.StatusOK)
		})

		h := middlewares.Chain(tracer(&trace, "a"), tracer(&trace, "b"), tracer(&trace, "c"))(final)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		Expect(trace).To(Equal([]string{
			"a:in", "b:in", "c:in",
			"handler",
			"c:out", "b:out", "a:out",
		}))
	})

	It("returns the underlying handler when no middlewares are given", func() {
		called := false
		final := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusTeapot)
		})

		h := middlewares.Chain()(final)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

		Expect(called).To(BeTrue())
		Expect(rec.Code).To(Equal(http.StatusTeapot))
	})
})

package middlewares_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/middlewares"
	"github.com/ArthurHlt/rparth/testutils"
)

// captureHandler stores every slog.Record it receives so tests can assert on
// attributes the middleware emits. It accepts all levels so the test doesn't
// have to track which level httplog picks for a given status.
type captureHandler struct {
	records []slog.Record
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.records = append(h.records, r)
	return nil
}
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

// findAttr walks a record's attributes, descending into groups, and returns
// the first value whose key matches. httplog's OTEL schema may bucket attrs
// into groups (split on ":"), so a recursive walk keeps the assertion robust
// to future schema layout changes.
func findAttr(r slog.Record, key string) (slog.Value, bool) {
	var found slog.Value
	var ok bool
	r.Attrs(func(a slog.Attr) bool {
		if v, hit := findInAttr(a, key); hit {
			found = v
			ok = true
			return false
		}
		return true
	})
	return found, ok
}

func findInAttr(a slog.Attr, key string) (slog.Value, bool) {
	if a.Key == key {
		return a.Value, true
	}
	if a.Value.Kind() == slog.KindGroup {
		for _, nested := range a.Value.Group() {
			if v, ok := findInAttr(nested, key); ok {
				return v, true
			}
		}
	}
	return slog.Value{}, false
}

var _ = Describe("AccessLog middleware", func() {
	var (
		captured    *captureHandler
		prevDefault *slog.Logger
	)

	BeforeEach(func() {
		// AccessLog captures slog.Default() at construction time, so the
		// swap has to happen before middlewares.AccessLog(...) is called.
		captured = &captureHandler{}
		prevDefault = slog.Default()
		slog.SetDefault(slog.New(captured))
	})

	AfterEach(func() {
		slog.SetDefault(prevDefault)
	})

	It("emits route_name from the route stored in the request context", func() {
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		h := middlewares.AccessLog(slog.LevelInfo)(next)

		req := testutils.RequestWithRoute(http.MethodGet, "/", "access-log-test-route")
		h.ServeHTTP(httptest.NewRecorder(), req)

		Expect(captured.records).To(HaveLen(1))
		v, ok := findAttr(captured.records[0], "route_name")
		Expect(ok).To(BeTrue(), "record should carry a route_name attr")
		Expect(v.String()).To(Equal("access-log-test-route"))
	})

	It("falls back to route_name=\"unknown\" when no route is in context", func() {
		next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		h := middlewares.AccessLog(slog.LevelInfo)(next)

		req := httptest.NewRequest(http.MethodGet, "/anything", nil)
		h.ServeHTTP(httptest.NewRecorder(), req)

		Expect(captured.records).To(HaveLen(1))
		v, ok := findAttr(captured.records[0], "route_name")
		Expect(ok).To(BeTrue(), "record should carry a route_name attr even without a matched route")
		Expect(v.String()).To(Equal("unknown"))
	})
})

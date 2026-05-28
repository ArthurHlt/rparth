package proxy_test

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/ArthurHlt/rparth/testutils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/ArthurHlt/rparth/contexts"
	"github.com/ArthurHlt/rparth/models"
	"github.com/ArthurHlt/rparth/proxy"
	"github.com/ArthurHlt/rparth/proxy/mocks"
)

// timeoutNetError is a net.Error whose Timeout() reports true, used to exercise
// the non-context timeout branch of the proxy error classifier.
type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return false }

func newResponse(status int, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newRequest(host, path string, header http.Header) *http.Request {
	if header == nil {
		header = http.Header{}
	}
	return &http.Request{
		Method: http.MethodGet,
		Host:   host,
		Header: header,
		URL:    &url.URL{Path: path},
	}
}

// serve drives the request through MarkRPRouteRequest before ServeHTTP, the
// way app.RunServer wires it.
func serve(p *proxy.Proxy, w http.ResponseWriter, req *http.Request) {
	p.MarkRPRouteRequest()(p).ServeHTTP(w, req)
}

var _ = Describe("Proxy.ServeHTTP", func() {
	var (
		transport    *mocks.MockRoundTripper
		received     *http.Request
		response     *http.Response
		transportErr error
		apiRoute     *models.RPRoute
		routes       models.RPRoutes
		p            *proxy.Proxy
		w            *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		apiRoute = &models.RPRoute{
			Name:   "api",
			Host:   "api.example.com",
			Prefix: "/",
			Target: testutils.MustYamlParseURL("http://api-backend:9000"),
		}
		routes = models.RPRoutes{apiRoute}
		received = nil
		transportErr = nil
		response = newResponse(http.StatusOK, "hello", http.Header{
			"Content-Type": []string{"text/plain"},
		})
		transport = mocks.NewMockRoundTripper(gomock.NewController(GinkgoT()))
		transport.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
			func(req *http.Request) (*http.Response, error) {
				received = req
				if transportErr != nil {
					return nil, transportErr
				}
				return response, nil
			},
		).AnyTimes()
		p = proxy.NewProxy(transport, routes)
		w = httptest.NewRecorder()
	})

	Describe("happy path", func() {
		It("returns the upstream status, headers, and body", func() {
			req := newRequest("api.example.com:80", "/users/42", nil)
			serve(p, w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Header().Get("Content-Type")).To(Equal("text/plain"))
			Expect(w.Body.String()).To(Equal("hello"))
		})

		It("rewrites the forwarded URL scheme and host to the route target", func() {
			req := newRequest("api.example.com:80", "/users/42", nil)
			serve(p, w, req)

			Expect(received).NotTo(BeNil())
			Expect(received.URL.Scheme).To(Equal("http"))
			Expect(received.URL.Host).To(Equal("api-backend:9000"))
			Expect(received.URL.Path).To(Equal("/users/42"))
			// http.Transport derives the wire-level Host: header from req.Host
			// (the field), not from req.Header["Host"]. Asserting it here guards
			// against virtual-hosted backends seeing the client-facing host.
			Expect(received.Host).To(Equal("api-backend:9000"))
		})

		It("does not mutate the original request URL", func() {
			req := newRequest("api.example.com:80", "/users/42", nil)
			serve(p, w, req)

			Expect(req.URL.Host).To(BeEmpty())
			Expect(req.URL.Scheme).To(BeEmpty())
		})
	})

	Describe("error paths", func() {
		It("returns 404 when no route matches", func() {
			req := newRequest("unknown.example.com:80", "/", nil)
			serve(p, w, req)

			Expect(w.Code).To(Equal(http.StatusNotFound))
			Expect(w.Body.String()).To(ContainSubstring("no route found"))
			Expect(received).To(BeNil())
		})

		It("returns 502 when the upstream transport errors", func() {
			transportErr = errors.New("dial failed")
			req := newRequest("api.example.com:80", "/", nil)
			serve(p, w, req)

			Expect(w.Code).To(Equal(http.StatusBadGateway))
			Expect(w.Body.String()).To(ContainSubstring("dial failed"))
		})

		DescribeTable("classifies upstream transport errors into http_proxy_errors_total reasons",
			func(rtErr error, reason string) {
				labels := map[string]string{"route_name": "api", "method": http.MethodGet, "reason": reason}
				before := testutils.MetricValue("http_proxy_errors_total", labels)

				transportErr = rtErr
				serve(p, w, newRequest("api.example.com:80", "/", nil))

				Expect(w.Code).To(Equal(http.StatusBadGateway))
				Expect(testutils.MetricValue("http_proxy_errors_total", labels) - before).To(Equal(float64(1)))
			},
			Entry("context deadline", context.DeadlineExceeded, "timeout"),
			Entry("context canceled", context.Canceled, "canceled"),
			Entry("DNS failure", &net.DNSError{Err: "no such host", Name: "api-backend"}, "dns"),
			Entry("connection refused", &net.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)}, "connection_refused"),
			Entry("TLS verification", &tls.CertificateVerificationError{Err: errors.New("bad cert")}, "tls"),
			Entry("net timeout", &net.OpError{Op: "read", Err: timeoutNetError{}}, "timeout"),
			Entry("generic network error", &net.OpError{Op: "read", Err: errors.New("connection reset")}, "connection"),
			Entry("opaque error", errors.New("boom"), "unknown"),
		)
	})

	Describe("hop-by-hop header sanitization", func() {
		It("strips standard hop-by-hop headers from the forwarded request", func() {
			req := newRequest("api.example.com:80", "/", http.Header{
				"Keep-Alive":        []string{"timeout=5"},
				"Transfer-Encoding": []string{"chunked"},
				"Upgrade":           []string{"websocket"},
				"X-Custom-Tracking": []string{"abc"},
			})
			serve(p, w, req)

			h := received.Header
			Expect(h.Get("Keep-Alive")).To(BeEmpty())
			Expect(h.Get("Transfer-Encoding")).To(BeEmpty())
			Expect(h.Get("Upgrade")).To(BeEmpty())
			// non-hop-by-hop headers are preserved
			Expect(h.Get("X-Custom-Tracking")).To(Equal("abc"))
		})

		It("strips headers named in the request Connection header", func() {
			req := newRequest("api.example.com:80", "/", http.Header{
				"Connection":    []string{"X-Custom-Hop, X-Another"},
				"X-Custom-Hop":  []string{"value1"},
				"X-Another":     []string{"value2"},
				"X-Should-Stay": []string{"value3"},
			})
			serve(p, w, req)

			h := received.Header
			Expect(h.Get("Connection")).To(BeEmpty())
			Expect(h.Get("X-Custom-Hop")).To(BeEmpty())
			Expect(h.Get("X-Another")).To(BeEmpty())
			Expect(h.Get("X-Should-Stay")).To(Equal("value3"))
		})

		It("strips standard hop-by-hop headers from the response", func() {
			response = newResponse(http.StatusOK, "ok", http.Header{
				"Keep-Alive":        []string{"timeout=5"},
				"Transfer-Encoding": []string{"chunked"},
				"Content-Type":      []string{"text/plain"},
			})
			req := newRequest("api.example.com:80", "/", nil)
			serve(p, w, req)

			Expect(w.Header().Get("Keep-Alive")).To(BeEmpty())
			Expect(w.Header().Get("Transfer-Encoding")).To(BeEmpty())
			Expect(w.Header().Get("Content-Type")).To(Equal("text/plain"))
		})

		It("strips headers named in the response Connection header", func() {
			response = newResponse(http.StatusOK, "ok", http.Header{
				"Connection": []string{"X-Resp-Hop"},
				"X-Resp-Hop": []string{"secret"},
				"X-Stay":     []string{"public"},
			})
			req := newRequest("api.example.com:80", "/", nil)
			serve(p, w, req)

			Expect(w.Header().Get("Connection")).To(BeEmpty())
			Expect(w.Header().Get("X-Resp-Hop")).To(BeEmpty())
			Expect(w.Header().Get("X-Stay")).To(Equal("public"))
		})
	})

	Describe("route-specific headers", func() {
		BeforeEach(func() {
			apiRoute.Headers = map[string][]string{
				"X-Forwarded-By": {"rparth"},
				"X-Tenant":       {"acme"},
			}
		})

		It("injects route headers into the forwarded request", func() {
			req := newRequest("api.example.com:80", "/", nil)
			serve(p, w, req)

			h := received.Header
			Expect(h.Get("X-Forwarded-By")).To(Equal("rparth"))
			Expect(h.Get("X-Tenant")).To(Equal("acme"))
		})

		It("overrides client-supplied values with route headers", func() {
			req := newRequest("api.example.com:80", "/", http.Header{
				"X-Tenant": []string{"attacker"},
			})
			serve(p, w, req)

			Expect(received.Header.Get("X-Tenant")).To(Equal("acme"))
		})
	})

	Describe("routing across multiple routes", func() {
		var webRoute *models.RPRoute

		BeforeEach(func() {
			webRoute = &models.RPRoute{
				Name:   "web",
				Host:   "web.example.com",
				Prefix: "/",
				Target: testutils.MustYamlParseURL("http://web-backend:7000"),
			}
			routes = models.RPRoutes{apiRoute, webRoute}
			p = proxy.NewProxy(transport, routes)
		})

		It("dispatches to the route matching the request host", func() {
			req := newRequest("web.example.com:80", "/dashboard", nil)
			serve(p, w, req)

			Expect(received.URL.Host).To(Equal("web-backend:7000"))
			Expect(received.URL.Path).To(Equal("/dashboard"))
		})
	})

	Describe("proxy-set forwarding headers", func() {
		It("sets X-Forwarded-For to the client IP parsed from RemoteAddr", func() {
			req := newRequest("api.example.com:80", "/", nil)
			req.RemoteAddr = "203.0.113.7:54321"
			serve(p, w, req)

			Expect(received.Header.Get("X-Forwarded-For")).To(Equal("203.0.113.7"))
		})

		It("falls back to the raw RemoteAddr when it has no port", func() {
			req := newRequest("api.example.com:80", "/", nil)
			req.RemoteAddr = "203.0.113.7"
			serve(p, w, req)

			Expect(received.Header.Get("X-Forwarded-For")).To(Equal("203.0.113.7"))
		})

		It("appends the client IP to an existing X-Forwarded-For chain", func() {
			req := newRequest("api.example.com:80", "/", http.Header{
				"X-Forwarded-For": []string{"198.51.100.1, 198.51.100.2"},
			})
			req.RemoteAddr = "203.0.113.7:54321"
			serve(p, w, req)

			Expect(received.Header.Get("X-Forwarded-For")).
				To(Equal("198.51.100.1, 198.51.100.2, 203.0.113.7"))
		})

		It("sets X-Forwarded-Scheme to http for plain requests", func() {
			req := newRequest("api.example.com:80", "/", nil)
			req.RemoteAddr = "1.2.3.4:80"
			serve(p, w, req)

			Expect(received.Header.Get("X-Forwarded-Scheme")).To(Equal("http"))
		})

		It("sets X-Forwarded-Scheme to https when req.TLS is set", func() {
			req := newRequest("api.example.com:80", "/", nil)
			req.RemoteAddr = "1.2.3.4:80"
			req.TLS = &tls.ConnectionState{}
			serve(p, w, req)

			Expect(received.Header.Get("X-Forwarded-Scheme")).To(Equal("https"))
		})

		It("sets X-Forwarded-Host to the request Host field", func() {
			req := newRequest("api.example.com:80", "/", nil)
			req.RemoteAddr = "1.2.3.4:80"
			serve(p, w, req)

			Expect(received.Header.Get("X-Forwarded-Host")).To(Equal("api.example.com:80"))
		})

		It("attaches the forwarding headers to the upstream request, not the client response", func() {
			req := newRequest("api.example.com:80", "/", nil)
			req.RemoteAddr = "203.0.113.7:54321"
			serve(p, w, req)

			Expect(received.Header.Get("X-Forwarded-For")).To(Equal("203.0.113.7"))
			Expect(received.Header.Get("X-Forwarded-Scheme")).To(Equal("http"))
			Expect(received.Header.Get("X-Forwarded-Host")).To(Equal("api.example.com:80"))

			Expect(w.Header().Get("X-Forwarded-For")).To(BeEmpty())
			Expect(w.Header().Get("X-Forwarded-Scheme")).To(BeEmpty())
			Expect(w.Header().Get("X-Forwarded-Host")).To(BeEmpty())
		})

		Describe("RFC 7239 Forwarded header", func() {
			It("emits a single 'for' entry with by/host/proto for a fresh request", func() {
				req := newRequest("api.example.com:80", "/", nil)
				req.RemoteAddr = "203.0.113.7:54321"
				serve(p, w, req)

				Expect(received.Header.Get("Forwarded")).To(Equal(
					"by=rparth;for=203.0.113.7;host=api.example.com:80;proto=http",
				))
			})

			It("only describes the current hop in Forwarded (chain stays in X-Forwarded-For)", func() {
				req := newRequest("api.example.com:80", "/", http.Header{
					"X-Forwarded-For": []string{"198.51.100.1, 198.51.100.2"},
				})
				req.RemoteAddr = "203.0.113.7:54321"
				serve(p, w, req)

				// Per RFC 7239, each proxy adds one Forwarded element for itself;
				// the upstream chain is conveyed via X-Forwarded-For.
				Expect(received.Header.Get("Forwarded")).To(Equal(
					"by=rparth;for=203.0.113.7;host=api.example.com:80;proto=http",
				))
			})

			It("uses proto=https when req.TLS is set", func() {
				req := newRequest("api.example.com:80", "/", nil)
				req.RemoteAddr = "203.0.113.7:54321"
				req.TLS = &tls.ConnectionState{}
				serve(p, w, req)

				Expect(received.Header.Get("Forwarded")).To(Equal(
					"by=rparth;for=203.0.113.7;host=api.example.com:80;proto=https",
				))
			})
		})
	})

	Describe("StripPrefix", func() {
		var trueVal, falseVal = true, false

		It("strips the matched prefix from the upstream URL.Path when enabled", func() {
			apiRoute.Prefix = "/api"
			apiRoute.StripPrefix = &trueVal

			req := newRequest("api.example.com:80", "/api/users/42", nil)
			serve(p, w, req)

			Expect(received.URL.Path).To(Equal("/users/42"))
		})

		It("does not strip when StripPrefix is nil (unset)", func() {
			apiRoute.Prefix = "/api"
			apiRoute.StripPrefix = nil

			req := newRequest("api.example.com:80", "/api/users/42", nil)
			serve(p, w, req)

			Expect(received.URL.Path).To(Equal("/api/users/42"))
		})

		It("does not strip when StripPrefix is explicitly false", func() {
			apiRoute.Prefix = "/api"
			apiRoute.StripPrefix = &falseVal

			req := newRequest("api.example.com:80", "/api/users/42", nil)
			serve(p, w, req)

			Expect(received.URL.Path).To(Equal("/api/users/42"))
		})

		It("does not strip when Prefix is '/' even with StripPrefix=true (would empty the path)", func() {
			apiRoute.Prefix = "/"
			apiRoute.StripPrefix = &trueVal

			req := newRequest("api.example.com:80", "/users/42", nil)
			serve(p, w, req)

			Expect(received.URL.Path).To(Equal("/users/42"))
		})
	})

	Describe("route timeout", func() {
		It("attaches a deadline reflecting route.Timeout to the forwarded request's context", func() {
			apiRoute.Timeout = 5
			req := newRequest("api.example.com:80", "/", nil)

			before := time.Now()
			serve(p, w, req)

			deadline, ok := received.Context().Deadline()
			Expect(ok).To(BeTrue())
			// deadline should sit ~5s in the future, allowing scheduling slop
			Expect(deadline).To(BeTemporally("~", before.Add(5*time.Second), 500*time.Millisecond))
		})

		It("does not attach a deadline when route.Timeout is 0 (timeout disabled)", func() {
			apiRoute.Timeout = 0
			req := newRequest("api.example.com:80", "/", nil)

			serve(p, w, req)

			_, ok := received.Context().Deadline()
			Expect(ok).To(BeFalse())
		})
	})
})

var _ = Describe("Proxy.MarkRPRouteRequest middleware", func() {
	var (
		apiRoute *models.RPRoute
		webRoute *models.RPRoute
		p        *proxy.Proxy
		w        *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		apiRoute = &models.RPRoute{
			Name:   "api",
			Host:   "api.example.com",
			Prefix: "/",
			Target: testutils.MustYamlParseURL("http://api-backend:9000"),
		}
		webRoute = &models.RPRoute{
			Name:   "web",
			Host:   "web.example.com",
			Prefix: "/",
			Target: testutils.MustYamlParseURL("http://web-backend:7000"),
		}
		// The transport is irrelevant here — the middleware never reaches it, so
		// the mock carries no expectations.
		p = proxy.NewProxy(mocks.NewMockRoundTripper(gomock.NewController(GinkgoT())), models.RPRoutes{apiRoute, webRoute})
		w = httptest.NewRecorder()
	})

	It("calls next without a route in context when no route matches", func() {
		// Critical for observability: even unmatched requests must reach the
		// access-log and metrics middlewares (which then label them as
		// route_name="unknown"). The middleware must not short-circuit here.
		nextCalled := false
		var seen *models.RPRoute
		next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			nextCalled = true
			seen = contexts.GetRPRoute(r)
		})

		req := newRequest("nowhere.example.com:80", "/", nil)
		p.MarkRPRouteRequest()(next).ServeHTTP(w, req)

		Expect(nextCalled).To(BeTrue())
		Expect(seen).To(BeNil())
	})
})

package proxy_test

import (
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/models"
	"github.com/ArthurHlt/rparth/proxy"
)


// fakeTransport captures the request it receives and returns a programmable response (or error).
type fakeTransport struct {
	received *http.Request
	response *http.Response
	err      error
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.received = req
	if f.err != nil {
		return nil, f.err
	}
	return f.response, nil
}

func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	Expect(err).NotTo(HaveOccurred())
	return u
}

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

var _ = Describe("Proxy.ServeHTTP", func() {
	var (
		transport *fakeTransport
		apiRoute  *models.RPRoute
		routes    models.RPRoutes
		p         *proxy.Proxy
		w         *httptest.ResponseRecorder
	)

	BeforeEach(func() {
		apiRoute = &models.RPRoute{
			Name:   "api",
			Host:   "api.example.com",
			Prefix: "/",
			Target: mustParseURL("http://api-backend:9000"),
		}
		routes = models.RPRoutes{apiRoute}
		transport = &fakeTransport{
			response: newResponse(http.StatusOK, "hello", http.Header{
				"Content-Type": []string{"text/plain"},
			}),
		}
		p = proxy.NewProxy(transport, routes)
		w = httptest.NewRecorder()
	})

	Describe("happy path", func() {
		It("returns the upstream status, headers, and body", func() {
			req := newRequest("api.example.com:80", "/users/42", nil)
			p.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Header().Get("Content-Type")).To(Equal("text/plain"))
			Expect(w.Body.String()).To(Equal("hello"))
		})

		It("rewrites the forwarded URL scheme and host to the route target", func() {
			req := newRequest("api.example.com:80", "/users/42", nil)
			p.ServeHTTP(w, req)

			Expect(transport.received).NotTo(BeNil())
			Expect(transport.received.URL.Scheme).To(Equal("http"))
			Expect(transport.received.URL.Host).To(Equal("api-backend:9000"))
			Expect(transport.received.URL.Path).To(Equal("/users/42"))
			// http.Transport derives the wire-level Host: header from req.Host
			// (the field), not from req.Header["Host"]. Asserting it here guards
			// against virtual-hosted backends seeing the client-facing host.
			Expect(transport.received.Host).To(Equal("api-backend:9000"))
		})

		It("does not mutate the original request URL", func() {
			req := newRequest("api.example.com:80", "/users/42", nil)
			p.ServeHTTP(w, req)

			Expect(req.URL.Host).To(BeEmpty())
			Expect(req.URL.Scheme).To(BeEmpty())
		})
	})

	Describe("error paths", func() {
		It("returns 500 when no route matches", func() {
			req := newRequest("unknown.example.com:80", "/", nil)
			p.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusInternalServerError))
			Expect(w.Body.String()).To(ContainSubstring("no route found"))
			Expect(transport.received).To(BeNil())
		})

		It("returns 500 when the upstream transport errors", func() {
			transport.err = errors.New("dial failed")
			req := newRequest("api.example.com:80", "/", nil)
			p.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusInternalServerError))
			Expect(w.Body.String()).To(ContainSubstring("dial failed"))
		})
	})

	Describe("hop-by-hop header sanitization", func() {
		It("strips standard hop-by-hop headers from the forwarded request", func() {
			req := newRequest("api.example.com:80", "/", http.Header{
				"Keep-Alive":        []string{"timeout=5"},
				"Transfer-Encoding": []string{"chunked"},
				"Upgrade":           []string{"websocket"},
				"X-Custom-Tracking": []string{"abc"},
			})
			p.ServeHTTP(w, req)

			h := transport.received.Header
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
			p.ServeHTTP(w, req)

			h := transport.received.Header
			Expect(h.Get("Connection")).To(BeEmpty())
			Expect(h.Get("X-Custom-Hop")).To(BeEmpty())
			Expect(h.Get("X-Another")).To(BeEmpty())
			Expect(h.Get("X-Should-Stay")).To(Equal("value3"))
		})

		It("strips standard hop-by-hop headers from the response", func() {
			transport.response = newResponse(http.StatusOK, "ok", http.Header{
				"Keep-Alive":        []string{"timeout=5"},
				"Transfer-Encoding": []string{"chunked"},
				"Content-Type":      []string{"text/plain"},
			})
			req := newRequest("api.example.com:80", "/", nil)
			p.ServeHTTP(w, req)

			Expect(w.Header().Get("Keep-Alive")).To(BeEmpty())
			Expect(w.Header().Get("Transfer-Encoding")).To(BeEmpty())
			Expect(w.Header().Get("Content-Type")).To(Equal("text/plain"))
		})

		It("strips headers named in the response Connection header", func() {
			transport.response = newResponse(http.StatusOK, "ok", http.Header{
				"Connection": []string{"X-Resp-Hop"},
				"X-Resp-Hop": []string{"secret"},
				"X-Stay":     []string{"public"},
			})
			req := newRequest("api.example.com:80", "/", nil)
			p.ServeHTTP(w, req)

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
			p.ServeHTTP(w, req)

			h := transport.received.Header
			Expect(h.Get("X-Forwarded-By")).To(Equal("rparth"))
			Expect(h.Get("X-Tenant")).To(Equal("acme"))
		})

		It("overrides client-supplied values with route headers", func() {
			req := newRequest("api.example.com:80", "/", http.Header{
				"X-Tenant": []string{"attacker"},
			})
			p.ServeHTTP(w, req)

			Expect(transport.received.Header.Get("X-Tenant")).To(Equal("acme"))
		})
	})

	Describe("routing across multiple routes", func() {
		var webRoute *models.RPRoute

		BeforeEach(func() {
			webRoute = &models.RPRoute{
				Name:   "web",
				Host:   "web.example.com",
				Prefix: "/",
				Target: mustParseURL("http://web-backend:7000"),
			}
			routes = models.RPRoutes{apiRoute, webRoute}
			p = proxy.NewProxy(transport, routes)
		})

		It("dispatches to the route matching the request host", func() {
			req := newRequest("web.example.com:80", "/dashboard", nil)
			p.ServeHTTP(w, req)

			Expect(transport.received.URL.Host).To(Equal("web-backend:7000"))
			Expect(transport.received.URL.Path).To(Equal("/dashboard"))
		})

		It("does not invoke the transport when no route matches", func() {
			req := newRequest("nowhere.example.com:80", "/", nil)
			p.ServeHTTP(w, req)

			Expect(transport.received).To(BeNil())
			Expect(w.Code).To(Equal(http.StatusInternalServerError))
		})
	})

	Describe("proxy-set forwarding headers", func() {
		It("sets X-Forwarded-For to the client IP parsed from RemoteAddr", func() {
			req := newRequest("api.example.com:80", "/", nil)
			req.RemoteAddr = "203.0.113.7:54321"
			p.ServeHTTP(w, req)

			Expect(transport.received.Header.Get("X-Forwarded-For")).To(Equal("203.0.113.7"))
		})

		It("falls back to the raw RemoteAddr when it has no port", func() {
			req := newRequest("api.example.com:80", "/", nil)
			req.RemoteAddr = "203.0.113.7"
			p.ServeHTTP(w, req)

			Expect(transport.received.Header.Get("X-Forwarded-For")).To(Equal("203.0.113.7"))
		})

		It("appends the client IP to an existing X-Forwarded-For chain", func() {
			req := newRequest("api.example.com:80", "/", http.Header{
				"X-Forwarded-For": []string{"198.51.100.1, 198.51.100.2"},
			})
			req.RemoteAddr = "203.0.113.7:54321"
			p.ServeHTTP(w, req)

			Expect(transport.received.Header.Get("X-Forwarded-For")).
				To(Equal("198.51.100.1, 198.51.100.2, 203.0.113.7"))
		})

		It("sets X-Forwarded-Scheme to http for plain requests", func() {
			req := newRequest("api.example.com:80", "/", nil)
			req.RemoteAddr = "1.2.3.4:80"
			p.ServeHTTP(w, req)

			Expect(transport.received.Header.Get("X-Forwarded-Scheme")).To(Equal("http"))
		})

		It("sets X-Forwarded-Scheme to https when req.TLS is set", func() {
			req := newRequest("api.example.com:80", "/", nil)
			req.RemoteAddr = "1.2.3.4:80"
			req.TLS = &tls.ConnectionState{}
			p.ServeHTTP(w, req)

			Expect(transport.received.Header.Get("X-Forwarded-Scheme")).To(Equal("https"))
		})

		It("sets X-Forwarded-Host to the request Host field", func() {
			req := newRequest("api.example.com:80", "/", nil)
			req.RemoteAddr = "1.2.3.4:80"
			p.ServeHTTP(w, req)

			Expect(transport.received.Header.Get("X-Forwarded-Host")).To(Equal("api.example.com:80"))
		})

		It("attaches the forwarding headers to the upstream request, not the client response", func() {
			req := newRequest("api.example.com:80", "/", nil)
			req.RemoteAddr = "203.0.113.7:54321"
			p.ServeHTTP(w, req)

			Expect(transport.received.Header.Get("X-Forwarded-For")).To(Equal("203.0.113.7"))
			Expect(transport.received.Header.Get("X-Forwarded-Scheme")).To(Equal("http"))
			Expect(transport.received.Header.Get("X-Forwarded-Host")).To(Equal("api.example.com:80"))

			Expect(w.Header().Get("X-Forwarded-For")).To(BeEmpty())
			Expect(w.Header().Get("X-Forwarded-Scheme")).To(BeEmpty())
			Expect(w.Header().Get("X-Forwarded-Host")).To(BeEmpty())
		})

		Describe("RFC 7239 Forwarded header", func() {
			It("emits a single 'for' entry with by/host/proto for a fresh request", func() {
				req := newRequest("api.example.com:80", "/", nil)
				req.RemoteAddr = "203.0.113.7:54321"
				p.ServeHTTP(w, req)

				Expect(transport.received.Header.Get("Forwarded")).To(Equal(
					"by=rparth;for=203.0.113.7;host=api.example.com:80;proto=http",
				))
			})

			It("only describes the current hop in Forwarded (chain stays in X-Forwarded-For)", func() {
				req := newRequest("api.example.com:80", "/", http.Header{
					"X-Forwarded-For": []string{"198.51.100.1, 198.51.100.2"},
				})
				req.RemoteAddr = "203.0.113.7:54321"
				p.ServeHTTP(w, req)

				// Per RFC 7239, each proxy adds one Forwarded element for itself;
				// the upstream chain is conveyed via X-Forwarded-For.
				Expect(transport.received.Header.Get("Forwarded")).To(Equal(
					"by=rparth;for=203.0.113.7;host=api.example.com:80;proto=http",
				))
			})

			It("uses proto=https when req.TLS is set", func() {
				req := newRequest("api.example.com:80", "/", nil)
				req.RemoteAddr = "203.0.113.7:54321"
				req.TLS = &tls.ConnectionState{}
				p.ServeHTTP(w, req)

				Expect(transport.received.Header.Get("Forwarded")).To(Equal(
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
			p.ServeHTTP(w, req)

			Expect(transport.received.URL.Path).To(Equal("/users/42"))
		})

		It("does not strip when StripPrefix is nil (unset)", func() {
			apiRoute.Prefix = "/api"
			apiRoute.StripPrefix = nil

			req := newRequest("api.example.com:80", "/api/users/42", nil)
			p.ServeHTTP(w, req)

			Expect(transport.received.URL.Path).To(Equal("/api/users/42"))
		})

		It("does not strip when StripPrefix is explicitly false", func() {
			apiRoute.Prefix = "/api"
			apiRoute.StripPrefix = &falseVal

			req := newRequest("api.example.com:80", "/api/users/42", nil)
			p.ServeHTTP(w, req)

			Expect(transport.received.URL.Path).To(Equal("/api/users/42"))
		})

		It("does not strip when Prefix is '/' even with StripPrefix=true (would empty the path)", func() {
			apiRoute.Prefix = "/"
			apiRoute.StripPrefix = &trueVal

			req := newRequest("api.example.com:80", "/users/42", nil)
			p.ServeHTTP(w, req)

			Expect(transport.received.URL.Path).To(Equal("/users/42"))
		})
	})

	Describe("route timeout", func() {
		It("attaches a deadline reflecting route.Timeout to the forwarded request's context", func() {
			apiRoute.Timeout = 5
			req := newRequest("api.example.com:80", "/", nil)

			before := time.Now()
			p.ServeHTTP(w, req)

			deadline, ok := transport.received.Context().Deadline()
			Expect(ok).To(BeTrue())
			// deadline should sit ~5s in the future, allowing scheduling slop
			Expect(deadline).To(BeTemporally("~", before.Add(5*time.Second), 500*time.Millisecond))
		})

		It("does not attach a deadline when route.Timeout is 0 (timeout disabled)", func() {
			apiRoute.Timeout = 0
			req := newRequest("api.example.com:80", "/", nil)

			p.ServeHTTP(w, req)

			_, ok := transport.received.Context().Deadline()
			Expect(ok).To(BeFalse())
		})

		It("uses each route's own Timeout when several are configured", func() {
			webRoute := &models.RPRoute{
				Name:    "web",
				Host:    "web.example.com",
				Prefix:  "/",
				Target:  mustParseURL("http://web-backend:7000"),
				Timeout: 7,
			}
			apiRoute.Timeout = 3
			routes = models.RPRoutes{apiRoute, webRoute}
			p = proxy.NewProxy(transport, routes)

			before := time.Now()
			req := newRequest("web.example.com:80", "/dashboard", nil)
			p.ServeHTTP(w, req)

			deadline, ok := transport.received.Context().Deadline()
			Expect(ok).To(BeTrue())
			Expect(deadline).To(BeTemporally("~", before.Add(7*time.Second), 500*time.Millisecond))
		})
	})
})

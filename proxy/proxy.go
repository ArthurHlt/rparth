package proxy

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ArthurHlt/rparth/models"
)

const byName = "rparth"

// init initializes the hopByHopHeaders variable.
// maybe a little bit overengineered, but it's because when filtering hop by hop header
// using getHeadersFromConnection, i don't want to browse a list in O(n) each time a request coming
// i prefer have a map with O(1) lookup for hop by hop headers
func init() {
	for header := range hopByHopHeadersMap {
		hopByHopHeaders = append(hopByHopHeaders, header)
	}
}

// use a struct as value because it's 0 in memory
var hopByHopHeadersMap = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailers":            {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

var hopByHopHeaders []string

type Proxy struct {
	rpRoutes  models.RPRoutes
	transport http.RoundTripper
}

func NewProxy(transport http.RoundTripper, rpRoutes models.RPRoutes) *Proxy {
	return &Proxy{
		rpRoutes:  rpRoutes,
		transport: transport,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	slog.Debug("start request received", "method", req.Method, "url", req.URL.String())
	resp, err := p.roundTrip(req)
	if err != nil {
		slog.Error("error proxying request", "err", err, "url", req.URL.String(), "method", req.Method)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		slog.Error("error writing response", "err", err, "url", req.URL.String(), "method", req.Method)
		return
	}
	slog.Debug("request finished", "url", req.URL.String(), "method", req.Method)
}

func (p *Proxy) roundTrip(req *http.Request) (*http.Response, error) {
	route, err := p.rpRoutes.FindRoute(req)
	if err != nil {
		return nil, err
	}
	slog.Debug(
		"route found",
		"route", route,
		"method", req.Method,
		"url", req.URL,
	)
	// this will how we handle timeout set by the user
	// if 0 we don't need to set a timeout
	ctx := req.Context()
	if route.Timeout != 0 {
		var cancelFunc context.CancelFunc
		ctx, cancelFunc = context.WithTimeout(ctx, time.Duration(route.Timeout)*time.Second)
		defer cancelFunc()
	}
	reqProxy := req.Clone(ctx)
	reqProxy.Host = route.Target.Host
	reqProxy.URL.Host = route.Target.Host
	reqProxy.URL.Scheme = route.Target.Scheme
	reqProxy.Header = p.sanitizeHopByHopHeaders(reqProxy.Header)
	if route.StripPrefix != nil && *route.StripPrefix && route.Prefix != "/" {
		reqProxy.URL.Path = strings.TrimPrefix(reqProxy.URL.Path, route.Prefix)
	}
	for k, v := range route.Headers {
		reqProxy.Header[k] = v
	}
	proxyHeaders := p.proxyHeaders(req)
	for k, v := range proxyHeaders {
		reqProxy.Header[k] = v
	}
	slog.Debug(
		"starting proxying request ...",
		"url", reqProxy.URL.String(),
		"method", reqProxy.Method,
		"route", route,
	)
	resp, err := p.transport.RoundTrip(reqProxy)
	if err != nil {
		slog.Error(
			"error proxying request",
			"err", err,
			"url", reqProxy.URL.String(),
			"method", reqProxy.Method,
			"route", route,
		)
		return nil, err
	}
	resp.Header = p.sanitizeHopByHopHeaders(resp.Header)
	slog.Debug("finished proxying request.",
		"url", reqProxy.URL.String(),
		"method", reqProxy.Method,
		"route", route,
	)
	return resp, nil
}

func (p *Proxy) sanitizeHopByHopHeaders(headers http.Header) http.Header {
	connection := headers.Get("Connection")
	headersToSan := make([]string, len(hopByHopHeaders))
	copy(headersToSan, hopByHopHeaders)
	headersToSan = append(headersToSan, p.getHeadersFromConnection(connection)...)
	for _, header := range headersToSan {
		headers.Del(header)
	}
	return headers
}

func (p *Proxy) getHeadersFromConnection(connections string) []string {
	if connections == "" {
		return []string{}
	}
	connHeaders := strings.Split(connections, ",")
	headersToSan := make([]string, 0)
	for _, header := range connHeaders {
		header = strings.TrimSpace(header)
		_, exists := hopByHopHeadersMap[header]
		if exists {
			continue
		}
		headersToSan = append(headersToSan, header)
	}
	return headersToSan
}

func (p *Proxy) proxyHeaders(req *http.Request) http.Header {
	forwardedScheme := p.getForwardedScheme(req)
	host := req.Host
	forwardedFor := p.getForwardedFor(req)
	headers := make(http.Header)
	headers.Set("X-Forwarded-For", strings.Join(forwardedFor, ", "))
	headers.Set("X-Forwarded-Scheme", forwardedScheme)
	headers.Set("X-Forwarded-Host", host)
	clientIPRfc := p.getClientIP(req)
	// if the client IP is an IPv6 address, we need to wrap it in square brackets and add quotes as requested by rfc
	if strings.Contains(clientIPRfc, ":") {
		clientIPRfc = fmt.Sprintf(`"[%s]"`, clientIPRfc)
	}

	// chaining multiple proxy in forwarded header is a little painful and not really the exercise
	// so i've decided to make it simple and show we can handle this rfc
	headers.Set("Forwarded",
		fmt.Sprintf("by=%s;for=%s;host=%s;proto=%s",
			byName,
			clientIPRfc,
			host,
			forwardedScheme,
		),
	)
	return headers
}

func (p *Proxy) getClientIP(req *http.Request) string {
	ip, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return ip
}

func (p *Proxy) getForwardedFor(req *http.Request) []string {
	forwardedFor := req.Header.Get("X-Forwarded-For")
	clientIp := p.getClientIP(req)
	if forwardedFor == "" {
		return []string{clientIp}
	}
	parsedForwardedFor := strings.Split(forwardedFor, ",")
	for i, ip := range parsedForwardedFor {
		parsedForwardedFor[i] = strings.TrimSpace(ip)
	}
	return append(parsedForwardedFor, clientIp)
}

func (p *Proxy) getForwardedScheme(req *http.Request) string {
	if req.TLS != nil {
		return "https"
	}
	return "http"
}

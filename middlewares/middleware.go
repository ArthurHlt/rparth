package middlewares

import "net/http"

type Middleware func(http.Handler) http.Handler

// Chain wraps next so the first middleware in mws is the outermost
// (runs first on the way in, last on the way out).
func Chain(mws ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			next = mws[i](next)
		}
		return next
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{w, http.StatusOK}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Unwrap lets http.NewResponseController traverse to the underlying
// writer so Flush/Hijack/etc. work through this wrapper. Without it,
// streaming responses (SSE) would silently lose explicit flushes.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

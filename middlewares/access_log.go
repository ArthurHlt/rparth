package middlewares

import (
	"log/slog"
	"net/http"

	"github.com/ArthurHlt/rparth/contexts"
	"github.com/go-chi/httplog/v3"
)

func AccessLog(level slog.Level) Middleware {
	return httplog.RequestLogger(slog.Default(), &httplog.Options{
		Level:         level,
		Schema:        httplog.SchemaOTEL,
		RecoverPanics: true,
		LogExtraAttrs: func(req *http.Request, reqBody string, respStatus int) []slog.Attr {
			route := contexts.GetRPRoute(req)
			routeName := "unknown"
			if route != nil {
				routeName = route.Name
			}
			return []slog.Attr{
				slog.String("route_name", routeName),
			}
		},
	})
}

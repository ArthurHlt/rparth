package middlewares

import (
	"log/slog"

	"github.com/go-chi/httplog/v3"
)

func AccessLog(level slog.Level) Middleware {
	return httplog.RequestLogger(slog.Default(), &httplog.Options{
		Level:         level,
		Schema:        httplog.SchemaOTEL,
		RecoverPanics: true,
	})
}

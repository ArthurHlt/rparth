package contexts

import (
	"context"
	"net/http"

	"github.com/ArthurHlt/rparth/models"
)

type RparthContextKey int

const (
	routeContextKey RparthContextKey = iota
)

func SetRPRoute(req *http.Request, route *models.RPRoute) *http.Request {
	if route == nil {
		return req
	}
	return req.WithContext(context.WithValue(req.Context(), routeContextKey, route))
}

func GetRPRoute(req *http.Request) *models.RPRoute {
	route, _ := req.Context().Value(routeContextKey).(*models.RPRoute)
	return route
}

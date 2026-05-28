package testutils

import (
	"net/http"
	"net/http/httptest"

	"github.com/ArthurHlt/rparth/contexts"
	"github.com/ArthurHlt/rparth/models"
)

func RequestWithRoute(method, path, routeName string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	if routeName != "" {
		req = contexts.SetRPRoute(req, &models.RPRoute{Name: routeName})
	}
	return req
}

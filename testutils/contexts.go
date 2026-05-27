package testutils

import (
	"net/http"
	"net/http/httptest"

	"github.com/ArthurHlt/rparth/contexts"
	"github.com/ArthurHlt/rparth/models"
)

func RequestWithRoute(method, routeName string) *http.Request {
	req := httptest.NewRequest(method, "/", nil)
	return contexts.SetRPRoute(req, &models.RPRoute{Name: routeName})
}

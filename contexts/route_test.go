package contexts_test

import (
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/contexts"
	"github.com/ArthurHlt/rparth/models"
)

var _ = Describe("GetRouteName", func() {
	It("returns 'unknown' when no route is set on the request", func() {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		Expect(contexts.GetRouteName(req)).To(Equal("unknown"))
	})

	It("returns the matched route name when a route is set", func() {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req = contexts.SetRPRoute(req, &models.RPRoute{Name: "api"})
		Expect(contexts.GetRouteName(req)).To(Equal("api"))
	})
})

package models_test

import (
	"net/http"
	"net/url"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/models"
)

func mustParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	Expect(err).NotTo(HaveOccurred())
	return u
}

func newRequest(host, path string) *http.Request {
	return &http.Request{
		Host: host,
		URL:  &url.URL{Path: path},
	}
}

var _ = Describe("RPRoute", func() {
	var route *models.RPRoute

	BeforeEach(func() {
		route = &models.RPRoute{
			Name:   "api",
			Host:   "api.example.com",
			Prefix: "/v1",
			Target: mustParseURL("http://backend:8080"),
		}
	})

	Describe("Validate", func() {
		It("succeeds for a fully-specified route", func() {
			Expect(route.Validate()).To(Succeed())
		})

		It("rejects an empty Name", func() {
			route.Name = ""
			Expect(route.Validate()).To(MatchError("route name cannot be empty"))
		})

		It("rejects a nil Target", func() {
			route.Target = nil
			Expect(route.Validate()).To(MatchError("route target cannot be empty"))
		})

		It("defaults an empty Prefix to '/'", func() {
			route.Prefix = ""
			Expect(route.Validate()).To(Succeed())
			Expect(route.Prefix).To(Equal("/"))
		})

		It("preserves an explicit Prefix", func() {
			route.Prefix = "/v2"
			Expect(route.Validate()).To(Succeed())
			Expect(route.Prefix).To(Equal("/v2"))
		})

		It("defaults a zero Timeout to 30", func() {
			route.Timeout = 0
			Expect(route.Validate()).To(Succeed())
			Expect(route.Timeout).To(Equal(uint(30)))
		})

		It("preserves an explicit Timeout", func() {
			route.Timeout = 5
			Expect(route.Validate()).To(Succeed())
			Expect(route.Timeout).To(Equal(uint(5)))
		})

		It("defaults a nil StripPrefix to true", func() {
			route.StripPrefix = nil
			Expect(route.Validate()).To(Succeed())
			Expect(route.StripPrefix).NotTo(BeNil())
			Expect(*route.StripPrefix).To(BeTrue())
		})

		It("preserves an explicit StripPrefix=false", func() {
			f := false
			route.StripPrefix = &f
			Expect(route.Validate()).To(Succeed())
			Expect(route.StripPrefix).NotTo(BeNil())
			Expect(*route.StripPrefix).To(BeFalse())
		})

		It("checks Name before Target", func() {
			route.Name = ""
			route.Target = nil
			Expect(route.Validate()).To(MatchError("route name cannot be empty"))
		})
	})

	Describe("Match", func() {
		Context("when Host is set on the route", func() {
			It("matches when host and prefix both match", func() {
				req := newRequest("api.example.com:80", "/v1/users")
				Expect(route.Match(req)).To(BeTrue())
			})

			It("matches when the path equals the prefix exactly", func() {
				req := newRequest("api.example.com:80", "/v1")
				Expect(route.Match(req)).To(BeTrue())
			})

			It("does not match when the host differs", func() {
				req := newRequest("other.example.com:80", "/v1/users")
				Expect(route.Match(req)).To(BeFalse())
			})

			It("does not match when the path does not start with the prefix", func() {
				req := newRequest("api.example.com:80", "/v2/users")
				Expect(route.Match(req)).To(BeFalse())
			})

			It("does not match when the path is empty", func() {
				req := newRequest("api.example.com:80", "")
				Expect(route.Match(req)).To(BeFalse())
			})

			It("matches when req.Host has no port (SplitHostPort falls back to raw host)", func() {
				req := newRequest("api.example.com", "/v1/users")
				Expect(route.Match(req)).To(BeTrue())
			})
		})

		Context("when Host is empty (wildcard)", func() {
			BeforeEach(func() {
				route.Host = ""
			})

			It("matches any host as long as the prefix matches", func() {
				req := newRequest("whatever.example.com:443", "/v1/users")
				Expect(route.Match(req)).To(BeTrue())
			})

			It("still requires the prefix to match", func() {
				req := newRequest("whatever.example.com:443", "/other")
				Expect(route.Match(req)).To(BeFalse())
			})
		})
	})

	Describe("String", func() {
		It("formats as 'name -> target'", func() {
			Expect(route.String()).To(Equal("api -> http://backend:8080"))
		})
	})
})

var _ = Describe("RPRoutes.FindRoute", func() {
	var (
		apiRoute *models.RPRoute
		webRoute *models.RPRoute
		routes   models.RPRoutes
	)

	BeforeEach(func() {
		apiRoute = &models.RPRoute{
			Name:   "api",
			Host:   "api.example.com",
			Prefix: "/v1",
			Target: mustParseURL("http://api-backend:8080"),
		}
		webRoute = &models.RPRoute{
			Name:   "web",
			Host:   "web.example.com",
			Prefix: "/",
			Target: mustParseURL("http://web-backend:8080"),
		}
		routes = models.RPRoutes{apiRoute, webRoute}
	})

	It("returns the matching route", func() {
		req := newRequest("api.example.com:80", "/v1/users")
		found, err := routes.FindRoute(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeIdenticalTo(apiRoute))
	})

	It("returns the second route when only it matches", func() {
		req := newRequest("web.example.com:80", "/home")
		found, err := routes.FindRoute(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeIdenticalTo(webRoute))
	})

	It("returns the first matching route when several could match", func() {
		t := mustParseURL("http://t:80")
		rs := models.RPRoutes{
			{Name: "first", Prefix: "/", Target: t},
			{Name: "second", Prefix: "/", Target: t},
		}
		req := newRequest("anything.example.com:80", "/x")
		found, err := rs.FindRoute(req)
		Expect(err).NotTo(HaveOccurred())
		Expect(found.Name).To(Equal("first"))
	})

	It("errors when no route matches", func() {
		req := newRequest("unknown.example.com:80", "/x")
		found, err := routes.FindRoute(req)
		Expect(err).To(MatchError("no route found"))
		Expect(found).To(BeNil())
	})

	It("errors when the route list is empty", func() {
		empty := models.RPRoutes{}
		req := newRequest("api.example.com:80", "/v1/users")
		found, err := empty.FindRoute(req)
		Expect(err).To(MatchError("no route found"))
		Expect(found).To(BeNil())
	})
})
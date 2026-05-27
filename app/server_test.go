package app_test

import (
	"log/slog"
	"net"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/app"
	"github.com/ArthurHlt/rparth/config"
	"github.com/ArthurHlt/rparth/models"
	"github.com/ArthurHlt/rparth/testutils"
)

var _ = Describe("App.RunServer", func() {
	var originalLogger *slog.Logger

	BeforeEach(func() {
		originalLogger = slog.Default()
	})

	AfterEach(func() {
		slog.SetDefault(originalLogger)
	})

	It("returns an error when the listen address is already in use", func() {
		// Hold a port so RunServer can't bind to it. This exercises the error
		// return without leaking a running server goroutine.
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer listener.Close()

		cnf := &config.Config{
			Routes: models.RPRoutes{
				{
					Name:   "api",
					Prefix: "/",
					Target: testutils.MustYamlParseURL("http://backend:8080"),
				},
			},
			Server: &config.Server{ListenAddr: listener.Addr().String()},
		}

		a := app.NewApp(cnf)

		Expect(a.RunServer()).To(HaveOccurred())
	})

	It("returns an error when the listen address is malformed", func() {
		cnf := &config.Config{
			Routes: models.RPRoutes{
				{
					Name:   "api",
					Prefix: "/",
					Target: testutils.MustYamlParseURL("http://backend:8080"),
				},
			},
			Server: &config.Server{ListenAddr: "not-a-valid-addr"},
		}

		a := app.NewApp(cnf)

		Expect(a.RunServer()).To(HaveOccurred())
	})
})

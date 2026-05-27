package app_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

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

	It("returns an error when the builder fails to bind the listener", func() {
		// Hold a port so the default builder's net.Listen call fails.
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		Expect(err).NotTo(HaveOccurred())
		defer listener.Close()

		cnf := &config.Config{
			Routes: models.RPRoutes{{Name: "api", Prefix: "/", Target: testutils.MustYamlParseURL("http://backend:8080")}},
			Server: &config.Server{ListenAddr: listener.Addr().String()},
		}
		a := app.NewAppBare(cnf)

		Expect(a.RunServer(context.Background(), context.Background())).To(HaveOccurred())
	})

	It("returns an error when the listen address is malformed", func() {
		cnf := &config.Config{
			Routes: models.RPRoutes{{Name: "api", Prefix: "/", Target: testutils.MustYamlParseURL("http://backend:8080")}},
			Server: &config.Server{ListenAddr: "not-a-valid-addr"},
		}
		a := app.NewAppBare(cnf)

		Expect(a.RunServer(context.Background(), context.Background())).To(HaveOccurred())
	})

	It("propagates an error from a custom builder", func() {
		cnf := &config.Config{
			Routes: models.RPRoutes{{Name: "api", Prefix: "/", Target: testutils.MustYamlParseURL("http://backend:8080")}},
			Server: &config.Server{ListenAddr: ":0"},
		}
		a := app.NewAppBare(cnf)

		boom := &net.AddrError{Err: "builder said no", Addr: "x"}
		a.SetServerBuilder(func(_ string) (*http.Server, net.Listener, error) {
			return nil, nil, boom
		})

		Expect(a.RunServer(context.Background(), context.Background())).To(MatchError(boom))
	})

	It("serves traffic and returns nil after graceful shutdown via stopCtx", func() {
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
		}))
		defer backend.Close()

		frontend, builder := newFrontend()
		defer frontend.Close()

		cnf := &config.Config{
			Routes: models.RPRoutes{{Name: "api", Prefix: "/", Target: testutils.MustYamlParseURL(backend.URL)}},
			Server: &config.Server{ListenAddr: frontend.Listener.Addr().String()},
		}
		a := app.NewAppBare(cnf)
		a.SetServerBuilder(builder)

		errCh, stopCancel, forceCancel := runInBackground(a)
		defer forceCancel()

		resp, err := http.Get(fmt.Sprintf("http://%s/", frontend.Listener.Addr().String()))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Body.Close()).To(Succeed())
		Expect(resp.StatusCode).To(Equal(http.StatusTeapot))

		stopCancel()
		Eventually(errCh, "3s").Should(Receive(BeNil()))
	})

	It("returns nil when forceCtx is cancelled while shutdown is blocked on an in-flight request", func() {
		// Backend that blocks until the test releases it. While blocked, the
		// proxy holds an active connection so server.Shutdown can't drain.
		release := make(chan struct{})
		backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			<-release
			w.WriteHeader(http.StatusOK)
		}))
		defer backend.Close()
		// Ensure the goroutine inside the backend handler unblocks at the end so
		// the in-flight request finishes and the abandoned shutdown goroutine
		// completes cleanly.
		defer close(release)

		frontend, builder := newFrontend()
		defer frontend.Close()

		cnf := &config.Config{
			Routes: models.RPRoutes{{Name: "api", Prefix: "/", Target: testutils.MustYamlParseURL(backend.URL)}},
			Server: &config.Server{ListenAddr: frontend.Listener.Addr().String()},
		}
		a := app.NewAppBare(cnf)
		a.SetServerBuilder(builder)

		errCh, stopCancel, forceCancel := runInBackground(a)
		defer stopCancel()
		defer forceCancel()

		// Fire a request that will block in the backend handler.
		reqStarted := make(chan struct{})
		go func() {
			defer GinkgoRecover()
			close(reqStarted)
			resp, err := http.Get(fmt.Sprintf("http://%s/", frontend.Listener.Addr().String()))
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
		<-reqStarted
		// Give the proxy a moment to actually start handling the request before
		// we trigger shutdown — otherwise Shutdown might see an idle connection.
		time.Sleep(100 * time.Millisecond)

		stopCancel()
		Consistently(errCh, "200ms").ShouldNot(Receive(), "RunServer must wait for shutdown or force")

		forceCancel()
		Eventually(errCh, "2s").Should(Receive(BeNil()))
	})

	Context("with TLS", func() {
		It("serves over HTTPS when Server.Tls is set, then shuts down gracefully", func() {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusTeapot)
			}))
			defer backend.Close()

			frontend, builder := newFrontend()
			defer frontend.Close()

			cnf := &config.Config{
				Routes: models.RPRoutes{{Name: "api", Prefix: "/", Target: testutils.MustYamlParseURL(backend.URL)}},
				Server: &config.Server{
					ListenAddr: frontend.Listener.Addr().String(),
					Tls: &config.ServerTLS{
						CertFile: testutils.AssetPath("cert.crt"),
						KeyFile:  testutils.AssetPath("key.key"),
					},
				},
			}
			a := app.NewAppBare(cnf)
			a.SetServerBuilder(builder)

			errCh, stopCancel, forceCancel := runInBackground(a)
			defer forceCancel()

			client := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed cert in test
				},
				Timeout: 3 * time.Second,
			}
			var resp *http.Response
			Eventually(func() error {
				r, err := client.Get(fmt.Sprintf("https://%s/", frontend.Listener.Addr().String()))
				if err != nil {
					return err
				}
				resp = r
				return nil
			}, "3s", "50ms").Should(Succeed())
			Expect(resp.Body.Close()).To(Succeed())
			Expect(resp.StatusCode).To(Equal(http.StatusTeapot))

			stopCancel()
			Eventually(errCh, "3s").Should(Receive(BeNil()))
		})

		It("returns an error when the TLS cert files don't exist", func() {
			frontend, builder := newFrontend()
			defer frontend.Close()

			cnf := &config.Config{
				Routes: models.RPRoutes{{Name: "api", Prefix: "/", Target: testutils.MustYamlParseURL("http://backend:8080")}},
				Server: &config.Server{
					ListenAddr: frontend.Listener.Addr().String(),
					Tls:        &config.ServerTLS{CertFile: "/nonexistent/cert.pem", KeyFile: "/nonexistent/key.pem"},
				},
			}
			a := app.NewAppBare(cnf)
			a.SetServerBuilder(builder)

			Expect(a.RunServer(context.Background(), context.Background())).To(HaveOccurred())
		})
	})
})

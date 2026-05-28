package config_test

import (
	"log/slog"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/config"
)

func writeConfigFile(content string) string {
	dir := GinkgoT().TempDir()
	path := filepath.Join(dir, "config.yml")
	Expect(os.WriteFile(path, []byte(content), 0o600)).To(Succeed())
	return path
}

var _ = Describe("ReadConfig", func() {
	Describe("happy path", func() {
		It("decodes a fully-specified configuration", func() {
			path := writeConfigFile(`
routes:
  - name: api
    host: api.example.com
    prefix: /v1
    target: http://api-backend:8080
    timeout: 5
    strip_prefix: false
    headers:
      x-tenant: ["acme"]
server:
  listen_addr: ":9090"
log:
  level: debug
  no_color: true
  in_json: true
`)
			cnf, err := config.ReadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cnf).NotTo(BeNil())

			Expect(cnf.Server).NotTo(BeNil())
			Expect(cnf.Server.ListenAddr).To(Equal(":9090"))

			Expect(cnf.Log.Level).To(Equal(slog.LevelDebug))
			Expect(cnf.Log.NoColor).To(BeTrue())
			Expect(cnf.Log.InJson).To(BeTrue())

			Expect(cnf.Routes).To(HaveLen(1))
			route := cnf.Routes[0]
			Expect(route.Name).To(Equal("api"))
			Expect(route.Host).To(Equal("api.example.com"))
			Expect(route.Prefix).To(Equal("/v1"))
			Expect(route.Target).NotTo(BeNil())
			Expect(route.Target.String()).To(Equal("http://api-backend:8080"))
			Expect(route.Timeout).To(Equal(uint(5)))
			Expect(route.StripPrefix).NotTo(BeNil())
			Expect(*route.StripPrefix).To(BeFalse())
			// Headers keys are canonicalized by RPRoute.Validate.
			Expect(route.Headers).To(HaveKeyWithValue("X-Tenant", []string{"acme"}))
		})

		It("defaults the server block to ':8080' when absent", func() {
			path := writeConfigFile(`
routes:
  - name: api
    target: http://api-backend:8080
`)
			cnf, err := config.ReadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cnf.Server).NotTo(BeNil())
			Expect(cnf.Server.ListenAddr).To(Equal(":8080"))
		})

		It("defaults the server's listen_addr to ':8080' when only log/routes provided", func() {
			path := writeConfigFile(`
routes:
  - name: api
    target: http://api-backend:8080
server: {}
`)
			cnf, err := config.ReadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cnf.Server).NotTo(BeNil())
			Expect(cnf.Server.ListenAddr).To(Equal(":8080"))
		})

		It("applies RPRoute defaults (prefix '/', timeout 30, strip_prefix true)", func() {
			path := writeConfigFile(`
routes:
  - name: minimal
    target: http://backend:8080
`)
			cnf, err := config.ReadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cnf.Routes).To(HaveLen(1))
			route := cnf.Routes[0]
			Expect(route.Prefix).To(Equal("/"))
			Expect(route.Timeout).To(Equal(uint(30)))
			Expect(route.StripPrefix).NotTo(BeNil())
			Expect(*route.StripPrefix).To(BeTrue())
			Expect(route.NoCache).To(BeFalse())
		})

		It("decodes no_cache: true on a route", func() {
			path := writeConfigFile(`
routes:
  - name: uncached
    target: http://backend:8080
    no_cache: true
`)
			cnf, err := config.ReadConfig(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(cnf.Routes[0].NoCache).To(BeTrue())
		})
	})

	Describe("error paths", func() {
		It("errors when the file does not exist", func() {
			_, err := config.ReadConfig(filepath.Join(GinkgoT().TempDir(), "nope.yml"))
			Expect(err).To(HaveOccurred())
			Expect(os.IsNotExist(err)).To(BeTrue())
		})

		It("errors when the file is empty", func() {
			path := writeConfigFile("")
			_, err := config.ReadConfig(path)
			Expect(err).To(MatchError("config file is empty"))
		})

		It("errors when the YAML is invalid", func() {
			path := writeConfigFile("routes: [unterminated\n")
			_, err := config.ReadConfig(path)
			Expect(err).To(HaveOccurred())
		})

		It("errors when routes is missing", func() {
			path := writeConfigFile(`
server:
  listen_addr: ":8080"
`)
			_, err := config.ReadConfig(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no routes configured"))
		})

		It("errors when routes is an empty list", func() {
			path := writeConfigFile(`
routes: []
`)
			_, err := config.ReadConfig(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no routes configured"))
		})

		It("propagates RPRoute validation errors (missing name)", func() {
			path := writeConfigFile(`
routes:
  - target: http://backend:8080
`)
			_, err := config.ReadConfig(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("route name cannot be empty"))
		})

		It("propagates RPRoute validation errors (missing target)", func() {
			path := writeConfigFile(`
routes:
  - name: api
`)
			_, err := config.ReadConfig(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("route target cannot be empty"))
		})

		It("rejects duplicate route names", func() {
			path := writeConfigFile(`
routes:
  - name: api
    target: http://a:8080
  - name: api
    target: http://b:8080
`)
			_, err := config.ReadConfig(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("duplicate route name: api"))
		})
	})

	Describe("log level parsing", func() {
		// slog.Level.UnmarshalJSON is invoked via yaml.UseJSONUnmarshaler() so
		// the textual names ("debug", "info", "warn", "error") must decode.
		DescribeTable("decodes textual slog levels",
			func(text string, expected slog.Level) {
				path := writeConfigFile(`
routes:
  - name: api
    target: http://backend:8080
log:
  level: ` + text + `
`)
				cnf, err := config.ReadConfig(path)
				Expect(err).NotTo(HaveOccurred())
				Expect(cnf.Log.Level).To(Equal(expected))
			},
			Entry("debug", "debug", slog.LevelDebug),
			Entry("info", "info", slog.LevelInfo),
			Entry("warn", "warn", slog.LevelWarn),
			Entry("error", "error", slog.LevelError),
		)
	})
})

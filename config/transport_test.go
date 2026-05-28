package config_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/config"
)

var _ = Describe("Transport config", func() {
	It("applies all defaults when the transport block is an empty mapping", func() {
		path := writeConfigFile(`
routes:
  - name: api
    target: http://backend:8080
transport: {}
`)
		cnf, err := config.ReadConfig(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(cnf.Transport.Timeout).To(Equal(30 * time.Second))
		Expect(cnf.Transport.KeepAlive).To(Equal(30 * time.Second))
		Expect(cnf.Transport.MaxIdleConns).To(Equal(1000))
		Expect(cnf.Transport.MaxIdleConnsPerHost).To(Equal(100))
		Expect(cnf.Transport.IdleConnTimeout).To(Equal(30 * time.Second))
		Expect(cnf.Transport.ResponseHeaderTimeout).To(Equal(30 * time.Second))
		Expect(cnf.Transport.TLSHandshakeTimeout).To(Equal(10 * time.Second))
	})

	// Quirk: Transport is a value type on Config, so omitting the `transport:`
	// key entirely never invokes Transport.UnmarshalYAML — defaults are NOT
	// applied. Use `transport: {}` to opt into defaults. This spec pins that
	// behaviour so it doesn't change silently.
	It("leaves fields zero when the transport block is omitted", func() {
		path := writeConfigFile(`
routes:
  - name: api
    target: http://backend:8080
`)
		cnf, err := config.ReadConfig(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(cnf.Transport).To(Equal(config.Transport{}))
	})

	It("applies defaults only to unset fields", func() {
		path := writeConfigFile(`
routes:
  - name: api
    target: http://backend:8080
transport:
  timeout: 5s
  max_idle_conns: 42
`)
		cnf, err := config.ReadConfig(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(cnf.Transport.Timeout).To(Equal(5 * time.Second))
		Expect(cnf.Transport.MaxIdleConns).To(Equal(42))
		Expect(cnf.Transport.KeepAlive).To(Equal(30 * time.Second))
		Expect(cnf.Transport.IdleConnTimeout).To(Equal(30 * time.Second))
		Expect(cnf.Transport.TLSHandshakeTimeout).To(Equal(10 * time.Second))
	})

	It("decodes every field when fully specified", func() {
		path := writeConfigFile(`
routes:
  - name: api
    target: http://backend:8080
transport:
  timeout: 1s
  keepalive: 2s
  max_idle_conns: 10
  max_idle_conns_per_host: 5
  idle_conn_timeout: 3s
  response_header_timeout: 4s
  tls_handshake_timeout: 6s
`)
		cnf, err := config.ReadConfig(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(cnf.Transport).To(Equal(config.Transport{
			Timeout:               1 * time.Second,
			KeepAlive:             2 * time.Second,
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   5,
			IdleConnTimeout:       3 * time.Second,
			ResponseHeaderTimeout: 4 * time.Second,
			TLSHandshakeTimeout:   6 * time.Second,
		}))
	})
})

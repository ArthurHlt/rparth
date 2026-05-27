package config_test

import (
	"github.com/goccy/go-yaml"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/config"
)

var _ = Describe("Server.UnmarshalYAML", func() {
	It("decodes an explicit listen_addr", func() {
		var s config.Server
		Expect(yaml.Unmarshal([]byte("listen_addr: \":9999\"\n"), &s)).To(Succeed())
		Expect(s.ListenAddr).To(Equal(":9999"))
	})

	It("defaults missing listen_addr to ':8080'", func() {
		var s config.Server
		Expect(yaml.Unmarshal([]byte("{}\n"), &s)).To(Succeed())
		Expect(s.ListenAddr).To(Equal(":8080"))
	})

	It("defaults empty listen_addr to ':8080'", func() {
		var s config.Server
		Expect(yaml.Unmarshal([]byte("listen_addr: \"\"\n"), &s)).To(Succeed())
		Expect(s.ListenAddr).To(Equal(":8080"))
	})

	It("returns an error on malformed YAML", func() {
		var s config.Server
		Expect(yaml.Unmarshal([]byte("listen_addr: [not, a, string]\n"), &s)).To(HaveOccurred())
	})

	It("decodes a TLS block", func() {
		var s config.Server
		Expect(yaml.Unmarshal([]byte("tls:\n  cert_file: /tmp/cert.pem\n  key_file: /tmp/key.pem\n"), &s)).To(Succeed())
		Expect(s.Tls).NotTo(BeNil())
		Expect(s.Tls.CertFile).To(Equal("/tmp/cert.pem"))
		Expect(s.Tls.KeyFile).To(Equal("/tmp/key.pem"))
	})

	It("leaves Tls nil when no tls block is given", func() {
		var s config.Server
		Expect(yaml.Unmarshal([]byte("listen_addr: \":9000\"\n"), &s)).To(Succeed())
		Expect(s.Tls).To(BeNil())
	})
})

var _ = Describe("ServerTls.UnmarshalYAML", func() {
	It("decodes a valid block", func() {
		var t config.ServerTLS
		Expect(yaml.Unmarshal([]byte("cert_file: /tmp/cert.pem\nkey_file: /tmp/key.pem\n"), &t)).To(Succeed())
		Expect(t.CertFile).To(Equal("/tmp/cert.pem"))
		Expect(t.KeyFile).To(Equal("/tmp/key.pem"))
	})

	It("rejects a missing cert_file", func() {
		var t config.ServerTLS
		Expect(yaml.Unmarshal([]byte("key_file: /tmp/key.pem\n"), &t)).To(MatchError(ContainSubstring("cert_file and key_file are required")))
	})

	It("rejects a missing key_file", func() {
		var t config.ServerTLS
		Expect(yaml.Unmarshal([]byte("cert_file: /tmp/cert.pem\n"), &t)).To(MatchError(ContainSubstring("cert_file and key_file are required")))
	})

	It("rejects an empty block", func() {
		var t config.ServerTLS
		Expect(yaml.Unmarshal([]byte("{}\n"), &t)).To(MatchError(ContainSubstring("cert_file and key_file are required")))
	})

	It("returns an error on malformed YAML", func() {
		var t config.ServerTLS
		Expect(yaml.Unmarshal([]byte("cert_file: [not, a, string]\n"), &t)).To(HaveOccurred())
	})
})

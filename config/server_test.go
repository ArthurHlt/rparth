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
})

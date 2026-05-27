package testutils

import (
	"net/url"

	"github.com/onsi/gomega"
)

func MustYamlParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return u
}

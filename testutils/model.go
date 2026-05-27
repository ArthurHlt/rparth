package testutils

import (
	"net/url"
	"path/filepath"
	"runtime"

	"github.com/onsi/gomega"
)

func MustYamlParseURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return u
}

func AssetPath(name string) string {
	_, thisFile, _, ok := runtime.Caller(0)
	gomega.Expect(ok).To(gomega.BeTrue(), "runtime.Caller failed")
	return filepath.Join(filepath.Dir(thisFile), "assets", name)
}

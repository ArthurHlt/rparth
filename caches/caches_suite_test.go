package caches_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCaches(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Caches Suite")
}

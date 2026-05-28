package contexts_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestContexts(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Contexts Suite")
}

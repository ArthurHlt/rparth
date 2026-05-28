package config_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/config"
)

var _ = Describe("Cache config — redis block", func() {
	It("decodes a redis cache config", func() {
		path := writeConfigFile(`
routes:
  - name: api
    target: http://backend:8080
cache:
  redis:
    url: redis://localhost:6379/0
    ttl: 1m
`)
		cnf, err := config.ReadConfig(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(cnf.Cache.Redis).NotTo(BeNil())
		Expect(cnf.Cache.Redis.URL).To(Equal("redis://localhost:6379/0"))
		Expect(cnf.Cache.Redis.Ttl).To(Equal(time.Minute))
	})

	It("defaults redis ttl to 10m when omitted", func() {
		path := writeConfigFile(`
routes:
  - name: api
    target: http://backend:8080
cache:
  redis:
    url: redis://localhost:6379/0
`)
		cnf, err := config.ReadConfig(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(cnf.Cache.Redis.Ttl).To(Equal(10 * time.Minute))
	})

	It("errors when redis url is missing", func() {
		path := writeConfigFile(`
routes:
  - name: api
    target: http://backend:8080
cache:
  redis:
    ttl: 1m
`)
		_, err := config.ReadConfig(path)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("url is required"))
	})
})

var _ = Describe("Cache config — mutual exclusion", func() {
	It("errors when both lru and redis are set", func() {
		path := writeConfigFile(`
routes:
  - name: api
    target: http://backend:8080
cache:
  lru: {}
  redis:
    url: redis://localhost:6379/0
`)
		_, err := config.ReadConfig(path)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("only one of lru or redis"))
	})
})

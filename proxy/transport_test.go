package proxy_test

import (
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/config"
	"github.com/ArthurHlt/rparth/proxy"
)

var _ = Describe("DefaultProxyTransport", func() {
	It("returns a *http.Transport configured from the supplied config.Transport", func() {
		cfg := config.Transport{
			Timeout:               1 * time.Second,
			KeepAlive:             2 * time.Second,
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   5,
			IdleConnTimeout:       3 * time.Second,
			ResponseHeaderTimeout: 4 * time.Second,
			TLSHandshakeTimeout:   6 * time.Second,
		}

		rt := proxy.DefaultProxyTransport(cfg)
		t, ok := rt.(*http.Transport)
		Expect(ok).To(BeTrue(), "expected *http.Transport, got %T", rt)

		Expect(t.MaxIdleConns).To(Equal(10))
		Expect(t.MaxIdleConnsPerHost).To(Equal(5))
		Expect(t.IdleConnTimeout).To(Equal(3 * time.Second))
		Expect(t.ResponseHeaderTimeout).To(Equal(4 * time.Second))
		Expect(t.TLSHandshakeTimeout).To(Equal(6 * time.Second))
	})

	It("disables transport-level compression so the response body can stream straight through", func() {
		rt := proxy.DefaultProxyTransport(config.Transport{})
		t, ok := rt.(*http.Transport)
		Expect(ok).To(BeTrue())
		Expect(t.DisableCompression).To(BeTrue())
	})

	It("forwards HTTP(S) proxy settings via http.ProxyFromEnvironment", func() {
		rt := proxy.DefaultProxyTransport(config.Transport{})
		t, ok := rt.(*http.Transport)
		Expect(ok).To(BeTrue())
		// Proxy is a function field; we can't compare functions directly,
		// so just assert it was set (non-nil). Behaviour comes from the
		// stdlib helper.
		Expect(t.Proxy).NotTo(BeNil())
	})
})

package app_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ArthurHlt/rparth/app"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestApp(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "App Suite")
}

// newFrontend returns an httptest.NewUnstartedServer that gives us a
// pre-bound free-port listener, and a builder that hands that listener to
// RunServer. The test owns the *httptest.Server's lifecycle (.Close) so the
// listener is reclaimed even if RunServer never takes ownership.
func newFrontend() (*httptest.Server, app.HttpServerBuilder) {
	ts := httptest.NewUnstartedServer(nil)
	builder := func(_ string) (*http.Server, net.Listener, error) {
		return ts.Config, ts.Listener, nil
	}
	return ts, builder
}

// runInBackground starts a.RunServer in a goroutine and returns the channel
// it'll publish its return value on, plus the cancels for both contexts.
func runInBackground(a *app.App) (<-chan error, context.CancelFunc, context.CancelFunc) {
	stopCtx, stopCancel := context.WithCancel(context.Background())
	forceCtx, forceCancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.RunServer(stopCtx, forceCtx)
	}()
	return errCh, stopCancel, forceCancel
}

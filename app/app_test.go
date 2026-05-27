package app_test

import (
	"context"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ArthurHlt/rparth/app"
	"github.com/ArthurHlt/rparth/config"
	"github.com/ArthurHlt/rparth/models"
	"github.com/ArthurHlt/rparth/testutils"
)

// NewApp swaps slog.Default(), which is global. Snapshot it once and restore
// it after every spec so other suites running in the same binary stay sane.
var _ = Describe("NewApp", func() {
	var originalLogger *slog.Logger

	BeforeEach(func() {
		originalLogger = slog.Default()
	})

	AfterEach(func() {
		slog.SetDefault(originalLogger)
	})

	baseConfig := func() *config.Config {
		return &config.Config{
			Routes: models.RPRoutes{
				{
					Name:   "api",
					Prefix: "/",
					Target: testutils.MustYamlParseURL("http://backend:8080"),
				},
			},
			Server: &config.Server{ListenAddr: ":0"},
		}
	}

	It("returns a non-nil app for a valid config", func() {
		Expect(app.NewApp(baseConfig())).NotTo(BeNil())
	})

	// tint's concrete handler type is unexported, so we can't assert against it
	// directly. We assert the negative — non-JSON mode produces something other
	// than *slog.JSONHandler — which is enough to catch the InJson branch
	// flipping the wrong way.
	It("does not install a JSON handler by default", func() {
		cnf := baseConfig()
		cnf.Log = config.Log{Level: slog.LevelInfo}

		app.NewApp(cnf)

		_, isJSON := slog.Default().Handler().(*slog.JSONHandler)
		Expect(isJSON).To(BeFalse())
	})

	It("installs a JSON handler when Log.InJson is true", func() {
		cnf := baseConfig()
		cnf.Log = config.Log{InJson: true, Level: slog.LevelInfo}

		app.NewApp(cnf)

		_, ok := slog.Default().Handler().(*slog.JSONHandler)
		Expect(ok).To(BeTrue(), "expected *slog.JSONHandler, got %T", slog.Default().Handler())
	})

	DescribeTable("propagates Log.Level to the installed handler",
		func(level slog.Level) {
			cnf := baseConfig()
			cnf.Log = config.Log{Level: level}

			app.NewApp(cnf)

			ctx := context.Background()
			Expect(slog.Default().Enabled(ctx, level)).To(BeTrue())
			if level > slog.LevelDebug {
				Expect(slog.Default().Enabled(ctx, level-1)).To(BeFalse())
			}
		},
		Entry("debug", slog.LevelDebug),
		Entry("info", slog.LevelInfo),
		Entry("warn", slog.LevelWarn),
		Entry("error", slog.LevelError),
	)

	It("honours Log.Level in JSON mode too", func() {
		cnf := baseConfig()
		cnf.Log = config.Log{InJson: true, Level: slog.LevelError}

		app.NewApp(cnf)

		ctx := context.Background()
		Expect(slog.Default().Enabled(ctx, slog.LevelError)).To(BeTrue())
		Expect(slog.Default().Enabled(ctx, slog.LevelWarn)).To(BeFalse())
	})
})

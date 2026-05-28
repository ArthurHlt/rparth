package main

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	metricBuildInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "rparth_build_info",
			Help: "Build information for rparth",
		},
		[]string{"version", "commit", "date"},
	)
)

package testutils

import "github.com/prometheus/client_golang/prometheus"

// MetricValue reads the numeric value (counter or gauge) of a single metric
// cell from the default Prometheus registry, matched by name and the given
// labels. It returns 0 when no matching cell is found, so callers should
// compare a before/after delta when the metric is a process-wide global.
func MetricValue(name string, labels map[string]string) float64 {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return 0
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
	cells:
		for _, m := range mf.GetMetric() {
			got := make(map[string]string, len(m.GetLabel()))
			for _, lp := range m.GetLabel() {
				got[lp.GetName()] = lp.GetValue()
			}
			for k, v := range labels {
				if got[k] != v {
					continue cells
				}
			}
			switch {
			case m.Counter != nil:
				return m.GetCounter().GetValue()
			case m.Gauge != nil:
				return m.GetGauge().GetValue()
			}
		}
	}
	return 0
}

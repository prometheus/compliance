package cases

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

// InstanceLabel exports a single metric - the current time - and checks that we receive
// that metric via remote write, and that it has a instance label that we expect.
func InstanceLabelTest() Test {
	return Test{
		Name: "InstanceLabel",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "gauge",
		}, func() float64 {
			return 42.0
		})),
		Expected: func(t *testing.T, bs []Batch) {
			// Check we got the metric
			gauges := countMetricWithValue(t, bs, labels.FromStrings("__name__", "gauge"), 42.0)
			require.True(t, gauges > 0, `found zero samples for gauge{job="test"}`)

			labelMustMatch(t, bs, "instance", `127.0.0.1:\d+`)
		},
	}
}

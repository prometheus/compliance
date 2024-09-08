package cases

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

// JobLabel exports a single metric - the current time - and checks that we receive
// that metric via remote write, and that it has a job label.
func JobLabelTest() Test {
	return Test{
		Name: "JobLabel",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "gauge",
		}, func() float64 {
			return 42.0
		})),
		Expected: func(t *testing.T, bs []Batch) {
			gauges := countMetricWithValue(t, bs, labels.FromStrings("__name__", "gauge", "job", "test"), 42.0)
			require.True(t, gauges > 0, `found zero samples for gauge{job="test"}`)
		},
	}
}

package cases

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

// UpTest exports a single, constant metric and checks that we receive
// up metrics for that target, and that it has the correct value.
func UpTest() Test {
	return Test{
		Name: "Up",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "gauge",
		}, func() float64 {
			return 42.0
		})),
		Expected: func(t *testing.T, bs []Batch) {
			ups := countMetricWithValue(t, bs, labels.FromStrings("__name__", "up", "job", "test"), 1.0)
			require.True(t, ups > 0, `found zero samples for up{job="test"}`)
		},
	}
}

// InvalidTest exports invalid Prometheus metrics and checks we receive
// an up == 0 metric for that job.
func InvalidTest() Test {
	return Test{
		Name: "Invalid",
		Metrics: staticHandler([]byte(`
# this is not valid prometheus
1234notvali}{ 444
`)),
		Expected: func(t *testing.T, bs []Batch) {
			ups := countMetricWithValue(t, bs, labels.FromStrings("__name__", "up", "job", "test"), 0)
			require.True(t, ups > 0, `found zero samples for up{job="test"}`)
		},
	}
}

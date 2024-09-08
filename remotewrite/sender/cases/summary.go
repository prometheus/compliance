package cases

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

// SummaryTest exports a single metric - a summary - and checks that we receive
// that metric via remote write, and that it has the correct value.
func SummaryTest() Test {
	summary := prometheus.NewSummary(prometheus.SummaryOpts{
		Name:       "summary",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	})

	summary.Observe(1.0)
	summary.Observe(2.0)
	summary.Observe(3.0)

	return Test{
		Name:    "Summary",
		Metrics: metricHandler(summary),
		Expected: func(t *testing.T, bs []Batch) {
			p50 := countMetricWithValue(t, bs, labels.FromStrings("__name__", "summary", "quantile", "0.5"), 2.0)
			p90 := countMetricWithValue(t, bs, labels.FromStrings("__name__", "summary", "quantile", "0.9"), 3.0)
			p99 := countMetricWithValue(t, bs, labels.FromStrings("__name__", "summary", "quantile", "0.99"), 3.0)
			sum := countMetricWithValue(t, bs, labels.FromStrings("__name__", "summary_sum"), 6.0)
			count := countMetricWithValue(t, bs, labels.FromStrings("__name__", "summary_count"), 3.0)

			require.Equal(t, count, p50)
			require.Equal(t, count, p90)
			require.Equal(t, count, p99)
			require.Equal(t, count, sum)
			require.True(t, count > 0, `found zero samples for {__name__="summary_count"}`)
		},
	}
}

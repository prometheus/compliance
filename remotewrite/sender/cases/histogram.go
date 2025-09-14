package cases

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

// HistogramTest exports a histogram and checks that we receive
// metrics for it, and that it has the correct value.
func HistogramTest() Test {
	hist := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "histogram",
		Buckets: []float64{1.0, 2.0},
	})
	hist.Observe(1.0)
	hist.Observe(2.0)

	return Test{
		Name:    "Histogram",
		Metrics: metricHandler(hist),
		Expected: func(t *testing.T, bs []Batch) {
			le1 := countMetricWithValue(t, bs, labels.FromStrings("__name__", "histogram_bucket", "le", "1"), 1.0)
			le2 := countMetricWithValue(t, bs, labels.FromStrings("__name__", "histogram_bucket", "le", "2"), 2.0)
			inf := countMetricWithValue(t, bs, labels.FromStrings("__name__", "histogram_bucket", "le", "+Inf"), 2.0)
			sum := countMetricWithValue(t, bs, labels.FromStrings("__name__", "histogram_sum"), 3.0)
			count := countMetricWithValue(t, bs, labels.FromStrings("__name__", "histogram_count"), 2.0)

			require.Equal(t, count, le1)
			require.Equal(t, count, le2)
			require.Equal(t, count, inf)
			require.Equal(t, count, sum)
			require.True(t, count > 0, `found zero samples for {__name__="histogram_count"}`)
		},
	}
}

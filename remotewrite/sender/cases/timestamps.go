package cases

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

// TimestampTest exports a single, constant metric and checks that we receive
// that metric with "reasonable" timestamps.
func TimestampTest() Test {
	start := timestampMs(time.Now())

	return Test{
		Name: "Timestamp",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "gauge",
		}, func() float64 {
			return 42.0
		})),
		Expected: func(t *testing.T, bs []Batch) {
			end := timestampMs(time.Now())
			var ts int64

			forAllSamples(bs, func(s sample) {
				// Check the timestamp is "in bounds" for the test.
				require.Greater(t, s.t, start)
				require.Less(t, s.t, end)

				// Check the timestamps are increasing.
				require.GreaterOrEqual(t, s.t, ts)
				ts = s.t
			})

			ups := countMetricWithValue(t, bs, labels.FromStrings("__name__", "gauge"), 42.0)
			require.True(t, ups > 0, `found zero samples for up{job="test"}`)
		},
	}
}

func timestampMs(t time.Time) int64 {
	return t.Unix()*1000 + int64(t.Nanosecond()/1000000)
}

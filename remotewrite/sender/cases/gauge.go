package cases

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GaugeTest exports a single metric - the current time - and checks that we receive
// that metric via remote write, and that it has the correct value.
func GaugeTest() Test {
	return Test{
		Name: "Gauge",
		Metrics: metricHandler(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "now",
		}, func() float64 {
			return float64(time.Now().Unix() * 1000)
		})),
		Expected: func(t *testing.T, bs []Batch) {
			nows := countMetricWithValueFn(bs, labels.FromStrings("__name__", "now"),
				func(ts int64, v float64) bool {
					assert.InEpsilon(t, float64(ts), v, 0.01)
					return true
				})
			require.True(t, nows > 0, `found zero samples for {__name__="now"}`)
		},
	}
}

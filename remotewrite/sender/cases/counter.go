package cases

import (
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CounterTest exports a single metric - a counter - and checks that we receive
// that metric via remote write, and that it has the correct value.
func CounterTest() Test {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "counter",
	})
	hander := metricHandler(counter)

	return Test{
		Name: "Counter",
		Metrics: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			hander.ServeHTTP(w, r)
			counter.Inc()
		}),
		Expected: func(t *testing.T, bs []Batch) {
			var value float64
			nows := countMetricWithValueFn(bs, labels.FromStrings("__name__", "counter"),
				func(ts int64, v float64) bool {
					assert.Equal(t, value, v)
					value += 1.0
					return true
				})
			require.True(t, nows > 0, `found zero samples for {__name__="counter"}`)
		},
	}
}

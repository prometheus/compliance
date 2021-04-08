package cases

import (
	"testing"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BasicTest exports a single metric - the current time - and checks that we receive
// that metric via remote write, and that it has the correct value.
var BasicTest = Test{
	Name: "Basic Test",
	Metrics: funcHandler("now", func() float64 {
		return float64(time.Now().Unix() * 1000)
	}),
	Expected: func(t *testing.T, bs []Batch) {
		nows := countMetricWithValueFn(bs, labels.FromStrings("__name__", "now", "job", "test"), func(ts int64, v float64) {
			assert.InEpsilon(t, float64(ts), v, 0.01)
		})
		require.True(t, nows > 0, `found zero samples for now{job="test"}`)
	},
}

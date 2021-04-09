package cases

import (
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/require"
)

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

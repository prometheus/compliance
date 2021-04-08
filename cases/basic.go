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
		ups, nows := 0, 0
		for _, b := range bs {
			require.True(t, len(b.samples) > 0)
			for _, s := range b.samples {
				ls := removeLabel(s.l, "instance")
				require.Equal(t, "__name__", ls[0].Name)
				switch s.l[0].Value {
				case "up":
					require.Equal(t, labels.FromStrings("__name__", "up", "job", "test"), ls)
					require.Equal(t, float64(1), s.v)
					ups++
				case "now":
					require.Equal(t, labels.FromStrings("__name__", "now", "job", "test"), ls)
					assert.InEpsilon(t, float64(s.t), s.v, 0.01)
					nows++
				case "scrape_duration_seconds", "scrape_samples_scraped", "scrape_samples_post_metric_relabeling", "scrape_series_added":
					// this is optional, but acceptable metric.
				default:
					require.False(t, true, "unkown metric: %s", s.l)
				}
			}
		}
		require.True(t, ups > 0)
		require.Equal(t, ups, nows)
	},
}

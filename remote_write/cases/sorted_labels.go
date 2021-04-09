package cases

import (
	"sort"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/require"
)

// SortedLabelsTest exports a single, constant metric with labels in the wrong order
// and checks that we receive the metrics with sorted labels.
func SortedLabelsTest() Test {
	return Test{
		Name: "SortedLabels",
		Metrics: staticHandler([]byte(`
# HELP test A gauge
# TYPE test gauge
test{b="2",a="1"} 1.0
`)),
		Expected: func(t *testing.T, bs []Batch) {
			forAllSamples(bs, func(s sample) {
				names := []string{}
				for i := range s.l {
					names = append(names, s.l[i].Name)
				}
				require.True(t, sort.IsSorted(sort.StringSlice(names)), "'%s' is not sorted", s.l.String())
			})

			ups := countMetricWithValue(t, bs, labels.FromStrings("__name__", "test", "a", "1", "b", "2"), 1.0)
			require.True(t, ups > 0, `found zero samples for test{a="1",b="2"}`)
		},
	}
}

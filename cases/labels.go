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

			tests := countMetricWithValue(t, bs, labels.FromStrings("__name__", "test", "a", "1", "b", "2"), 1.0)
			require.True(t, tests > 0, `found zero samples for test{a="1",b="2"}`)
		},
	}
}

// RepeatedLabelsTest exports a single, constant metric with repeated labels
// and checks that we receive the metrics without repeated labels (and get up=0 instead).
func RepeatedLabelsTest() Test {
	return Test{
		Name: "RepeatedLabels",
		Metrics: staticHandler([]byte(`
# HELP test A gauge
# TYPE test gauge
test{a="1",a="1"} 1.0
`)),
		Expected: func(t *testing.T, bs []Batch) {
			forAllSamples(bs, func(s sample) {
				counts := map[string]int{}
				for i := range s.l {
					counts[s.l[i].Name]++
				}
				for name, count := range counts {
					require.Equal(t, 1, count, "label '%s' is repeated %d times", name, count)
				}
			})

			ups := countMetricWithValue(t, bs, labels.FromStrings("__name__", "up", "job", "test"), 0.0)
			require.True(t, ups > 0, `found zero samples for up{job="test"} = 0`)
		},
	}
}

// EmptyLabelsTests exports a single, constant metric with an empty labels
// and checks that we receive the metrics without said label.
func EmptyLabelsTest() Test {
	return Test{
		Name: "EmptyLabels",
		Metrics: staticHandler([]byte(`
# HELP test A gauge
# TYPE test gauge
test{a=""} 1.0
`)),
		Expected: func(t *testing.T, bs []Batch) {
			forAllSamples(bs, func(s sample) {
				for i := range s.l {
					require.NotEmpty(t, s.l[i].Value, "'%s' contains empty labels", s.l.String())
				}
			})

			tests := countMetricWithValue(t, bs, labels.FromStrings("__name__", "test"), 1.0)
			require.True(t, tests > 0, `found zero samples for {"__name__"="test"}`)
		},
	}
}

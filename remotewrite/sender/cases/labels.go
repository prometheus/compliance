package cases

import (
	"sort"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
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
// and checks that we don't receive metrics any metrics - the scrape should fail.
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

			tests := countMetricWithValue(t, bs, labels.FromStrings("__name__", "test", "a", "1"), 1.0)
			require.True(t, tests == 0, `found samples for test{a="1"}, none expected`)
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

// NameLabelTests exports a single, constant metric with no name label
// and checks that we don't receive metrics without a name label - the scape should fail.
func NameLabelTest() Test {
	return Test{
		Name: "NameLabel",
		Metrics: staticHandler([]byte(`
# HELP test A gauge
# TYPE test gauge
{label="value"} 1.0
`)),
		Expected: func(t *testing.T, bs []Batch) {
			forAllSamples(bs, func(s sample) {
				for i := range s.l {
					if s.l[i].Name == "__name__" {
						return
					}
				}

				require.True(t, false, "metric '%s' is missing name label", s.l.String())
			})

			samples := countMetricWithValue(t, bs, labels.FromStrings("label", "value"), 1.0)
			require.True(t, samples == 0, `found non-zero samples for {label="value"} = 1.0`)
		},
	}
}

// HonorLabels exports a single, constant metric with a job label
// and checks that we receive metrics a exported_job label.
func HonorLabelsTest() Test {
	return Test{
		Name: "HonorLabels",
		Metrics: staticHandler([]byte(`
# HELP test A gauge
# TYPE test gauge
test{job="original", instance="foo"} 1.0
`)),
		Expected: func(t *testing.T, bs []Batch) {
			samples := countMetricWithValue(t, bs, labels.FromStrings("__name__", "test", "exported_job", "original", "exported_instance", "foo"), 1.0)
			require.Greater(t, samples, 0, `found zero samples for test{exported_job="original"} = 1.0`)
		},
	}
}

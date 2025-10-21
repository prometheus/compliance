package cases

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

// labelMustMatch checks that a given label matches a given pattern
// on every sample.
func labelMustMatch(t *testing.T, bs []Batch, label, pattern string) {
	forAllSamples(bs, func(s sample) {
		found := false
		s.l.Range(func(l labels.Label) {
			if l.Name == label {
				require.Regexp(t, pattern, l.Value)
				found = true
			}
		})
		require.True(t, found, "label '%s' not found", label)
	})
}

// countMetricWithValue counts all samples with the given labels and value.
// NB we looks for samples with labels that are a subset of the required labels,
// and we fail if we find samples with those labels but different values.
func countMetricWithValue(t *testing.T, bs []Batch, ls labels.Labels, value float64) int {
	return countMetricWithValueFn(bs, ls, func(_ int64, v float64) bool {
		require.Equal(t, value, v)
		return true
	})
}

// countMetricWithValueFn counts all samples with the given labels.
// NB we looks for samples with labels that are a subset of the required labels,
// and we pass the timestamp and value to a function for checking.
func countMetricWithValueFn(bs []Batch, ls labels.Labels, valueFn func(int64, float64) bool) int {
	count := 0
	forAllSamples(bs, func(s sample) {
		if labelsContain(s.l, ls) && valueFn(s.t, s.v) {
			count++
		}
	})
	return count
}

// forAllSamples calls f on all samples in bs.
func forAllSamples(bs []Batch, f func(s sample)) {
	for _, b := range bs {
		for _, s := range b.samples {
			f(s)
		}
	}
}

// labelsContain returns true if inner is a subset of outer.
func labelsContain(outer, inner labels.Labels) bool {
	// Check that every label in inner exists in outer with the same value
	allFound := true
	inner.Range(func(innerLabel labels.Label) {
		outerValue := outer.Get(innerLabel.Name)
		if outerValue != innerLabel.Value {
			allFound = false
		}
	})
	return allFound
}

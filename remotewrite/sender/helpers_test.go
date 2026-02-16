// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"sort"
	"strings"
	"testing"

	writev1 "github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

// extractLabels extracts labels from a TimeSeries using the symbol table.
func extractLabels(ts *writev2.TimeSeries, symbols []string) map[string]string {
	labels := make(map[string]string)
	refs := ts.LabelsRefs

	// Labels are stored as pairs: [key_ref, value_ref, key_ref, value_ref, ...].
	for i := 0; i < len(refs); i += 2 {
		if i+1 >= len(refs) {
			break
		}
		keyRef := refs[i]
		valueRef := refs[i+1]

		// Validate symbol indices.
		if int(keyRef) >= len(symbols) || int(valueRef) >= len(symbols) {
			continue
		}

		key := symbols[keyRef]
		value := symbols[valueRef]
		labels[key] = value
	}
	return labels
}

// extractExemplarLabels extracts labels from an Exemplar using the symbol table.
func extractExemplarLabels(ex *writev2.Exemplar, symbols []string) map[string]string {
	labels := make(map[string]string)
	refs := ex.LabelsRefs

	for i := 0; i < len(refs); i += 2 {
		if i+1 >= len(refs) {
			break
		}
		keyRef := refs[i]
		valueRef := refs[i+1]

		if int(keyRef) >= len(symbols) || int(valueRef) >= len(symbols) {
			continue
		}

		key := symbols[keyRef]
		value := symbols[valueRef]
		labels[key] = value
	}

	return labels
}

// isSorted checks if label names are sorted lexicographically.
func isSorted(symbols []string, refs []uint32) bool {
	var prevKey string
	for i := 0; i < len(refs); i += 2 {
		keyRef := refs[i]
		if int(keyRef) >= len(symbols) {
			return false
		}
		key := symbols[keyRef]
		if prevKey != "" && key <= prevKey {
			return false
		}
		prevKey = key
	}
	return true
}

// isSortedRW1 checks if label names are sorted lexicographically.
func isSortedRW1(labels []writev1.Label) bool {
	return sort.SliceIsSorted(labels, func(i, j int) bool {
		return strings.Compare(labels[i].Name, labels[j].Name) < 0
	})
}

func TestIsSorted(t *testing.T) {
	symbols := []string{"", "a", "c", "b", "x", "__name__"}
	require.True(t, isSorted(symbols, []uint32{5, 1, 1, 1, 3, 1, 2, 1, 4, 1}))
	require.False(t, isSorted(symbols, []uint32{5, 1, 1, 1, 2, 1, 3, 1, 4, 1}))

	require.True(t, isSortedRW1([]writev1.Label{{Name: "__name__"}, {Name: "a"}, {Name: "b"}, {Name: "c"}}))
	require.False(t, isSortedRW1([]writev1.Label{{Name: "__name__"}, {Name: "a"}, {Name: "x"}, {Name: "c"}}))
	require.False(t, isSortedRW1([]writev1.Label{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "__name__"}}))
}

// findTimeseriesByMetricName finds a timeseries by metric name from a captured request.
func findTimeseriesByMetricName(req *writev2.Request, metricName string) (*writev2.TimeSeries, map[string]string) {
	for i := range req.Timeseries {
		ts := &req.Timeseries[i]
		labels := extractLabels(ts, req.Symbols)
		if labels["__name__"] == metricName {
			return ts, labels
		}
	}
	return nil, nil
}

// requireTimeseriesByMetricName finds a timeseries by metric name and fails the test if not found.
func requireTimeseriesByMetricName(t *testing.T, req *writev2.Request, metricName string) (*writev2.TimeSeries, map[string]string) {
	t.Helper()
	ts, labels := findTimeseriesByMetricName(req, metricName)
	require.NotNil(t, ts, "Timeseries with metric name %q must be present", metricName)
	return ts, labels
}

// requireTimeseriesRW1ByMetricName finds a timeseries by metric name and fails the test if not found.
func requireTimeseriesRW1ByMetricName(t *testing.T, req *writev1.WriteRequest, metricName string) *writev1.TimeSeries {
	t.Helper()

	for i := range req.Timeseries {
		for _, l := range req.Timeseries[i].Labels {
			if l.Name == "__name__" && l.Value == metricName {
				return &req.Timeseries[i]
			}
		}
	}
	t.Fatalf("Timeseries with metric name %q must be present", metricName)
	return nil
}

// findHistogramData attempts to find histogram data in both classic and native formats.
// Returns (classicFound, nativeTS) where:
//   - classicFound: true if classic histogram metrics (_count, _sum, _bucket) are found
//   - nativeTS: pointer to timeseries containing native histogram, or nil if not found
func findHistogramData(req *writev2.Request, baseName string) (classicFound bool, nativeTS *writev2.TimeSeries) {
	for i := range req.Timeseries {
		ts := &req.Timeseries[i]
		labels := extractLabels(ts, req.Symbols)
		metricName := labels["__name__"]

		// Check for classic histogram components.
		if metricName == baseName+"_count" || metricName == baseName+"_sum" || metricName == baseName+"_bucket" {
			classicFound = true
		}

		// Check for native histogram format.
		if metricName == baseName && len(ts.Histograms) > 0 {
			nativeTS = ts
		}
	}
	return classicFound, nativeTS
}

// extractHistogramCount extracts count from either classic or native histogram format.
// Returns (count, found) where found indicates if count was successfully extracted.
func extractHistogramCount(req *writev2.Request, baseName string) (float64, bool) {
	// Try classic format first.
	ts, _ := findTimeseriesByMetricName(req, baseName+"_count")
	if ts != nil && len(ts.Samples) > 0 {
		return ts.Samples[0].Value, true
	}

	// Try native format.
	ts, _ = findTimeseriesByMetricName(req, baseName)
	if ts != nil && len(ts.Histograms) > 0 {
		hist := ts.Histograms[0]
		if hist.Count != nil {
			if countInt, ok := hist.Count.(*writev2.Histogram_CountInt); ok {
				return float64(countInt.CountInt), true
			} else if countFloat, ok := hist.Count.(*writev2.Histogram_CountFloat); ok {
				return countFloat.CountFloat, true
			}
		}
	}

	return 0, false
}

// extractHistogramSum extracts sum from either classic or native histogram format.
func extractHistogramSum(req *writev2.Request, baseName string) (float64, bool) {
	// Try classic format first.
	ts, _ := findTimeseriesByMetricName(req, baseName+"_sum")
	if ts != nil && len(ts.Samples) > 0 {
		return ts.Samples[0].Value, true
	}

	// Try native format.
	ts, _ = findTimeseriesByMetricName(req, baseName)
	if ts != nil && len(ts.Histograms) > 0 {
		return ts.Histograms[0].Sum, true
	}

	return 0, false
}

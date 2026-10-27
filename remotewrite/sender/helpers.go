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

package sender

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

type timeseriesResult struct {
	TimeSeries *writev2.TimeSeries
	Labels     map[string]string
}

func flattenTimeseriesResult(t testing.TB, results []timeseriesResult) (ret timeseriesResult) {
	require.NotEmpty(t, results)

	var lbls map[string]string
	for _, r := range results {
		if lbls == nil {
			lbls = r.Labels
		} else {
			require.Equal(t, lbls, r.Labels, "found two different series with the same name; expected same series")
		}
		if ret.TimeSeries == nil {
			ret.TimeSeries = r.TimeSeries
		} else {
			ret.TimeSeries.Samples = append(ret.TimeSeries.Samples, r.TimeSeries.Samples...)
			ret.TimeSeries.Histograms = append(ret.TimeSeries.Histograms, r.TimeSeries.Histograms...)
			ret.TimeSeries.Exemplars = append(ret.TimeSeries.Exemplars, r.TimeSeries.Exemplars...)
		}
	}
	return ret
}

// findTimeseriesByMetricName finds a timeseries by metric name from a captured request.
func findTimeseriesByMetricName(req *writev2.Request, metricName string) []timeseriesResult {
	var results []timeseriesResult
	for i := range req.Timeseries {
		ts := &req.Timeseries[i]
		labels := extractLabels(ts, req.Symbols)
		if labels["__name__"] == metricName {
			results = append(results, timeseriesResult{TimeSeries: ts, Labels: labels})
		}
	}
	return results
}

// requireTimeseriesByMetricName finds a timeseries by metric name and fails the test if not found.
func requireTimeseriesByMetricName(t *testing.T, req *writev2.Request, metricName string) []timeseriesResult {
	t.Helper()
	results := findTimeseriesByMetricName(req, metricName)
	require.NotEmpty(t, results, "Timeseries with metric name %q must be present", metricName)
	return results
}

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
	"bytes"
	"encoding/binary"
	"sort"
	"strings"
	"testing"
	"time"

	writev1 "github.com/prometheus/prometheus/prompb"
	dto "github.com/prometheus/prometheus/prompb/io/prometheus/client"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

// extractLabels extracts labels from a TimeSeries using the symbol table.
func extractLabels(ts *writev2.TimeSeries, symbols []string) map[string]string {
	return extractRefsLabels(ts.LabelsRefs, symbols)
}

// extractExemplarLabels extracts labels from an Exemplar using the symbol table.
func extractExemplarLabels(ex *writev2.Exemplar, symbols []string) map[string]string {
	return extractRefsLabels(ex.LabelsRefs, symbols)
}

func extractRefsLabels(refs []uint32, symbols []string) map[string]string {
	labels := make(map[string]string, len(refs)/2)
	// Labels are stored as pairs: [key_ref, value_ref, key_ref, value_ref, ...].
	for i := 0; i+1 < len(refs); i += 2 {
		keyRef := refs[i]
		valueRef := refs[i+1]
		if int(keyRef) >= len(symbols) || int(valueRef) >= len(symbols) {
			continue
		}
		labels[symbols[keyRef]] = symbols[valueRef]
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
			cp := *r.TimeSeries
			cp.Samples = append([]writev2.Sample(nil), r.TimeSeries.Samples...)
			cp.Histograms = append([]writev2.Histogram(nil), r.TimeSeries.Histograms...)
			cp.Exemplars = append([]writev2.Exemplar(nil), r.TimeSeries.Exemplars...)
			ret.TimeSeries = &cp
			ret.Labels = r.Labels
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

// findHistogramData finds the first native histogram for a metric name in a request.
func findHistogramData(req *writev2.Request, metricName string) *writev2.Histogram {
	results := findTimeseriesByMetricName(req, metricName)
	for _, r := range results {
		if len(r.TimeSeries.Histograms) > 0 {
			return &r.TimeSeries.Histograms[0]
		}
	}
	return nil
}

// extractHistogramCount extracts the total observation count from a native histogram.
func extractHistogramCount(h *writev2.Histogram) float64 {
	switch c := h.Count.(type) {
	case *writev2.Histogram_CountInt:
		return float64(c.CountInt)
	case *writev2.Histogram_CountFloat:
		return c.CountFloat
	default:
		return 0
	}
}

// extractHistogramSum extracts the sum of observations from a native histogram.
func extractHistogramSum(h *writev2.Histogram) float64 {
	return h.Sum
}

// metricFamiliesToProtobuf serializes MetricFamily proto messages using length-delimited encoding
// for the PrometheusProto exposition format.
func metricFamiliesToProtobuf(families []dto.MetricFamily) string {
	var buf bytes.Buffer
	var varintBuf [binary.MaxVarintLen64]byte
	for i := range families {
		data, err := families[i].Marshal()
		if err != nil {
			panic(err)
		}
		n := binary.PutUvarint(varintBuf[:], uint64(len(data)))
		buf.Write(varintBuf[:n])
		buf.Write(data)
	}
	return buf.String()
}

// withCreatedTimestamp sets the created/start timestamp (field 15) on h to ts and returns h.
func withCreatedTimestamp(h *dto.Histogram, ts time.Time) *dto.Histogram {
	// Field 15 (wire type 2: length-delimited) in dto.Histogram is created_timestamp/start_timestamp
	// (google.protobuf.Timestamp: field 1 varint seconds, field 2 varint nanos). Unmarshaling directly into h
	// merges field 15 without referencing the field by name (renamed from CreatedTimestamp to StartTimestamp)
	// or importing the indirect github.com/gogo/protobuf/types package.
	var tsBuf [binary.MaxVarintLen64 * 3]byte
	n := 0
	tsBuf[n] = 1 << 3
	n++
	n += binary.PutUvarint(tsBuf[n:], uint64(ts.Unix()))
	if nanos := ts.Nanosecond(); nanos != 0 {
		tsBuf[n] = 2 << 3
		n++
		n += binary.PutUvarint(tsBuf[n:], uint64(nanos))
	}

	var msgBuf [binary.MaxVarintLen64 * 4]byte
	m := 0
	msgBuf[m] = (15 << 3) | 2
	m++
	m += binary.PutUvarint(msgBuf[m:], uint64(n))
	copy(msgBuf[m:], tsBuf[:n])
	m += n

	if err := h.Unmarshal(msgBuf[:m]); err != nil {
		panic(err)
	}
	return h
}

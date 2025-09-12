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

// Package main provides compliance testing helpers for Prometheus remote write v2 receivers.
package main

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

// ticker provides a global ticker for timing-based operations in tests.
var ticker = time.NewTicker(10 * time.Millisecond)

// labelsMatch checks if two label maps have the same key-value pairs.
func labelsMatch(metric model.Metric, labels map[string]string) bool {
	if len(metric) != len(labels) {
		return false
	}
	for k, v := range labels {
		if metricVal, exists := metric[model.LabelName(k)]; !exists || string(metricVal) != v {
			return false
		}
	}
	return true
}

// mapToMetric converts a map[string]string to model.Metric.
func mapToMetric(labels map[string]string) model.Metric {
	metric := make(model.Metric)
	for k, v := range labels {
		metric[model.LabelName(k)] = model.LabelValue(v)
	}
	return metric
}

type HistogramWithLabels struct {
	Labels           map[string]string
	Histogram        writev2.Histogram
	Offset           time.Duration
	CreatedTimestamp *time.Time
}

type SampleWithLabels struct {
	Labels           map[string]string
	Value            float64
	Offset           time.Duration
	CreatedTimestamp *time.Time
}

// getHeaderValue extracts and parses the X-Prometheus-Remote-Write header value for a given key.
func getHeaderValue(t *testing.T, header http.Header, key string) int {
	fullHeaderName := "X-Prometheus-Remote-Write-" + key + "-Written"
	v := header.Get(fullHeaderName)
	if v == "" {
		// Senders CAN assume that any missing X-Prometheus-Remote-Write-*-Written
		// response header means no element from this category (e.g. Sample)
		// was written by the Receiver (count of 0).
		return 0
	}
	i, err := strconv.Atoi(v)
	require.NoError(t, err)
	return i
}

// requestParams holds the expected parameters for request validation.
type requestParams struct {
	samples          int
	exemplars        int
	histograms       int
	strict           bool
	success          bool
	retryable        bool // Reserved for future use to indicate if the request should be retried on failure.
	exactReponseCode int
}

// validateResponse validates an HTTP response against expected request parameters.
func validateResponse(t *testing.T, params requestParams, resp *http.Response) {
	t.Helper()
	var (
		samplesWritten    = getHeaderValue(t, resp.Header, "Samples")
		exemplarsWritten  = getHeaderValue(t, resp.Header, "Exemplars")
		histogramsWritten = getHeaderValue(t, resp.Header, "Histograms")
	)
	if params.exactReponseCode != 0 {
		require.Equal(t, params.exactReponseCode, resp.StatusCode, "Response code should be exactly %d", params.exactReponseCode)
	}
	switch resp.StatusCode / 100 {
	case 2:
		// HTTP 2xx.
		require.True(t, params.success, "Response code is %d but success is false", resp.StatusCode)
		require.Equal(t, params.samples, samplesWritten, "%d samples written", samplesWritten)
		require.Equal(t, params.exemplars, exemplarsWritten, "%d exemplars written", exemplarsWritten)
		require.Equal(t, params.histograms, histogramsWritten, "%d histograms written", histogramsWritten)
		if params.strict {
			require.Equal(t, http.StatusNoContent, resp.StatusCode, "Response code is %d", resp.StatusCode)
		}
	case 4:
		// HTTP 4xx.
		require.False(t, params.retryable, "Response code is %d but retryable is true", resp.StatusCode)
		if params.exactReponseCode == 0 {
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response code should be exactly %d", http.StatusBadRequest)
		}
		require.GreaterOrEqual(t, params.samples, samplesWritten, "%d samples written", samplesWritten)
		require.GreaterOrEqual(t, params.exemplars, exemplarsWritten, "%d exemplars written", exemplarsWritten)
		require.GreaterOrEqual(t, params.histograms, histogramsWritten, "%d histograms written", histogramsWritten)
		require.GreaterOrEqual(t, params.samples+params.exemplars+params.histograms, samplesWritten+exemplarsWritten+histogramsWritten, "Total written")
	case 5:
		// HTTP 5xx.
		require.False(t, params.success, "Response code is %d but success is true", resp.StatusCode)
		require.True(t, params.retryable, "Response code is %d but retryable is false", resp.StatusCode)
		require.GreaterOrEqual(t, params.samples, samplesWritten, "%d samples written", samplesWritten)
		require.GreaterOrEqual(t, params.exemplars, exemplarsWritten, "%d exemplars written", exemplarsWritten)
		require.GreaterOrEqual(t, params.histograms, histogramsWritten, "%d histograms written", histogramsWritten)
		require.GreaterOrEqual(t, params.samples+params.exemplars+params.histograms, samplesWritten+exemplarsWritten+histogramsWritten, "Total written")
	default:
		require.Fail(t, "Response code is %d but should be 200, 400, or 500", resp.StatusCode)
	}
}

// ExemplarWithLabels represents an exemplar with its metric and exemplar labels.
type ExemplarWithLabels struct {
	Labels         map[string]string
	ExemplarLabels map[string]string
	Value          float64
	Offset         time.Duration
}

// MetadataWithLabels represents metric metadata with its associated labels.
type MetadataWithLabels struct {
	Labels map[string]string
	Type   writev2.Metadata_MetricType
	Help   string
	Unit   string
}

// RequestOpts contains all data required for generating a remote write request.
type RequestOpts struct {
	Samples       []SampleWithLabels
	Exemplars     []ExemplarWithLabels
	Metadata      []MetadataWithLabels
	Histograms    []HistogramWithLabels
	UnsafeRequest bool
}

// generateRequest generates an HTTP request with the given options.
func generateRequest(opts RequestOpts) *http.Request {
	<-ticker.C
	now := time.Now()

	// Perform safety checks unless UnsafeRequest is true.
	if !opts.UnsafeRequest {
		// Check: No Samples and Histograms in the same request.
		if len(opts.Samples) > 0 && len(opts.Histograms) > 0 {
			panic("cannot have both Samples and Histograms in the same request")
		}

		// Check: All exemplars are attached to samples or histograms (have same label set).
		for _, exemplar := range opts.Exemplars {
			found := false
			// Check against samples.
			for _, sample := range opts.Samples {
				if labelsMatch(mapToMetric(sample.Labels), exemplar.Labels) {
					found = true
					break
				}
			}
			// Check against histograms if not found in samples.
			if !found {
				for _, histogram := range opts.Histograms {
					if labelsMatch(mapToMetric(histogram.Labels), exemplar.Labels) {
						found = true
						break
					}
				}
			}
			if !found {
				panic("exemplar has no matching sample or histogram with same label set")
			}
		}

		// Check: All metadata are attached to samples or histograms (have same metric name).
		for _, metadata := range opts.Metadata {
			metadataName := metadata.Labels["__name__"]
			if metadataName == "" {
				continue // Skip metadata without __name__.
			}
			found := false
			// Check against samples.
			for _, sample := range opts.Samples {
				if sample.Labels["__name__"] == metadataName {
					found = true
					break
				}
			}
			// Check against histograms if not found in samples.
			if !found {
				for _, histogram := range opts.Histograms {
					if histogram.Labels["__name__"] == metadataName {
						found = true
						break
					}
				}
			}
			if !found {
				panic("metadata has no matching sample or histogram with same metric name")
			}
		}
	}

	symbols := writev2.NewSymbolTable()

	var timeseries []writev2.TimeSeries

	// Build metadata lookup by metric name for fast access.
	metadataByName := make(map[string]writev2.Metadata)
	for _, mw := range opts.Metadata {
		metricName := mw.Labels["__name__"]
		if metricName != "" {
			metadataByName[metricName] = writev2.Metadata{
				Type:    mw.Type,
				HelpRef: symbols.Symbolize(mw.Help),
				UnitRef: symbols.Symbolize(mw.Unit),
			}
		}
	}

	// Pre-process samples to identify the last sample for each series.
	seriesLastSample := make(map[string]int) // series key -> last sample index.
	for i, s := range opts.Samples {
		key := mapToMetric(s.Labels).String()
		seriesLastSample[key] = i // This will end up with the highest index for each series.
	}

	// Track which exemplars have been used to avoid duplication.
	usedExemplars := make(map[int]bool)

	for i, s := range opts.Samples {
		var labelRefs []uint32
		for k, v := range s.Labels {
			labelRefs = append(labelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
		}

		// Check if there are any exemplars for this sample.
		var sampleExemplars []writev2.Exemplar
		seriesKey := mapToMetric(s.Labels).String()
		isLastSampleForSeries := (seriesLastSample[seriesKey] == i)

		for ei, ew := range opts.Exemplars {
			// Skip if exemplar already used.
			if usedExemplars[ei] {
				continue
			}

			// Match exemplar with sample by labels.
			match := labelsMatch(mapToMetric(s.Labels), ew.Labels)
			if match {
				var exemplarLabelRefs []uint32
				for k, v := range ew.ExemplarLabels {
					exemplarLabelRefs = append(exemplarLabelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
				}

				exemplar := writev2.Exemplar{
					LabelsRefs: exemplarLabelRefs,
					Value:      ew.Value,
					Timestamp:  now.Add(ew.Offset).UnixMilli(),
				}
				sampleExemplars = append(sampleExemplars, exemplar)
				usedExemplars[ei] = true

				// If this is not the last sample for this series, only attach one exemplar.
				if !isLastSampleForSeries {
					break
				}
				// If this is the last sample for this series, continue to attach all remaining matching exemplars.
			}
		}

		ts := writev2.TimeSeries{
			LabelsRefs: labelRefs,
			Samples: []writev2.Sample{
				{
					Timestamp: now.Add(s.Offset).UnixMilli(),
					Value:     s.Value,
				},
			},
			Exemplars: sampleExemplars,
		}

		if s.CreatedTimestamp != nil {
			ts.CreatedTimestamp = s.CreatedTimestamp.UnixMilli()
		}

		if metricName := s.Labels["__name__"]; metricName != "" {
			if metadata, found := metadataByName[metricName]; found {
				ts.Metadata = metadata
			}
		}

		timeseries = append(timeseries, ts)
	}

	// Pre-process histograms to identify the last histogram for each series (similar to samples).
	histogramSeriesLastSample := make(map[string]int) // series key -> last histogram index.
	for i, hw := range opts.Histograms {
		key := mapToMetric(hw.Labels).String()
		histogramSeriesLastSample[key] = i
	}

	for i, hw := range opts.Histograms {
		var labelRefs []uint32
		for k, v := range hw.Labels {
			labelRefs = append(labelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
		}

		// Create a copy of the histogram and overwrite its timestamp.
		hist := hw.Histogram
		hist.Timestamp = now.Add(hw.Offset).UnixMilli()

		// Check for exemplars for this histogram.
		var histogramExemplars []writev2.Exemplar
		seriesKey := mapToMetric(hw.Labels).String()
		isLastHistogramForSeries := (histogramSeriesLastSample[seriesKey] == i)

		for ei, ew := range opts.Exemplars {
			// Skip if exemplar already used.
			if usedExemplars[ei] {
				continue
			}

			// Match exemplar with histogram by labels.
			match := labelsMatch(mapToMetric(hw.Labels), ew.Labels)
			if match {
				var exemplarLabelRefs []uint32
				for k, v := range ew.ExemplarLabels {
					exemplarLabelRefs = append(exemplarLabelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
				}

				exemplar := writev2.Exemplar{
					LabelsRefs: exemplarLabelRefs,
					Value:      ew.Value,
					Timestamp:  now.Add(ew.Offset).UnixMilli(),
				}
				histogramExemplars = append(histogramExemplars, exemplar)
				usedExemplars[ei] = true

				// If this is not the last histogram for this series, only attach one exemplar.
				if !isLastHistogramForSeries {
					break
				}
				// If this is the last histogram for this series, continue to attach all remaining matching exemplars.
			}
		}

		ts := writev2.TimeSeries{
			LabelsRefs: labelRefs,
			Histograms: []writev2.Histogram{hist},
			Exemplars:  histogramExemplars,
		}

		if hw.CreatedTimestamp != nil {
			ts.CreatedTimestamp = hw.CreatedTimestamp.UnixMilli()
		}

		if metricName := hw.Labels["__name__"]; metricName != "" {
			if metadata, found := metadataByName[metricName]; found {
				ts.Metadata = metadata
			}
		}

		timeseries = append(timeseries, ts)
	}

	// Add any remaining exemplars that didn't match existing samples as separate timeseries.
	for i, ew := range opts.Exemplars {
		// Skip exemplars that were already distributed to samples or histograms.
		if usedExemplars[i] {
			continue
		}

		matched := false
		for _, s := range opts.Samples {
			if labelsMatch(mapToMetric(s.Labels), ew.Labels) {
				matched = true
				break
			}
		}

		if !matched {
			for _, h := range opts.Histograms {
				if labelsMatch(mapToMetric(h.Labels), ew.Labels) {
					matched = true
					break
				}
			}
		}

		if !matched {
			var labelRefs []uint32
			for k, v := range ew.Labels {
				labelRefs = append(labelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
			}

			var exemplarLabelRefs []uint32
			for k, v := range ew.ExemplarLabels {
				exemplarLabelRefs = append(exemplarLabelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
			}

			exemplar := writev2.Exemplar{
				LabelsRefs: exemplarLabelRefs,
				Value:      ew.Value,
				Timestamp:  now.Add(ew.Offset).UnixMilli(),
			}

			ts := writev2.TimeSeries{
				LabelsRefs: labelRefs,
				Exemplars:  []writev2.Exemplar{exemplar},
			}
			timeseries = append(timeseries, ts)
		}
	}

	// Add any remaining metadata that wasn't attached to samples or histograms as separate timeseries.
	for _, mw := range opts.Metadata {
		metricName := mw.Labels["__name__"]
		if metricName == "" {
			continue // Skip metadata without __name__.
		}

		// Check if this metadata was already attached to a sample or histogram.
		matched := false
		for _, s := range opts.Samples {
			if s.Labels["__name__"] == metricName {
				matched = true
				break
			}
		}

		if !matched {
			for _, h := range opts.Histograms {
				if h.Labels["__name__"] == metricName {
					matched = true
					break
				}
			}
		}

		if !matched {
			var labelRefs []uint32
			for k, v := range mw.Labels {
				labelRefs = append(labelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
			}

			ts := writev2.TimeSeries{
				LabelsRefs: labelRefs,
				Metadata: writev2.Metadata{
					Type:    mw.Type,
					HelpRef: symbols.Symbolize(mw.Help),
					UnitRef: symbols.Symbolize(mw.Unit),
				},
			}
			timeseries = append(timeseries, ts)
		}
	}

	req := &writev2.Request{
		Symbols:    symbols.Symbols(),
		Timeseries: timeseries,
	}

	data, _ := req.Marshal()
	compressed := snappy.Encode(nil, data)

	httpReq := http.Request{
		Method: "POST",
		Header: http.Header{
			"Content-Encoding":                  []string{"snappy"},
			"Content-Type":                      []string{"application/x-protobuf;proto=io.prometheus.write.v2.Request"},
			"X-Prometheus-Remote-Write-Version": []string{"2.0.0"},
		},
		Body: io.NopCloser(bytes.NewReader(compressed)),
	}
	return &httpReq
}

// histogram creates a writev2.Histogram with the specified parameters.
func histogram(sum float64, includePositive, includeNegative, includeZero, useCustomBuckets, badCount bool) writev2.Histogram {
	// Generate spans programmatically based on boolean parameters.
	var positiveSpans []writev2.BucketSpan
	var negativeSpans []writev2.BucketSpan
	var positiveDeltas []int64
	var negativeDeltas []int64
	var customValues []float64
	count := uint64(0)

	if includePositive {
		positiveSpans = []writev2.BucketSpan{
			{Offset: 0, Length: 3},
			{Offset: 2, Length: 2},
		}
		positiveDeltas = []int64{1, 2, 1, 3, 1}
		// Deltas decode to: [1, 3, 4, 7, 8] which sum to 23
		count += 23
	}

	if includeNegative {
		negativeSpans = []writev2.BucketSpan{
			{Offset: 0, Length: 2},
		}
		negativeDeltas = []int64{1, 1}
		// Deltas decode to: [1, 2] which sum to 3
		count += 3
	}

	if includeZero {
		count += 1
	}

	if useCustomBuckets {
		customValues = []float64{0.1, 0.5, 1.0, 2.5, 5.0, 10.0}
		// For custom buckets, we need actual bucket counts that sum to the total.
		// Override any previous spans/deltas since custom buckets use positive spans.
		positiveSpans = []writev2.BucketSpan{
			{Offset: 0, Length: 6}, // 6 custom buckets
		}
		positiveDeltas = []int64{2, 1, 2, 3, -1, -2} // Delta-encoded bucket counts that decode to [2,3,5,8,7,5] and sum to 30.
		count = 30                                   // Total observations across all custom buckets.
	}

	if badCount {
		count = 99
	}

	hist := writev2.Histogram{
		Count: &writev2.Histogram_CountInt{CountInt: count},
		Sum:   sum,
	}

	if includePositive || useCustomBuckets {
		hist.PositiveSpans = positiveSpans
		hist.PositiveDeltas = positiveDeltas
	}

	if includeNegative {
		hist.NegativeSpans = negativeSpans
		hist.NegativeDeltas = negativeDeltas
	}

	if includeZero {
		hist.ZeroCount = &writev2.Histogram_ZeroCountInt{ZeroCountInt: 1}
	}

	if useCustomBuckets {
		hist.Schema = -53
		hist.CustomValues = customValues
	}

	return hist
}

// should marks a test as having a "SHOULD" RFC compliance level.
func should(t *testing.T) {
	t.Attr("rfcLevel", "SHOULD")
}

// must marks a test as having a "MUST" RFC compliance level.
func must(t *testing.T) {
	t.Attr("rfcLevel", "MUST")
}

// may marks a test as having a "MAY" RFC compliance level.
func may(t *testing.T) {
	t.Attr("rfcLevel", "MAY")
}

// testJobInstanceLabels returns the commonly used test job instance labels.
func testJobInstanceLabels() map[string]string {
	return map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9090"}
}

// basicMetric returns basic metric labels with just the metric name.
func basicMetric(name string) map[string]string {
	return map[string]string{"__name__": name}
}

// traceExemplar returns common exemplar labels with a trace ID.
func traceExemplar(traceId string) map[string]string {
	return map[string]string{"trace_id": traceId}
}

// httpAPILabels returns HTTP API labels with the specified method.
func httpAPILabels(method string) map[string]string {
	return map[string]string{"__name__": "http_requests_total", "job": "api", "method": method}
}

// runComplianceTest runs a test case with both MUST and SHOULD RFC compliance levels.
// This function eliminates code duplication between the two variants by automatically
// generating both strict (SHOULD) and basic (MUST) compliance tests for successful cases.
func runComplianceTest(t *testing.T, name string, description string, opts RequestOpts, params requestParams, expectSuccess bool) {
	t.Helper()

	// Run SHOULD test (strict compliance) only for expected successful cases.
	if expectSuccess {
		t.Run(name+" returns 204", func(t *testing.T) {
			should(t)
			t.Attr("description", description)

			// Copy params to avoid mutation.
			shouldParams := params
			shouldParams.success = true
			shouldParams.strict = true

			runRequest(t, generateRequest(opts), shouldParams)
		})
	}

	// Run MUST test (basic compliance) for all cases.
	t.Run(name, func(t *testing.T) {
		must(t)
		t.Attr("description", description)

		// Copy params to avoid mutation.
		mustParams := params
		mustParams.success = expectSuccess
		mustParams.strict = false

		runRequest(t, generateRequest(opts), mustParams)
	})
}

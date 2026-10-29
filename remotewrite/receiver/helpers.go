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

package receiver

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/common/model"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

// mergeExternalLabels merges externalLabels into lbls without overwriting keys already present in lbls.
// Empty label maps are returned unchanged so negative tests for empty label sets remain intact.
func mergeExternalLabels(lbls, externalLabels map[string]string) map[string]string {
	if len(lbls) == 0 || len(externalLabels) == 0 {
		return lbls
	}
	merged := make(map[string]string, len(externalLabels)+len(lbls))
	maps.Copy(merged, externalLabels)
	maps.Copy(merged, lbls)
	return merged
}

// symbolizeSortedLabels symbolizes a series label map with externalLabels merged and
// label names sorted in lexicographical order, as required by the Remote-Write 2.0 specification.
func symbolizeSortedLabels(symbols *writev2.SymbolsTable, lbls, externalLabels map[string]string) []uint32 {
	return symbolizeSortedRawLabels(symbols, mergeExternalLabels(lbls, externalLabels))
}

// symbolizeSortedRawLabels symbolizes a label map without merging externalLabels,
// with label names sorted in lexicographical order. Used for exemplar attachment labels.
func symbolizeSortedRawLabels(symbols *writev2.SymbolsTable, lbls map[string]string) []uint32 {
	if len(lbls) == 0 {
		return nil
	}
	keys := slices.Sorted(maps.Keys(lbls))
	refs := make([]uint32, 0, len(keys)*2)
	for _, k := range keys {
		refs = append(refs, symbols.Symbolize(k), symbols.Symbolize(lbls[k]))
	}
	return refs
}

// ticker paces request generation so sequential test cases receive distinct millisecond timestamps.
var ticker = time.NewTicker(10 * time.Millisecond)

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

func mapToMetric(labels map[string]string) model.Metric {
	metric := make(model.Metric, len(labels))
	for k, v := range labels {
		metric[model.LabelName(k)] = model.LabelValue(v)
	}
	return metric
}

// HistogramWithLabels represents a native histogram sample and its series labels for request generation.
type HistogramWithLabels struct {
	Labels         map[string]string
	Histogram      writev2.Histogram
	Offset         time.Duration
	StartTimestamp *time.Time
}

// SampleWithLabels represents a float sample and its series labels for request generation.
type SampleWithLabels struct {
	Labels         map[string]string
	Value          float64
	Offset         time.Duration
	StartTimestamp *time.Time
}

// ExemplarWithLabels represents an exemplar together with its parent series and attachment labels.
type ExemplarWithLabels struct {
	Labels         map[string]string
	ExemplarLabels map[string]string
	Value          float64
	Offset         time.Duration
}

// MetadataWithLabels represents metric metadata and its target series labels.
type MetadataWithLabels struct {
	Labels map[string]string
	Type   writev2.Metadata_MetricType
	Help   string
	Unit   string
}

// RequestOpts contains the input data used to construct a Remote-Write v2 HTTP request.
type RequestOpts struct {
	Samples       []SampleWithLabels
	Exemplars     []ExemplarWithLabels
	Metadata      []MetadataWithLabels
	Histograms    []HistogramWithLabels
	UnsafeRequest bool
	Mode          remote.WriteMode
	ContentType   string
	RawTimeseries []writev2.TimeSeries
	RawSymbols    []string
}

func getHeaderValue(t *testing.T, header http.Header, key string) int {
	fullHeaderName := "X-Prometheus-Remote-Write-" + key + "-Written"
	v := header.Get(fullHeaderName)
	if v == "" {
		// Senders may assume any missing X-Prometheus-Remote-Write-*-Written header means 0 written.
		return 0
	}
	i, err := strconv.Atoi(v)
	require.NoError(t, err)
	return i
}

type requestParams struct {
	samples           int
	exemplars         int
	histograms        int
	strict            bool
	success           bool
	retryable         bool
	exactResponseCode int
	allowAny4xx       bool
	validateHeaders   func(t *testing.T, header http.Header)
}

func validateResponse(t *testing.T, params requestParams, resp ResponseResult) {
	t.Helper()
	var (
		samplesWritten    = getHeaderValue(t, resp.Header, "Samples")
		exemplarsWritten  = getHeaderValue(t, resp.Header, "Exemplars")
		histogramsWritten = getHeaderValue(t, resp.Header, "Histograms")
	)
	if params.exactResponseCode != 0 {
		require.Equal(t, params.exactResponseCode, resp.StatusCode, "Response code should be exactly %d", params.exactResponseCode)
	}
	switch resp.StatusCode / 100 {
	case 2:
		require.True(t, params.success, "Response code is %d but success is false", resp.StatusCode)
		require.Equal(t, params.samples, samplesWritten, "%d samples written", samplesWritten)
		require.Equal(t, params.exemplars, exemplarsWritten, "%d exemplars written", exemplarsWritten)
		require.Equal(t, params.histograms, histogramsWritten, "%d histograms written", histogramsWritten)
		if params.strict {
			require.Equal(t, http.StatusNoContent, resp.StatusCode, "Response code is %d", resp.StatusCode)
		}
	case 4:
		require.False(t, params.retryable, "Response code is %d but retryable is true", resp.StatusCode)
		if params.exactResponseCode == 0 && !params.allowAny4xx {
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, "Response code should be exactly %d", http.StatusBadRequest)
		}
		require.GreaterOrEqual(t, params.samples, samplesWritten, "%d samples written", samplesWritten)
		require.GreaterOrEqual(t, params.exemplars, exemplarsWritten, "%d exemplars written", exemplarsWritten)
		require.GreaterOrEqual(t, params.histograms, histogramsWritten, "%d histograms written", histogramsWritten)
		require.GreaterOrEqual(t, params.samples+params.exemplars+params.histograms, samplesWritten+exemplarsWritten+histogramsWritten, "Total written")
	case 5:
		require.False(t, params.success, "Response code is %d but success is true", resp.StatusCode)
		require.True(t, params.retryable, "Response code is %d but retryable is false", resp.StatusCode)
		require.GreaterOrEqual(t, params.samples, samplesWritten, "%d samples written", samplesWritten)
		require.GreaterOrEqual(t, params.exemplars, exemplarsWritten, "%d exemplars written", exemplarsWritten)
		require.GreaterOrEqual(t, params.histograms, histogramsWritten, "%d histograms written", histogramsWritten)
		require.GreaterOrEqual(t, params.samples+params.exemplars+params.histograms, samplesWritten+exemplarsWritten+histogramsWritten, "Total written")
	default:
		require.Fail(t, "Response code is %d but should be 2xx, 4xx, or 5xx", resp.StatusCode)
	}
	if params.validateHeaders != nil {
		params.validateHeaders(t, resp.Header)
	}
}

func generateRequest(opts RequestOpts, externalLabels map[string]string) (*http.Request, error) {
	<-ticker.C
	now := time.Now()

	if !opts.UnsafeRequest {
		if len(opts.Samples) > 0 && len(opts.Histograms) > 0 {
			return nil, fmt.Errorf("cannot have both Samples and Histograms in the same request")
		}

		for _, exemplar := range opts.Exemplars {
			found := false
			for _, sample := range opts.Samples {
				if labelsMatch(mapToMetric(sample.Labels), exemplar.Labels) {
					found = true
					break
				}
			}
			if !found {
				for _, hist := range opts.Histograms {
					if labelsMatch(mapToMetric(hist.Labels), exemplar.Labels) {
						found = true
						break
					}
				}
			}
			if !found {
				return nil, fmt.Errorf("exemplar has no matching sample or histogram with same label set")
			}
		}

		for _, metadata := range opts.Metadata {
			metadataName := metadata.Labels["__name__"]
			found := false
			for _, sample := range opts.Samples {
				if sample.Labels["__name__"] == metadataName {
					found = true
					break
				}
			}
			if !found {
				for _, hist := range opts.Histograms {
					if hist.Labels["__name__"] == metadataName {
						found = true
						break
					}
				}
			}
			if !found {
				return nil, fmt.Errorf("metadata %q has no matching sample or histogram with same metric name", metadataName)
			}
		}
	}

	symbols := writev2.NewSymbolTable()
	var timeseries []writev2.TimeSeries

	metadataByName := make(map[string]writev2.Metadata)
	for _, mw := range opts.Metadata {
		metricName := mw.Labels["__name__"]
		if metricName != "" || opts.UnsafeRequest {
			metadataByName[metricName] = writev2.Metadata{
				Type:    mw.Type,
				HelpRef: symbols.Symbolize(mw.Help),
				UnitRef: symbols.Symbolize(mw.Unit),
			}
		}
	}

	seriesLastSample := make(map[string]int)
	for i, s := range opts.Samples {
		key := mapToMetric(s.Labels).String()
		seriesLastSample[key] = i
	}

	usedExemplars := make(map[int]bool)

	for i, s := range opts.Samples {
		labelRefs := symbolizeSortedLabels(&symbols, s.Labels, externalLabels)

		var sampleExemplars []writev2.Exemplar
		seriesKey := mapToMetric(s.Labels).String()
		isLastSampleForSeries := seriesLastSample[seriesKey] == i

		for ei, ew := range opts.Exemplars {
			if usedExemplars[ei] {
				continue
			}
			if labelsMatch(mapToMetric(s.Labels), ew.Labels) {
				exemplarLabelRefs := symbolizeSortedRawLabels(&symbols, ew.ExemplarLabels)
				sampleExemplars = append(sampleExemplars, writev2.Exemplar{
					LabelsRefs: exemplarLabelRefs,
					Value:      ew.Value,
					Timestamp:  now.Add(ew.Offset).UnixMilli(),
				})
				usedExemplars[ei] = true
				if !isLastSampleForSeries {
					break
				}
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

		if s.StartTimestamp != nil {
			if s.StartTimestamp.IsZero() {
				ts.Samples[0].StartTimestamp = ts.Samples[0].Timestamp
			} else {
				ts.Samples[0].StartTimestamp = s.StartTimestamp.UnixMilli()
			}
		}

		if metricName := s.Labels["__name__"]; metricName != "" || opts.UnsafeRequest {
			if metadata, found := metadataByName[metricName]; found {
				ts.Metadata = metadata
			}
		}

		timeseries = append(timeseries, ts)
	}

	histogramSeriesLastSample := make(map[string]int)
	for i, hw := range opts.Histograms {
		key := mapToMetric(hw.Labels).String()
		histogramSeriesLastSample[key] = i
	}

	for i, hw := range opts.Histograms {
		labelRefs := symbolizeSortedLabels(&symbols, hw.Labels, externalLabels)

		hist := hw.Histogram
		hist.Timestamp = now.Add(hw.Offset).UnixMilli()

		var histogramExemplars []writev2.Exemplar
		seriesKey := mapToMetric(hw.Labels).String()
		isLastHistogramForSeries := histogramSeriesLastSample[seriesKey] == i

		for ei, ew := range opts.Exemplars {
			if usedExemplars[ei] {
				continue
			}
			if labelsMatch(mapToMetric(hw.Labels), ew.Labels) {
				exemplarLabelRefs := symbolizeSortedRawLabels(&symbols, ew.ExemplarLabels)
				histogramExemplars = append(histogramExemplars, writev2.Exemplar{
					LabelsRefs: exemplarLabelRefs,
					Value:      ew.Value,
					Timestamp:  now.Add(ew.Offset).UnixMilli(),
				})
				usedExemplars[ei] = true
				if !isLastHistogramForSeries {
					break
				}
			}
		}

		ts := writev2.TimeSeries{
			LabelsRefs: labelRefs,
			Histograms: []writev2.Histogram{hist},
			Exemplars:  histogramExemplars,
		}

		if hw.StartTimestamp != nil {
			if hw.StartTimestamp.IsZero() {
				ts.Histograms[0].StartTimestamp = ts.Histograms[0].Timestamp
			} else {
				ts.Histograms[0].StartTimestamp = hw.StartTimestamp.UnixMilli()
			}
		}

		if metricName := hw.Labels["__name__"]; metricName != "" || opts.UnsafeRequest {
			if metadata, found := metadataByName[metricName]; found {
				ts.Metadata = metadata
			}
		}

		timeseries = append(timeseries, ts)
	}

	for i, ew := range opts.Exemplars {
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
			labelRefs := symbolizeSortedLabels(&symbols, ew.Labels, externalLabels)
			exemplarLabelRefs := symbolizeSortedRawLabels(&symbols, ew.ExemplarLabels)
			timeseries = append(timeseries, writev2.TimeSeries{
				LabelsRefs: labelRefs,
				Exemplars: []writev2.Exemplar{
					{
						LabelsRefs: exemplarLabelRefs,
						Value:      ew.Value,
						Timestamp:  now.Add(ew.Offset).UnixMilli(),
					},
				},
			})
		}
	}

	for _, mw := range opts.Metadata {
		metricName := mw.Labels["__name__"]
		if metricName == "" {
			continue
		}
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
			labelRefs := symbolizeSortedLabels(&symbols, mw.Labels, externalLabels)
			timeseries = append(timeseries, writev2.TimeSeries{
				LabelsRefs: labelRefs,
				Metadata: writev2.Metadata{
					Type:    mw.Type,
					HelpRef: symbols.Symbolize(mw.Help),
					UnitRef: symbols.Symbolize(mw.Unit),
				},
			})
		}
	}

	if len(opts.RawTimeseries) > 0 {
		timeseries = append(timeseries, opts.RawTimeseries...)
	}

	reqSymbols := symbols.Symbols()
	if opts.RawSymbols != nil {
		reqSymbols = opts.RawSymbols
	}

	req := &writev2.Request{
		Symbols:    reqSymbols,
		Timeseries: timeseries,
	}

	data, err := req.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal writev2 request: %w", err)
	}
	compressed := snappy.Encode(nil, data)

	contentType := opts.ContentType
	if contentType == "" {
		contentType = remote.ContentTypeHeader(remote.WriteV2MessageType, opts.Mode)
	}

	return &http.Request{
		Method: http.MethodPost,
		Header: http.Header{
			"Content-Encoding":                  []string{"snappy"},
			"Content-Type":                      []string{contentType},
			"X-Prometheus-Remote-Write-Version": []string{"2.0.0"},
		},
		Body: io.NopCloser(bytes.NewReader(compressed)),
	}, nil
}

func histogram(sum float64, includePositive, includeNegative, includeZero, useCustomBuckets, badCount bool) writev2.Histogram {
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
		count += 23
	}

	if includeNegative {
		negativeSpans = []writev2.BucketSpan{
			{Offset: 0, Length: 2},
		}
		negativeDeltas = []int64{1, 1}
		count += 3
	}

	if includeZero {
		count += 1
	}

	if useCustomBuckets {
		customValues = []float64{0.1, 0.5, 1.0, 2.5, 5.0, 10.0}
		positiveSpans = []writev2.BucketSpan{
			{Offset: 0, Length: 6},
		}
		positiveDeltas = []int64{2, 1, 2, 3, -1, -2}
		count = 30
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

func floatHistogram(sum float64, includePositive, includeNegative, includeZero, useCustomBuckets, badCount bool) writev2.Histogram {
	hist := histogram(sum, includePositive, includeNegative, includeZero, useCustomBuckets, false)

	deltasToCounts := func(deltas []int64) []float64 {
		if len(deltas) == 0 {
			return nil
		}
		counts := make([]float64, len(deltas))
		var curr int64
		for i, d := range deltas {
			curr += d
			counts[i] = float64(curr)
		}
		return counts
	}

	hist.PositiveCounts = deltasToCounts(hist.PositiveDeltas)
	hist.PositiveDeltas = nil
	hist.NegativeCounts = deltasToCounts(hist.NegativeDeltas)
	hist.NegativeDeltas = nil

	if includeZero {
		hist.ZeroCount = &writev2.Histogram_ZeroCountFloat{ZeroCountFloat: float64(hist.GetZeroCountInt())}
	}

	count := float64(hist.GetCountInt())
	if badCount {
		count = -1.0
	}
	hist.Count = &writev2.Histogram_CountFloat{CountFloat: count}

	return hist
}

func testJobInstanceLabels() map[string]string {
	return map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9090"}
}

func basicMetric(name string) map[string]string {
	return map[string]string{"__name__": name}
}

func traceExemplar(traceID string) map[string]string {
	return map[string]string{"trace_id": traceID}
}

var slugReplacer = strings.NewReplacer(" ", "_", ",", "")

func slugify(s string) string {
	return slugReplacer.Replace(s)
}

// complianceTest constructs a Test from a dynamic RequestOpts builder.
// Expected-success cases validate both MUST (2xx status and written count headers)
// and SHOULD (204 No Content) against a single HTTP request execution.
func complianceTest(name, description string, buildOpts func() RequestOpts, params requestParams, expectSuccess bool) Test {
	buildReq := func(externalLabels map[string]string) (*http.Request, error) {
		return generateRequest(buildOpts(), externalLabels)
	}

	if !expectSuccess {
		mustParams := params
		mustParams.success = false
		mustParams.strict = false
		return Test{
			Name:         name,
			Description:  description,
			RFCLevel:     MustLevel,
			BuildRequest: buildReq,
			Validate: func(t *testing.T, res ResponseResult) {
				validateResponse(t, mustParams, res)
			},
		}
	}

	mustParams := params
	mustParams.success = true
	mustParams.strict = false

	shouldParams := params
	shouldParams.success = true
	shouldParams.strict = true

	return Test{
		Name:         name,
		Description:  description,
		BuildRequest: buildReq,
		ValidateCases: []ValidateCase{
			{
				Name:        "accepts_request",
				Description: description,
				RFCLevel:    MustLevel,
				Validate: func(t *testing.T, res ResponseResult) {
					validateResponse(t, mustParams, res)
				},
			},
			{
				Name:        "returns_204",
				Description: description + " (204 No Content)",
				RFCLevel:    ShouldLevel,
				Validate: func(t *testing.T, res ResponseResult) {
					validateResponse(t, shouldParams, res)
				},
			},
		},
	}
}

// staticComplianceTest wraps a constant RequestOpts into complianceTest.
func staticComplianceTest(name, description string, opts RequestOpts, params requestParams, expectSuccess bool) Test {
	return complianceTest(name, description, func() RequestOpts { return opts }, params, expectSuccess)
}

// singleRequestTest constructs a Test with a single RFC requirement level.
func singleRequestTest(name, description string, rfc RFCLevel, buildReq RequestBuilder, params requestParams) Test {
	return Test{
		Name:         name,
		Description:  description,
		RFCLevel:     rfc,
		BuildRequest: buildReq,
		Validate: func(t *testing.T, res ResponseResult) {
			validateResponse(t, params, res)
		},
	}
}

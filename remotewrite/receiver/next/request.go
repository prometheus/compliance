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
	"io"
	"net/http"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/common/model"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

// SampleWithLabels describes a single float sample to send, along with its labels.
type SampleWithLabels struct {
	Labels           map[string]string
	Value            float64
	Offset           time.Duration
	CreatedTimestamp *time.Time
}

// HistogramWithLabels describes a single native histogram to send, along with its labels.
type HistogramWithLabels struct {
	Labels           map[string]string
	Histogram        writev2.Histogram
	Offset           time.Duration
	CreatedTimestamp *time.Time
}

// ExemplarWithLabels describes a single exemplar to send, along with its metric and exemplar labels.
type ExemplarWithLabels struct {
	Labels         map[string]string
	ExemplarLabels map[string]string
	Value          float64
	Offset         time.Duration
}

// MetadataWithLabels describes metric metadata to send, along with the labels of the metric it applies to.
type MetadataWithLabels struct {
	Labels map[string]string
	Type   writev2.Metadata_MetricType
	Help   string
	Unit   string
}

// RequestOpts contains all data required for generating a Remote Write v2 request.
type RequestOpts struct {
	Samples       []SampleWithLabels
	Exemplars     []ExemplarWithLabels
	Metadata      []MetadataWithLabels
	Histograms    []HistogramWithLabels
	UnsafeRequest bool
}

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
	metric := make(model.Metric)
	for k, v := range labels {
		metric[model.LabelName(k)] = model.LabelValue(v)
	}
	return metric
}

// generateRequest generates a snappy-compressed Remote Write v2 HTTP request from opts.
func generateRequest(opts RequestOpts) *http.Request {
	now := time.Now()

	if !opts.UnsafeRequest {
		if len(opts.Samples) > 0 && len(opts.Histograms) > 0 {
			panic("cannot have both Samples and Histograms in the same request")
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
		var labelRefs []uint32
		for k, v := range s.Labels {
			labelRefs = append(labelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
		}

		var sampleExemplars []writev2.Exemplar
		seriesKey := mapToMetric(s.Labels).String()
		isLastSampleForSeries := seriesLastSample[seriesKey] == i

		for ei, ew := range opts.Exemplars {
			if usedExemplars[ei] {
				continue
			}
			if labelsMatch(mapToMetric(s.Labels), ew.Labels) {
				var exemplarLabelRefs []uint32
				for k, v := range ew.ExemplarLabels {
					exemplarLabelRefs = append(exemplarLabelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
				}
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
				{Timestamp: now.Add(s.Offset).UnixMilli(), Value: s.Value},
			},
			Exemplars: sampleExemplars,
		}
		// NOTE: CreatedTimestamp (start-timestamp) is not wired into the request yet:
		// the pinned github.com/prometheus/prometheus version predates writev2.TimeSeries
		// start-timestamp support. A dependency bump (already tracked via dependabot) is
		// a prerequisite for CounterWithCreatedTimestamp-style tests to be meaningful.
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
		var labelRefs []uint32
		for k, v := range hw.Labels {
			labelRefs = append(labelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
		}

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
				var exemplarLabelRefs []uint32
				for k, v := range ew.ExemplarLabels {
					exemplarLabelRefs = append(exemplarLabelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
				}
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
			var labelRefs []uint32
			for k, v := range ew.Labels {
				labelRefs = append(labelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
			}
			var exemplarLabelRefs []uint32
			for k, v := range ew.ExemplarLabels {
				exemplarLabelRefs = append(exemplarLabelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
			}
			timeseries = append(timeseries, writev2.TimeSeries{
				LabelsRefs: labelRefs,
				Exemplars: []writev2.Exemplar{{
					LabelsRefs: exemplarLabelRefs,
					Value:      ew.Value,
					Timestamp:  now.Add(ew.Offset).UnixMilli(),
				}},
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
			var labelRefs []uint32
			for k, v := range mw.Labels {
				labelRefs = append(labelRefs, symbols.Symbolize(k), symbols.Symbolize(v))
			}
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

	req := &writev2.Request{
		Symbols:    symbols.Symbols(),
		Timeseries: timeseries,
	}
	data, _ := req.Marshal()
	compressed := snappy.Encode(nil, data)

	return &http.Request{
		Method: http.MethodPost,
		Header: http.Header{
			"Content-Encoding":                  []string{"snappy"},
			"Content-Type":                      []string{"application/x-protobuf;proto=io.prometheus.write.v2.Request"},
			"X-Prometheus-Remote-Write-Version": []string{"2.0.0"},
		},
		Body: io.NopCloser(bytes.NewReader(compressed)),
	}
}

// Histogram creates a writev2.Histogram with the specified parameters. Exported so
// ComplianceTests implementations across files can build histogram test fixtures.
func Histogram(sum float64, includePositive, includeNegative, includeZero, useCustomBuckets, badCount bool) writev2.Histogram {
	var positiveSpans, negativeSpans []writev2.BucketSpan
	var positiveDeltas, negativeDeltas []int64
	var customValues []float64
	count := uint64(0)

	if includePositive {
		positiveSpans = []writev2.BucketSpan{{Offset: 0, Length: 3}, {Offset: 2, Length: 2}}
		positiveDeltas = []int64{1, 2, 1, 3, 1}
		count += 23
	}
	if includeNegative {
		negativeSpans = []writev2.BucketSpan{{Offset: 0, Length: 2}}
		negativeDeltas = []int64{1, 1}
		count += 3
	}
	if includeZero {
		count++
	}
	if useCustomBuckets {
		customValues = []float64{0.1, 0.5, 1.0, 2.5, 5.0, 10.0}
		positiveSpans = []writev2.BucketSpan{{Offset: 0, Length: 6}}
		positiveDeltas = []int64{2, 1, 2, 3, -1, -2}
		count = 30
	}
	if badCount {
		count = 99
	}

	hist := writev2.Histogram{Count: &writev2.Histogram_CountInt{CountInt: count}, Sum: sum}
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

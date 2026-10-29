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
	"fmt"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/value"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

type namedFloatValue struct {
	name string
	val  float64
}

var sampleFloatValues = []namedFloatValue{
	{name: "1.0", val: 1.0},
	{name: "StaleNaN", val: math.Float64frombits(value.StaleNaN)},
	{name: "NaN", val: math.Float64frombits(value.NormalNaN)},
	{name: "Inf", val: math.Inf(1)},
	{name: "-Inf", val: math.Inf(-1)},
}

func samplesTests() []Test {
	var tests []Test

	// Single metric samples across float values including StaleNaN, NaN, and Inf.
	for _, fv := range sampleFloatValues {
		v := fv.val
		cases := []struct {
			name    string
			metric  SampleWithLabels
			success bool
		}{
			{
				name:    "simple metric",
				metric:  SampleWithLabels{Labels: basicMetric("up"), Value: v},
				success: true,
			},
			{
				name:    "simple metric with multiple labels",
				metric:  SampleWithLabels{Labels: testJobInstanceLabels(), Value: v},
				success: true,
			},
			{
				name:    "simple metric with newlines",
				metric:  SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "test\njob\n", "instance": "localhost:9090"}, Value: v},
				success: true,
			},
			{
				name:    "simple metric with dots",
				metric:  SampleWithLabels{Labels: map[string]string{"__name__": "resource.cpu.usage", "job.name": "testjob", "instance.name": "localhost:9090"}, Value: v},
				success: true,
			},
			{
				name:    "simple metric with spaces",
				metric:  SampleWithLabels{Labels: map[string]string{"__name__": "resource cpu usage", "job name": "testjob", "instance name": "localhost:9090"}, Value: v},
				success: true,
			},
			{
				name:    "simple metric without name label",
				metric:  SampleWithLabels{Labels: map[string]string{"job": "testjob", "instance": "localhost:9090"}, Value: v},
				success: false,
			},
			{
				name:    "empty metric",
				metric:  SampleWithLabels{Labels: map[string]string{}, Value: v},
				success: false,
			},
		}
		for _, tc := range cases {
			tests = append(tests, staticComplianceTest(
				fmt.Sprintf("samples/single/%s/%s", fv.name, slugify(tc.name)),
				tc.name,
				RequestOpts{Samples: []SampleWithLabels{tc.metric}},
				requestParams{samples: 1},
				tc.success,
			))
		}
	}

	// Counter with start_timestamp set in the past.
	tests = append(tests, complianceTest(
		"samples/counter_with_created_timestamp",
		"Counter with created timestamp",
		func() RequestOpts {
			createdTime := time.Now().Add(-1 * time.Hour)
			return RequestOpts{
				Samples: []SampleWithLabels{
					{
						Labels:         map[string]string{"__name__": "http_requests_total", "job": "api"},
						Value:          100.0,
						StartTimestamp: &createdTime,
					},
				},
			}
		},
		requestParams{samples: 1},
		true,
	))

	// Multiple metric samples in a single request, including partial-write failure cases.
	for _, fv := range sampleFloatValues {
		v := fv.val
		cases := []struct {
			name         string
			metrics      []SampleWithLabels
			success      bool
			validSamples int
		}{
			{
				name: "multiple metrics",
				metrics: []SampleWithLabels{
					{Labels: testJobInstanceLabels(), Value: v},
					{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9091"}, Value: v},
					{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9092"}, Value: v},
					{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9093"}, Value: v},
				},
				success:      true,
				validSamples: 4,
			},
			{
				name: "multiple metrics, without name label",
				metrics: []SampleWithLabels{
					{Labels: map[string]string{"job": "testjob", "instance": "localhost:9090"}, Value: v},
					{Labels: map[string]string{"job": "testjob", "instance": "localhost:9091"}, Value: v},
					{Labels: map[string]string{"job": "testjob", "instance": "localhost:9092"}, Value: v},
					{Labels: map[string]string{"job": "testjob", "instance": "localhost:9093"}, Value: v},
				},
				success:      false,
				validSamples: 0,
			},
			{
				name: "multiple metrics, 1 without name label",
				metrics: []SampleWithLabels{
					{Labels: map[string]string{"job": "testjob", "instance": "localhost:9090"}, Value: v},
					{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9091"}, Value: v},
					{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9092"}, Value: v},
					{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9093"}, Value: v},
				},
				success:      false,
				validSamples: 3,
			},
		}
		for _, tc := range cases {
			tests = append(tests, staticComplianceTest(
				fmt.Sprintf("samples/multiple/%s/%s", fv.name, slugify(tc.name)),
				tc.name,
				RequestOpts{Samples: tc.metrics},
				requestParams{samples: tc.validSamples},
				tc.success,
			))
		}
	}

	// Single TimeSeries containing multiple samples ordered chronologically.
	tests = append(tests, singleRequestTest(
		"samples/chronological_single_timeseries",
		"Test a single TimeSeries containing multiple samples ordered chronologically",
		MustLevel,
		func(externalLabels map[string]string) (*http.Request, error) {
			symbols := writev2.NewSymbolTable()
			labelRefs := symbolizeSortedLabels(&symbols, map[string]string{"__name__": "chronological_series_total", "job": "batch"}, externalLabels)
			now := time.Now()
			ts := writev2.TimeSeries{
				LabelsRefs: labelRefs,
				Samples: []writev2.Sample{
					{Timestamp: now.Add(-4 * time.Millisecond).UnixMilli(), Value: 1.0},
					{Timestamp: now.Add(-2 * time.Millisecond).UnixMilli(), Value: 2.0},
					{Timestamp: now.UnixMilli(), Value: 3.0},
				},
			}
			return generateRequest(RequestOpts{
				RawTimeseries: []writev2.TimeSeries{ts},
				RawSymbols:    symbols.Symbols(),
				UnsafeRequest: true,
			}, externalLabels)
		},
		requestParams{samples: 3, success: true},
	))

	// Out-of-order sample rejection for an already-committed series.
	oooLabels := map[string]string{"__name__": "out_of_order_series_total", "job": "batch"}
	tests = append(tests, Test{
		Name:        "samples/out_of_order_rejected",
		Description: "Subsequent sample with older timestamp for the same series is rejected with 400 status",
		RFCLevel:    ShouldLevel,
		SetupRequests: []RequestBuilder{
			func(externalLabels map[string]string) (*http.Request, error) {
				return generateRequest(RequestOpts{
					Samples: []SampleWithLabels{{Labels: oooLabels, Value: 10.0}},
				}, externalLabels)
			},
		},
		BuildRequest: func(externalLabels map[string]string) (*http.Request, error) {
			return generateRequest(RequestOpts{
				Samples: []SampleWithLabels{{Labels: oooLabels, Value: 5.0, Offset: -10 * time.Minute}},
			}, externalLabels)
		},
		Validate: func(t *testing.T, res ResponseResult) {
			validateResponse(t, requestParams{samples: 1, success: false}, res)
		},
	})

	// Symbol table and label_refs structural validation.
	symbolCases := []struct {
		name        string
		description string
		buildOpts   func() RequestOpts
	}{
		{
			name:        "odd_labels_refs_length",
			description: "TimeSeries with odd number of elements in labels_refs must be rejected",
			buildOpts: func() RequestOpts {
				symbols := writev2.NewSymbolTable()
				nameIdx := symbols.Symbolize("__name__")
				valIdx := symbols.Symbolize("odd_refs_metric")
				return RequestOpts{
					RawSymbols: symbols.Symbols(),
					RawTimeseries: []writev2.TimeSeries{
						{
							LabelsRefs: []uint32{nameIdx, valIdx, nameIdx},
							Samples:    []writev2.Sample{{Timestamp: time.Now().UnixMilli(), Value: 1.0}},
						},
					},
					UnsafeRequest: true,
				}
			},
		},
		{
			name:        "out_of_bounds_label_name_ref",
			description: "TimeSeries with label name ref exceeding symbol table length must be rejected",
			buildOpts: func() RequestOpts {
				symbols := writev2.NewSymbolTable()
				valIdx := symbols.Symbolize("oob_name_metric")
				return RequestOpts{
					RawSymbols: symbols.Symbols(),
					RawTimeseries: []writev2.TimeSeries{
						{
							LabelsRefs: []uint32{999, valIdx},
							Samples:    []writev2.Sample{{Timestamp: time.Now().UnixMilli(), Value: 1.0}},
						},
					},
					UnsafeRequest: true,
				}
			},
		},
		{
			name:        "out_of_bounds_label_value_ref",
			description: "TimeSeries with label value ref exceeding symbol table length must be rejected",
			buildOpts: func() RequestOpts {
				symbols := writev2.NewSymbolTable()
				nameIdx := symbols.Symbolize("__name__")
				return RequestOpts{
					RawSymbols: symbols.Symbols(),
					RawTimeseries: []writev2.TimeSeries{
						{
							LabelsRefs: []uint32{nameIdx, 999},
							Samples:    []writev2.Sample{{Timestamp: time.Now().UnixMilli(), Value: 1.0}},
						},
					},
					UnsafeRequest: true,
				}
			},
		},
		{
			name:        "empty_symbols_table",
			description: "Request with empty symbols table and non-empty labels_refs must be rejected",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					RawSymbols: []string{},
					RawTimeseries: []writev2.TimeSeries{
						{
							LabelsRefs: []uint32{0, 0},
							Samples:    []writev2.Sample{{Timestamp: time.Now().UnixMilli(), Value: 1.0}},
						},
					},
					UnsafeRequest: true,
				}
			},
		},
		{
			name:        "duplicate_label_names",
			description: "TimeSeries with duplicate label names must be rejected",
			buildOpts: func() RequestOpts {
				symbols := writev2.NewSymbolTable()
				nameIdx := symbols.Symbolize("__name__")
				metricIdx := symbols.Symbolize("dup_label_metric")
				jobIdx := symbols.Symbolize("job")
				val1Idx := symbols.Symbolize("job_a")
				val2Idx := symbols.Symbolize("job_b")
				return RequestOpts{
					RawSymbols: symbols.Symbols(),
					RawTimeseries: []writev2.TimeSeries{
						{
							LabelsRefs: []uint32{nameIdx, metricIdx, jobIdx, val1Idx, jobIdx, val2Idx},
							Samples:    []writev2.Sample{{Timestamp: time.Now().UnixMilli(), Value: 1.0}},
						},
					},
					UnsafeRequest: true,
				}
			},
		},
		{
			name:        "empty_label_name",
			description: "TimeSeries with an empty label name must be rejected",
			buildOpts: func() RequestOpts {
				symbols := writev2.NewSymbolTable()
				nameIdx := symbols.Symbolize("__name__")
				metricIdx := symbols.Symbolize("empty_label_name_metric")
				valIdx := symbols.Symbolize("some_val")
				return RequestOpts{
					RawSymbols: symbols.Symbols(),
					RawTimeseries: []writev2.TimeSeries{
						{
							LabelsRefs: []uint32{0, valIdx, nameIdx, metricIdx},
							Samples:    []writev2.Sample{{Timestamp: time.Now().UnixMilli(), Value: 1.0}},
						},
					},
					UnsafeRequest: true,
				}
			},
		},
		{
			name:        "invalid_utf8_label_name",
			description: "TimeSeries with invalid UTF-8 in label name must be rejected",
			buildOpts: func() RequestOpts {
				symbols := writev2.NewSymbolTable()
				nameIdx := symbols.Symbolize("__name__")
				metricIdx := symbols.Symbolize("invalid_utf8_name_metric")
				badUTF8Idx := symbols.Symbolize("\xff\xfe")
				valIdx := symbols.Symbolize("val")
				return RequestOpts{
					RawSymbols: symbols.Symbols(),
					RawTimeseries: []writev2.TimeSeries{
						{
							LabelsRefs: []uint32{nameIdx, metricIdx, badUTF8Idx, valIdx},
							Samples:    []writev2.Sample{{Timestamp: time.Now().UnixMilli(), Value: 1.0}},
						},
					},
					UnsafeRequest: true,
				}
			},
		},
		{
			name:        "invalid_utf8_label_value",
			description: "TimeSeries with invalid UTF-8 in label value must be rejected",
			buildOpts: func() RequestOpts {
				symbols := writev2.NewSymbolTable()
				nameIdx := symbols.Symbolize("__name__")
				metricIdx := symbols.Symbolize("invalid_utf8_metric")
				jobIdx := symbols.Symbolize("job")
				badUTF8Idx := symbols.Symbolize("\xff\xfe")
				return RequestOpts{
					RawSymbols: symbols.Symbols(),
					RawTimeseries: []writev2.TimeSeries{
						{
							LabelsRefs: []uint32{nameIdx, metricIdx, jobIdx, badUTF8Idx},
							Samples:    []writev2.Sample{{Timestamp: time.Now().UnixMilli(), Value: 1.0}},
						},
					},
					UnsafeRequest: true,
				}
			},
		},
	}

	for _, sc := range symbolCases {
		buildOpts := sc.buildOpts
		tests = append(tests, singleRequestTest(
			"symbols/"+sc.name,
			sc.description,
			MustLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				return generateRequest(buildOpts(), externalLabels)
			},
			requestParams{success: false},
		))
	}

	return tests
}

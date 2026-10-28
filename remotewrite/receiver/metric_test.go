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
	"math"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/value"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

// values contains test float values including special values (NaN, Inf) for metric testing.
var values = map[string]float64{
	"1.0":      1.0,
	"StaleNaN": math.Float64frombits(value.StaleNaN),
	"NaN":      math.Float64frombits(value.NormalNaN),
	"Inf":      math.Inf(1),
	"-Inf":     math.Inf(-1),
}

func TestSingleMetric(t *testing.T) {
	t.Attr("description", "Test single metric samples with different float values including special values (NaN, Inf)")
	for k, v := range values {
		t.Run(k, func(t *testing.T) {
			tc := []struct {
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
			for _, tc := range tc {
				runComplianceTest(t, tc.name, tc.name,
					RequestOpts{Samples: []SampleWithLabels{tc.metric}},
					requestParams{
						samples: 1,
					},
					tc.success)
			}
		})
	}
}

func TestCounterWithCreatedTimestamp(t *testing.T) {
	should(t)
	t.Attr("description", "Test counter with created timestamp set in the past")

	now := time.Now()
	createdTime := now.Add(-1 * time.Hour)

	sample := SampleWithLabels{
		Labels:           map[string]string{"__name__": "http_requests_total", "job": "api"},
		Value:            100.0,
		StartTimestamp:   &createdTime,
	}

	runComplianceTest(t, "", "Counter with created timestamp",
		RequestOpts{Samples: []SampleWithLabels{sample}},
		requestParams{
			samples: 1,
		},
		true)
}

func TestMultipleMetrics(t *testing.T) {
	should(t)
	t.Attr("description", "Test multiple metric samples in single request with validation of partial success scenarios")
	for k, v := range values {
		t.Run(k, func(t *testing.T) {
			tc := []struct {
				name         string
				metrics      []SampleWithLabels
				success      bool
				validSamples int
			}{
				{
					name: "multiple metrics",
					metrics: []SampleWithLabels{
						SampleWithLabels{Labels: testJobInstanceLabels(), Value: v},
						SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9091"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9092"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9093"}, Value: v},
					},
					success:      true,
					validSamples: 4,
				},
				{
					name: "multiple metrics, without name label",
					metrics: []SampleWithLabels{
						SampleWithLabels{Labels: map[string]string{"job": "testjob", "instance": "localhost:9090"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"job": "testjob", "instance": "localhost:9091"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"job": "testjob", "instance": "localhost:9092"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"job": "testjob", "instance": "localhost:9093"}, Value: v},
					},
					success:      false,
					validSamples: 0,
				},
				{
					name: "multiple metrics, 1 without name label",
					metrics: []SampleWithLabels{
						SampleWithLabels{Labels: map[string]string{"job": "testjob", "instance": "localhost:9090"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9091"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9092"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9093"}, Value: v},
					},
					success:      false,
					validSamples: 3,
				},
			}
			for _, tc := range tc {
				runComplianceTest(t, tc.name, tc.name,
					RequestOpts{Samples: tc.metrics},
					requestParams{
						samples: tc.validSamples,
					},
					tc.success)
			}
		})
	}
}

func TestChronologicalSamplesInSingleTimeSeries(t *testing.T) {
	must(t)
	t.Attr("description", "Test a single TimeSeries containing multiple samples ordered chronologically")

	symbols := writev2.NewSymbolTable()
	labelRefs := symbolizeSortedLabels(&symbols, map[string]string{"__name__": "chronological_series_total", "job": "batch"})
	now := time.Now()
	ts := writev2.TimeSeries{
		LabelsRefs: labelRefs,
		Samples: []writev2.Sample{
			{Timestamp: now.Add(-4 * time.Millisecond).UnixMilli(), Value: 1.0},
			{Timestamp: now.Add(-2 * time.Millisecond).UnixMilli(), Value: 2.0},
			{Timestamp: now.UnixMilli(), Value: 3.0},
		},
	}

	runRequest(t, generateRequest(RequestOpts{
		RawTimeseries: []writev2.TimeSeries{ts},
		RawSymbols:    symbols.Symbols(),
		UnsafeRequest: true,
	}), requestParams{
		samples: 3,
		success: true,
	})
}

func TestOutOfOrderSamplesRejected(t *testing.T) {
	should(t)
	t.Attr("description", "Test that a TimeSeries with samples older than the latest committed sample is rejected with 400 status")

	labels := map[string]string{"__name__": "out_of_order_series_total", "job": "batch"}

	t.Run("initial_sample", func(t *testing.T) {
		should(t)
		t.Attr("description", "Initial sample for series succeeds")
		runRequest(t, generateRequest(RequestOpts{
			Samples: []SampleWithLabels{
				{Labels: labels, Value: 10.0},
			},
		}), requestParams{
			samples: 1,
			success: true,
		})
	})

	t.Run("older_sample_rejected", func(t *testing.T) {
		should(t)
		t.Attr("description", "Subsequent sample with older timestamp for the same series is rejected with 400 status")
		runRequest(t, generateRequest(RequestOpts{
			Samples: []SampleWithLabels{
				{Labels: labels, Value: 5.0, Offset: -10 * time.Minute},
			},
		}), requestParams{
			samples: 1,
			success: false,
		})
	})
}

func TestSymbolTableAndLabelRefsValidation(t *testing.T) {
	must(t)
	t.Attr("description", "Test that invalid symbol table references and malformed series label sets are rejected with 400 status")

	nowMilli := time.Now().UnixMilli()
	sample := []writev2.Sample{{Timestamp: nowMilli, Value: 1.0}}

	testCases := []struct {
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
							Samples:    sample,
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
							Samples:    sample,
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
							Samples:    sample,
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
							Samples:    sample,
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
							Samples:    sample,
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
							Samples:    sample,
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
							Samples:    sample,
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
							Samples:    sample,
						},
					},
					UnsafeRequest: true,
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Attr("description", tc.description)
			runRequest(t, generateRequest(tc.buildOpts()), requestParams{
				success: false,
			})
		})
	}
}

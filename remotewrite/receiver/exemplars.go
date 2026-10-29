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
	"math"
	"net/http"
	"time"

	"github.com/prometheus/prometheus/model/value"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

func exemplarsTests() []Test {
	var tests []Test

	basicCases := []struct {
		name           string
		labels         map[string]string
		exemplarLabels map[string]string
		value          float64
		success        bool
		description    string
	}{
		{
			name:           "basic_exemplar",
			labels:         map[string]string{"__name__": "http_requests_total", "job": "api"},
			exemplarLabels: traceExemplar("abc123def456"),
			value:          1.0,
			success:        true,
			description:    "Basic exemplar with trace_id",
		},
		{
			name:           "exemplar_with_span_id",
			labels:         map[string]string{"__name__": "request_duration_seconds", "method": "GET"},
			exemplarLabels: map[string]string{"trace_id": "xyz789", "span_id": "span123"},
			value:          0.125,
			success:        true,
			description:    "Exemplar with trace_id and span_id",
		},
		{
			name:           "exemplar_without_trace_id",
			labels:         map[string]string{"__name__": "cpu_usage", "instance": "host1"},
			exemplarLabels: map[string]string{"user_id": "user123"},
			value:          0.85,
			success:        true,
			description:    "Exemplar without trace_id (non-tracing use case)",
		},
		{
			name:           "exemplar_with_multiple_labels",
			labels:         map[string]string{"__name__": "request_latency", "service": "auth"},
			exemplarLabels: map[string]string{"trace_id": "trace456", "user": "admin", "endpoint": "/login"},
			value:          0.05,
			success:        true,
			description:    "Exemplar with multiple labels",
		},
		{
			name:           "exemplar_no_labels",
			labels:         basicMetric("simple_counter"),
			exemplarLabels: map[string]string{},
			value:          1.0,
			success:        true,
			description:    "Exemplar without any labels",
		},
		{
			name:           "exemplar_with_special_chars",
			labels:         basicMetric("test_metric"),
			exemplarLabels: map[string]string{"trace_id": "trace-with-dashes_and_underscores.123"},
			value:          42.0,
			success:        true,
			description:    "Exemplar with special characters in label values",
		},
		{
			name:           "exemplar_with_unicode",
			labels:         map[string]string{"__name__": "unicode_test", "region": "🌍"},
			exemplarLabels: map[string]string{"trace_id": "unicode_trace_测试"},
			value:          100.0,
			success:        true,
			description:    "Exemplar with unicode characters",
		},
	}

	for _, tc := range basicCases {
		sample := SampleWithLabels{Labels: tc.labels, Value: tc.value}
		exemplar := ExemplarWithLabels{Labels: tc.labels, ExemplarLabels: tc.exemplarLabels, Value: tc.value}
		tests = append(tests, staticComplianceTest(
			"exemplars/"+tc.name,
			tc.description,
			RequestOpts{
				Samples:   []SampleWithLabels{sample},
				Exemplars: []ExemplarWithLabels{exemplar},
			},
			requestParams{samples: 1, exemplars: 1},
			tc.success,
		))
	}

	// Exemplar attached to counter with start_timestamp.
	tests = append(tests, complianceTest(
		"exemplars/with_created_timestamp",
		"Exemplar with counter that has created timestamp",
		func() RequestOpts {
			createdTime := time.Now().Add(-2 * time.Hour)
			return RequestOpts{
				Samples: []SampleWithLabels{
					{
						Labels:         map[string]string{"__name__": "http_requests_total", "job": "api"},
						Value:          250.0,
						StartTimestamp: &createdTime,
					},
				},
				Exemplars: []ExemplarWithLabels{
					{
						Labels:         map[string]string{"__name__": "http_requests_total", "job": "api"},
						ExemplarLabels: map[string]string{"trace_id": "abc123", "span_id": "span456"},
						Value:          250.0,
					},
				},
			}
		},
		requestParams{samples: 1, exemplars: 1},
		true,
	))

	// Multiple exemplars across series.
	tests = append(tests, staticComplianceTest(
		"exemplars/multiple",
		"Test sending multiple exemplars with matching samples",
		RequestOpts{
			Samples: []SampleWithLabels{
				{Labels: map[string]string{"__name__": "request_duration", "method": "GET"}, Value: 0.1},
				{Labels: map[string]string{"__name__": "request_duration", "method": "POST"}, Value: 0.25},
				{Labels: map[string]string{"__name__": "error_count", "service": "payment"}, Value: 1.0},
			},
			Exemplars: []ExemplarWithLabels{
				{
					Labels:         map[string]string{"__name__": "request_duration", "method": "GET"},
					ExemplarLabels: map[string]string{"trace_id": "trace1", "span_id": "span1"},
					Value:          0.1,
				},
				{
					Labels:         map[string]string{"__name__": "request_duration", "method": "POST"},
					ExemplarLabels: map[string]string{"trace_id": "trace2", "span_id": "span2"},
					Value:          0.25,
				},
				{
					Labels:         map[string]string{"__name__": "error_count", "service": "payment"},
					ExemplarLabels: map[string]string{"trace_id": "trace3", "user": "test_user"},
					Value:          1.0,
				},
			},
		},
		requestParams{samples: 3, exemplars: 3},
		true,
	))

	// Exemplars with distinct timestamp offsets.
	tests = append(tests, staticComplianceTest(
		"exemplars/timestamps",
		"Test exemplars with different timestamps and matching samples",
		RequestOpts{
			Samples: []SampleWithLabels{
				{Labels: map[string]string{"__name__": "latency_hist", "instance": "host1"}, Value: 0.05, Offset: -1 * time.Hour},
				{Labels: map[string]string{"__name__": "latency_hist", "instance": "host2"}, Value: 0.15, Offset: -1 * time.Minute},
				{Labels: map[string]string{"__name__": "latency_hist", "instance": "host3"}, Value: 0.08},
			},
			Exemplars: []ExemplarWithLabels{
				{
					Labels:         map[string]string{"__name__": "latency_hist", "instance": "host1"},
					ExemplarLabels: map[string]string{"trace_id": "old_trace"},
					Value:          0.05,
					Offset:         -1 * time.Hour,
				},
				{
					Labels:         map[string]string{"__name__": "latency_hist", "instance": "host2"},
					ExemplarLabels: map[string]string{"trace_id": "recent_trace"},
					Value:          0.15,
					Offset:         -1 * time.Minute,
				},
				{
					Labels:         map[string]string{"__name__": "latency_hist", "instance": "host3"},
					ExemplarLabels: map[string]string{"trace_id": "current_trace"},
					Value:          0.08,
				},
			},
		},
		requestParams{samples: 3, exemplars: 3},
		true,
	))

	// Exemplar metric name validation.
	validationCases := []struct {
		name        string
		labels      map[string]string
		success     bool
		description string
	}{
		{
			name:        "exemplar_without_name_label",
			labels:      map[string]string{"job": "test"},
			success:     false,
			description: "Exemplar without __name__ label should be rejected",
		},
		{
			name:        "exemplar_with_empty_name",
			labels:      map[string]string{"__name__": ""},
			success:     false,
			description: "Exemplar with empty metric name should be rejected",
		},
		{
			name:        "exemplar_with_valid_name",
			labels:      basicMetric("valid_metric"),
			success:     true,
			description: "Exemplar with valid metric name should be accepted",
		},
	}

	for _, vc := range validationCases {
		lbls := vc.labels
		tests = append(tests, singleRequestTest(
			"exemplars/validation/"+vc.name,
			vc.description,
			ShouldLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				return generateRequest(RequestOpts{
					Samples: []SampleWithLabels{{Labels: lbls, Value: 1.0}},
					Exemplars: []ExemplarWithLabels{
						{
							Labels:         lbls,
							ExemplarLabels: map[string]string{"trace_id": "test_trace"},
							Value:          1.0,
						},
					},
				}, externalLabels)
			},
			requestParams{samples: 1, exemplars: 1, success: vc.success},
		))
	}

	// Exemplar labels_refs structural validation (MUST).
	refCases := []struct {
		name        string
		metricName  string
		buildRefs   func(traceIdx uint32) []uint32
		description string
	}{
		{
			name:       "odd_exemplar_labels_refs",
			metricName: "odd_exemplar_refs_metric",
			buildRefs: func(traceIdx uint32) []uint32 {
				return []uint32{traceIdx}
			},
			description: "Exemplar with odd number of elements in labels_refs should be rejected",
		},
		{
			name:       "out_of_bounds_exemplar_labels_refs",
			metricName: "oob_exemplar_refs_metric",
			buildRefs: func(traceIdx uint32) []uint32 {
				return []uint32{traceIdx, 999}
			},
			description: "Exemplar with symbol reference outside symbol table bounds should be rejected",
		},
	}

	for _, rc := range refCases {
		metricName := rc.metricName
		buildRefs := rc.buildRefs
		tests = append(tests, singleRequestTest(
			"exemplars/validation/"+rc.name,
			rc.description,
			MustLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				nowMilli := time.Now().UnixMilli()
				symbols := writev2.NewSymbolTable()
				labelRefs := symbolizeSortedLabels(&symbols, basicMetric(metricName), externalLabels)
				traceIdx := symbols.Symbolize("trace_id")
				ts := writev2.TimeSeries{
					LabelsRefs: labelRefs,
					Samples:    []writev2.Sample{{Timestamp: nowMilli, Value: 1.0}},
					Exemplars: []writev2.Exemplar{
						{
							LabelsRefs: buildRefs(traceIdx),
							Value:      1.0,
							Timestamp:  nowMilli,
						},
					},
				}
				return generateRequest(RequestOpts{
					RawSymbols:    symbols.Symbols(),
					RawTimeseries: []writev2.TimeSeries{ts},
					UnsafeRequest: true,
				}, externalLabels)
			},
			requestParams{samples: 1, success: false},
		))
	}

	// Exemplar special float values.
	specialValues := []namedFloatValue{
		{name: "zero_value", val: 0.0},
		{name: "negative_value", val: -123.456},
		{name: "large_value", val: 1e10},
		{name: "small_value", val: 1e-10},
		{name: "nan_value", val: math.Float64frombits(value.NormalNaN)},
		{name: "inf_value", val: math.Inf(1)},
		{name: "-inf_value", val: math.Inf(-1)},
	}

	for _, sv := range specialValues {
		tests = append(tests, staticComplianceTest(
			"exemplars/special_values/"+sv.name,
			sv.name,
			RequestOpts{
				Samples: []SampleWithLabels{{Labels: basicMetric("test_metric"), Value: sv.val}},
				Exemplars: []ExemplarWithLabels{
					{
						Labels:         basicMetric("test_metric"),
						ExemplarLabels: traceExemplar("special_value_trace"),
						Value:          sv.val,
					},
				},
			},
			requestParams{samples: 1, exemplars: 1},
			true,
		))
	}

	return tests
}

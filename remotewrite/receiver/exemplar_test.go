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
)

func TestExemplars(t *testing.T) {
	must(t)
	testCases := []struct {
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

	for _, tc := range testCases {
		sample := SampleWithLabels{Labels: tc.labels, Value: tc.value}
		exemplar := ExemplarWithLabels{Labels: tc.labels, ExemplarLabels: tc.exemplarLabels, Value: tc.value}

		runComplianceTest(t, tc.name, tc.description,
			RequestOpts{
				Samples:   []SampleWithLabels{sample},
				Exemplars: []ExemplarWithLabels{exemplar},
			},
			requestParams{
				samples:   1,
				exemplars: 1,
			},
			tc.success)
	}
}

func TestExemplarWithCreatedTimestamp(t *testing.T) {
	should(t)
	t.Attr("description", "Test exemplar attached to counter with created timestamp")

	now := time.Now()
	createdTime := now.Add(-2 * time.Hour)

	sample := SampleWithLabels{
		Labels:           map[string]string{"__name__": "http_requests_total", "job": "api"},
		Value:            250.0,
		CreatedTimestamp: &createdTime,
	}
	exemplar := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "http_requests_total", "job": "api"},
		ExemplarLabels: map[string]string{"trace_id": "abc123", "span_id": "span456"},
		Value:          250.0,
	}

	runComplianceTest(t, "", "Exemplar with counter that has created timestamp",
		RequestOpts{
			Samples:   []SampleWithLabels{sample},
			Exemplars: []ExemplarWithLabels{exemplar},
		},
		requestParams{
			samples:   1,
			exemplars: 1,
		},
		true)
}

func TestMultipleExemplars(t *testing.T) {
	sample1 := SampleWithLabels{Labels: map[string]string{"__name__": "request_duration", "method": "GET"}, Value: 0.1}
	exemplar1 := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "request_duration", "method": "GET"},
		ExemplarLabels: map[string]string{"trace_id": "trace1", "span_id": "span1"},
		Value:          0.1,
	}

	sample2 := SampleWithLabels{Labels: map[string]string{"__name__": "request_duration", "method": "POST"}, Value: 0.25}
	exemplar2 := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "request_duration", "method": "POST"},
		ExemplarLabels: map[string]string{"trace_id": "trace2", "span_id": "span2"},
		Value:          0.25,
	}

	sample3 := SampleWithLabels{Labels: map[string]string{"__name__": "error_count", "service": "payment"}, Value: 1.0}
	exemplar3 := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "error_count", "service": "payment"},
		ExemplarLabels: map[string]string{"trace_id": "trace3", "user": "test_user"},
		Value:          1.0,
	}

	runComplianceTest(t, "", "Test sending multiple exemplars with matching samples",
		RequestOpts{
			Samples:   []SampleWithLabels{sample1, sample2, sample3},
			Exemplars: []ExemplarWithLabels{exemplar1, exemplar2, exemplar3},
		},
		requestParams{
			samples:   3,
			exemplars: 3,
		},
		true)
}

func TestExemplarTimestamps(t *testing.T) {
	sample1 := SampleWithLabels{Labels: map[string]string{"__name__": "latency_hist"}, Value: 0.05, Offset: -1 * time.Hour}
	exemplar1 := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "latency_hist"},
		ExemplarLabels: map[string]string{"trace_id": "old_trace"},
		Value:          0.05,
		Offset:         -1 * time.Hour,
	}

	sample2 := SampleWithLabels{Labels: map[string]string{"__name__": "latency_hist"}, Value: 0.15, Offset: -1 * time.Minute}
	exemplar2 := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "latency_hist"},
		ExemplarLabels: map[string]string{"trace_id": "recent_trace"},
		Value:          0.15,
		Offset:         -1 * time.Minute,
	}

	sample3 := SampleWithLabels{Labels: map[string]string{"__name__": "latency_hist"}, Value: 0.08}
	exemplar3 := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "latency_hist"},
		ExemplarLabels: map[string]string{"trace_id": "current_trace"},
		Value:          0.08,
	}

	runComplianceTest(t, "", "Test exemplars with different timestamps and matching samples",
		RequestOpts{
			Samples:   []SampleWithLabels{sample1, sample2, sample3},
			Exemplars: []ExemplarWithLabels{exemplar1, exemplar2, exemplar3},
		},
		requestParams{
			samples:   3,
			exemplars: 3,
		},
		true)
}

func TestExemplarValidation(t *testing.T) {
	should(t)
	testCases := []struct {
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

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Attr("description", tc.description)

			sample := SampleWithLabels{Labels: tc.labels, Value: 1.0}
			exemplar := ExemplarWithLabels{
				Labels:         tc.labels,
				ExemplarLabels: map[string]string{"trace_id": "test_trace"},
				Value:          1.0,
			}

			runRequest(t, generateRequest(RequestOpts{
				Samples:   []SampleWithLabels{sample},
				Exemplars: []ExemplarWithLabels{exemplar},
			}), requestParams{
				samples:   1,
				exemplars: 1,
				success:   tc.success,
			})
		})
	}
}

func TestExemplarSpecialValues(t *testing.T) {
	t.Attr("description", "Test exemplars with special float values")

	testCases := []struct {
		name  string
		value float64
	}{
		{"zero_value", 0.0},
		{"negative_value", -123.456},
		{"large_value", 1e10},
		{"small_value", 1e-10},
		{"nan_value", float64(value.NormalNaN)},
		{"inf_value", math.Inf(1)},
		{"-inf_value", math.Inf(-1)},
	}

	for _, tc := range testCases {
		sample := SampleWithLabels{Labels: basicMetric("test_metric"), Value: tc.value}
		exemplar := ExemplarWithLabels{
			Labels:         basicMetric("test_metric"),
			ExemplarLabels: traceExemplar("special_value_trace"),
			Value:          tc.value,
		}

		runComplianceTest(t, tc.name, tc.name,
			RequestOpts{
				Samples:   []SampleWithLabels{sample},
				Exemplars: []ExemplarWithLabels{exemplar},
			},
			requestParams{
				samples:   1,
				exemplars: 1,
			},
			true)
	}
}

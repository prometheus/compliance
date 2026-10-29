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
	"net/http"
	"time"

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

func histogramsTests() []Test {
	var tests []Test

	intCases := []struct {
		name             string
		labels           map[string]string
		includePositive  bool
		includeNegative  bool
		includeZero      bool
		useCustomBuckets bool
		expectSuccess    bool
		badCount         bool
		description      string
	}{
		{
			name:            "basic_histogram",
			labels:          basicMetric("test_histogram"),
			includePositive: true,
			includeNegative: true,
			includeZero:     true,
			expectSuccess:   true,
			description:     "Basic histogram with positive, negative and zero buckets",
		},
		{
			name:             "custom_buckets_histogram",
			labels:           basicMetric("test_histogram"),
			includePositive:  true,
			useCustomBuckets: true,
			expectSuccess:    true,
			description:      "Histogram with custom bucket values",
		},
		{
			name:            "positive_only_histogram",
			labels:          map[string]string{"__name__": "test_histogram", "job": "test"},
			includePositive: true,
			includeZero:     true,
			expectSuccess:   true,
			description:     "Histogram with only positive and zero buckets",
		},
		{
			name:            "negative_only_histogram",
			labels:          map[string]string{"__name__": "test_histogram", "instance": "localhost"},
			includeNegative: true,
			expectSuccess:   true,
			description:     "Histogram with only negative buckets",
		},
		{
			name:          "minimal_histogram",
			labels:        basicMetric("minimal_hist"),
			includeZero:   true,
			expectSuccess: true,
			description:   "Minimal histogram with only zero bucket",
		},
		{
			name:            "bad_count_histogram",
			labels:          basicMetric("minimal_hist"),
			includePositive: true,
			expectSuccess:   false,
			badCount:        true,
			description:     "Histogram with bad count",
		},
	}

	for _, tc := range intCases {
		hist := HistogramWithLabels{
			Labels:    tc.labels,
			Histogram: histogram(1.0, tc.includePositive, tc.includeNegative, tc.includeZero, tc.useCustomBuckets, tc.badCount),
		}
		tests = append(tests, staticComplianceTest(
			"histograms/"+tc.name,
			tc.description,
			RequestOpts{Histograms: []HistogramWithLabels{hist}},
			requestParams{histograms: 1},
			tc.expectSuccess,
		))
	}

	// Histogram with start_timestamp set in the past.
	tests = append(tests, complianceTest(
		"histograms/with_created_timestamp",
		"Histogram with created timestamp",
		func() RequestOpts {
			createdTime := time.Now().Add(-30 * time.Minute)
			return RequestOpts{
				Histograms: []HistogramWithLabels{
					{
						Labels:         map[string]string{"__name__": "request_duration_seconds", "job": "api"},
						Histogram:      histogram(1.5, true, true, true, false, false),
						StartTimestamp: &createdTime,
					},
				},
			}
		},
		requestParams{histograms: 1},
		true,
	))

	// Multiple histograms in a single request.
	tests = append(tests, staticComplianceTest(
		"histograms/multiple",
		"Test sending multiple histograms in a single request",
		RequestOpts{
			Histograms: []HistogramWithLabels{
				{
					Labels:    map[string]string{"__name__": "hist1", "job": "test"},
					Histogram: histogram(10.5, true, false, true, false, false),
				},
				{
					Labels:    map[string]string{"__name__": "hist2", "job": "test"},
					Histogram: histogram(5.2, false, true, false, false, false),
				},
			},
		},
		requestParams{histograms: 2},
		true,
	))

	// Empty histogram with no buckets.
	tests = append(tests, staticComplianceTest(
		"histograms/empty",
		"Test histogram with no buckets",
		RequestOpts{
			Histograms: []HistogramWithLabels{
				{
					Labels:    basicMetric("empty_hist"),
					Histogram: histogram(0.0, false, false, false, false, false),
				},
			},
		},
		requestParams{histograms: 1},
		true,
	))

	// Histogram with metadata.
	tests = append(tests, staticComplianceTest(
		"histograms/with_metadata",
		"Test histogram with associated metadata",
		RequestOpts{
			Metadata: []MetadataWithLabels{
				{
					Labels: basicMetric("request_duration_seconds"),
					Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
					Help:   "HTTP request duration in seconds",
					Unit:   "seconds",
				},
			},
			Histograms: []HistogramWithLabels{
				{
					Labels:    map[string]string{"__name__": "request_duration_seconds", "job": "api"},
					Histogram: histogram(2.5, true, true, true, false, false),
				},
			},
		},
		requestParams{histograms: 1},
		true,
	))

	// Histogram with exemplar.
	tests = append(tests, staticComplianceTest(
		"histograms/with_exemplar",
		"Test histogram with exemplar",
		RequestOpts{
			Exemplars: []ExemplarWithLabels{
				{
					Labels:         map[string]string{"__name__": "http_request_duration_seconds", "method": "GET"},
					ExemplarLabels: map[string]string{"trace_id": "abc123def456", "span_id": "span789"},
					Value:          1.23,
				},
			},
			Histograms: []HistogramWithLabels{
				{
					Labels:    map[string]string{"__name__": "http_request_duration_seconds", "method": "GET"},
					Histogram: histogram(1.23, true, false, true, false, false),
				},
			},
		},
		requestParams{histograms: 1, exemplars: 1},
		true,
	))

	// Histogram with both metadata and exemplar.
	tests = append(tests, staticComplianceTest(
		"histograms/with_metadata_and_exemplar",
		"Test histogram with both metadata and exemplar",
		RequestOpts{
			Metadata: []MetadataWithLabels{
				{
					Labels: basicMetric("response_size_bytes"),
					Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
					Help:   "HTTP response size in bytes",
					Unit:   "bytes",
				},
			},
			Exemplars: []ExemplarWithLabels{
				{
					Labels:         map[string]string{"__name__": "response_size_bytes", "service": "upload"},
					ExemplarLabels: map[string]string{"trace_id": "large_upload_trace", "user_id": "user456"},
					Value:          10485760,
				},
			},
			Histograms: []HistogramWithLabels{
				{
					Labels:    map[string]string{"__name__": "response_size_bytes", "service": "upload"},
					Histogram: histogram(10485760, true, true, true, false, false),
				},
			},
		},
		requestParams{histograms: 1, exemplars: 1},
		true,
	))

	// Multiple histograms with metadata.
	tests = append(tests, staticComplianceTest(
		"histograms/multiple_with_metadata",
		"Test multiple histograms each with their own metadata",
		RequestOpts{
			Metadata: []MetadataWithLabels{
				{
					Labels: basicMetric("cpu_usage_histogram"),
					Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
					Help:   "CPU usage distribution",
					Unit:   "ratio",
				},
				{
					Labels: basicMetric("memory_usage_histogram"),
					Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
					Help:   "Memory usage distribution",
					Unit:   "bytes",
				},
			},
			Histograms: []HistogramWithLabels{
				{
					Labels:    map[string]string{"__name__": "cpu_usage_histogram", "instance": "server1"},
					Histogram: histogram(0.75, true, false, true, false, false),
				},
				{
					Labels:    map[string]string{"__name__": "memory_usage_histogram", "instance": "server1"},
					Histogram: histogram(8589934592, true, true, false, false, false),
				},
			},
		},
		requestParams{histograms: 2},
		true,
	))

	// Float histograms.
	floatCases := []struct {
		name             string
		labels           map[string]string
		includePositive  bool
		includeNegative  bool
		includeZero      bool
		useCustomBuckets bool
		expectSuccess    bool
		badCount         bool
		description      string
	}{
		{
			name:            "basic_float_histogram",
			labels:          basicMetric("test_float_histogram"),
			includePositive: true,
			includeNegative: true,
			includeZero:     true,
			expectSuccess:   true,
			description:     "Basic float histogram with positive, negative, and zero buckets",
		},
		{
			name:             "custom_buckets_float_histogram",
			labels:           basicMetric("test_float_custom_histogram"),
			includePositive:  true,
			useCustomBuckets: true,
			expectSuccess:    true,
			description:      "Float histogram with custom bucket values (NHCB)",
		},
		{
			name:          "empty_float_histogram",
			labels:        basicMetric("empty_float_hist"),
			expectSuccess: true,
			description:   "Empty float histogram with zero count and sum",
		},
		{
			name:            "bad_count_float_histogram",
			labels:          basicMetric("bad_float_hist"),
			includePositive: true,
			expectSuccess:   false,
			badCount:        true,
			description:     "Float histogram with negative total count",
		},
	}

	for _, tc := range floatCases {
		hist := HistogramWithLabels{
			Labels:    tc.labels,
			Histogram: floatHistogram(1.0, tc.includePositive, tc.includeNegative, tc.includeZero, tc.useCustomBuckets, tc.badCount),
		}
		tests = append(tests, staticComplianceTest(
			"histograms/float/"+tc.name,
			tc.description,
			RequestOpts{Histograms: []HistogramWithLabels{hist}},
			requestParams{histograms: 1},
			tc.expectSuccess,
		))
	}

	// Reset hints and schemas.
	resetSchemaCases := []struct {
		name        string
		metricName  string
		schema      int32
		resetHint   writev2.Histogram_ResetHint
		description string
	}{
		{
			name:        "reset_hint_unspecified_schema_0",
			metricName:  "hist_reset_unspecified",
			schema:      0,
			resetHint:   writev2.Histogram_RESET_HINT_UNSPECIFIED,
			description: "Histogram with unspecified reset hint and schema 0",
		},
		{
			name:        "reset_hint_yes_schema_3",
			metricName:  "hist_reset_yes",
			schema:      3,
			resetHint:   writev2.Histogram_RESET_HINT_YES,
			description: "Histogram with explicit counter reset hint YES and schema 3",
		},
		{
			name:        "reset_hint_no_schema_minus_2",
			metricName:  "hist_reset_no",
			schema:      -2,
			resetHint:   writev2.Histogram_RESET_HINT_NO,
			description: "Histogram with counter reset hint NO and schema -2",
		},
		{
			name:        "reset_hint_gauge_schema_5",
			metricName:  "hist_reset_gauge",
			schema:      5,
			resetHint:   writev2.Histogram_RESET_HINT_GAUGE,
			description: "Gauge histogram with reset hint GAUGE and schema 5",
		},
	}

	for _, tc := range resetSchemaCases {
		rawHist := histogram(3.14, true, true, true, false, false)
		rawHist.Schema = tc.schema
		rawHist.ResetHint = tc.resetHint

		tests = append(tests, staticComplianceTest(
			"histograms/reset_hints_and_schemas/"+tc.name,
			tc.description,
			RequestOpts{
				Histograms: []HistogramWithLabels{
					{
						Labels:    basicMetric(tc.metricName),
						Histogram: rawHist,
					},
				},
			},
			requestParams{histograms: 1},
			true,
		))
	}

	// Structurally invalid histograms rejected with 400.
	invalidCases := []struct {
		name        string
		metricName  string
		hist        writev2.Histogram
		description string
	}{
		{
			name:        "negative_bucket_count",
			metricName:  "invalid_neg_bucket_hist",
			description: "Integer histogram whose delta-encoded buckets decode to a negative count",
			hist: writev2.Histogram{
				Count:          &writev2.Histogram_CountInt{CountInt: 1},
				Sum:            1.0,
				PositiveSpans:  []writev2.BucketSpan{{Offset: 0, Length: 2}},
				PositiveDeltas: []int64{2, -5},
			},
		},
		{
			name:        "span_length_mismatch",
			metricName:  "invalid_span_mismatch_hist",
			description: "Histogram where bucket span lengths do not match bucket delta count",
			hist: writev2.Histogram{
				Count:          &writev2.Histogram_CountInt{CountInt: 5},
				Sum:            1.0,
				PositiveSpans:  []writev2.BucketSpan{{Offset: 0, Length: 3}},
				PositiveDeltas: []int64{5},
			},
		},
		{
			name:        "unsorted_custom_bucket_values",
			metricName:  "invalid_custom_bounds_hist",
			description: "Custom buckets histogram with non-increasing custom_values",
			hist: writev2.Histogram{
				Schema:         -53,
				Count:          &writev2.Histogram_CountInt{CountInt: 4},
				Sum:            2.0,
				CustomValues:   []float64{5.0, 1.0},
				PositiveSpans:  []writev2.BucketSpan{{Offset: 0, Length: 2}},
				PositiveDeltas: []int64{2, 0},
			},
		},
	}

	for _, ic := range invalidCases {
		hist := HistogramWithLabels{
			Labels:    basicMetric(ic.metricName),
			Histogram: ic.hist,
		}
		tests = append(tests, singleRequestTest(
			"histograms/invalid_rejected/"+ic.name,
			ic.description,
			MustLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				return generateRequest(RequestOpts{Histograms: []HistogramWithLabels{hist}}, externalLabels)
			},
			requestParams{success: false},
		))
	}

	return tests
}

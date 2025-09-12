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
	"testing"
	"time"

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

func TestHistograms(t *testing.T) {
	must(t)
	testCases := []struct {
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
			name:             "basic_histogram",
			labels:           basicMetric("test_histogram"),
			includePositive:  true,
			includeNegative:  true,
			includeZero:      true,
			useCustomBuckets: false,
			expectSuccess:    true,
			description:      "Basic histogram with positive, negative and zero buckets",
		},
		{
			name:             "custom_buckets_histogram",
			labels:           basicMetric("test_histogram"),
			includePositive:  true,
			includeNegative:  false,
			includeZero:      false,
			useCustomBuckets: true,
			expectSuccess:    true,
			description:      "Histogram with custom bucket values",
		},
		{
			name:             "positive_only_histogram",
			labels:           map[string]string{"__name__": "test_histogram", "job": "test"},
			includePositive:  true,
			includeNegative:  false,
			includeZero:      true,
			useCustomBuckets: false,
			expectSuccess:    true,
			description:      "Histogram with only positive and zero buckets",
		},
		{
			name:             "negative_only_histogram",
			labels:           map[string]string{"__name__": "test_histogram", "instance": "localhost"},
			includePositive:  false,
			includeNegative:  true,
			includeZero:      false,
			useCustomBuckets: false,
			expectSuccess:    true,
			description:      "Histogram with only negative buckets",
		},
		{
			name:             "minimal_histogram",
			labels:           basicMetric("minimal_hist"),
			includePositive:  false,
			includeNegative:  false,
			includeZero:      true,
			useCustomBuckets: false,
			expectSuccess:    true,
			description:      "Minimal histogram with only zero bucket",
		},
		{
			name:             "bad_count_histogram",
			labels:           basicMetric("minimal_hist"),
			includePositive:  true,
			includeNegative:  false,
			includeZero:      false,
			useCustomBuckets: false,
			expectSuccess:    false,
			badCount:         true,
			description:      "Histogram with bad count",
		},
	}

	for _, tc := range testCases {
		hist := HistogramWithLabels{
			Labels:    tc.labels,
			Histogram: histogram(1.0, tc.includePositive, tc.includeNegative, tc.includeZero, tc.useCustomBuckets, tc.badCount),
		}

		runComplianceTest(t, tc.name, tc.description,
			RequestOpts{Histograms: []HistogramWithLabels{hist}},
			requestParams{
				histograms: 1,
			},
			tc.expectSuccess)
	}
}
func TestHistogramWithCreatedTimestamp(t *testing.T) {
	should(t)
	t.Attr("description", "Test histogram with created timestamp")

	now := time.Now()
	createdTime := now.Add(-30 * time.Minute)

	hist := HistogramWithLabels{
		Labels:           map[string]string{"__name__": "request_duration_seconds", "job": "api"},
		Histogram:        histogram(1.5, true, true, true, false, false),
		CreatedTimestamp: &createdTime,
	}

	runComplianceTest(t, "", "Histogram with created timestamp",
		RequestOpts{Histograms: []HistogramWithLabels{hist}},
		requestParams{
			histograms: 1,
		},
		true)
}

func TestMultipleHistograms(t *testing.T) {
	hist1 := HistogramWithLabels{
		Labels:    map[string]string{"__name__": "hist1", "job": "test"},
		Histogram: histogram(10.5, true, false, true, false, false),
	}
	hist2 := HistogramWithLabels{
		Labels:    map[string]string{"__name__": "hist2", "job": "test"},
		Histogram: histogram(5.2, false, true, false, false, false),
	}

	runComplianceTest(t, "", "Test sending multiple histograms in a single request",
		RequestOpts{Histograms: []HistogramWithLabels{hist1, hist2}},
		requestParams{
			histograms: 2,
		},
		true)
}

func TestEmptyHistogram(t *testing.T) {
	hist := HistogramWithLabels{
		Labels:    basicMetric("empty_hist"),
		Histogram: histogram(0.0, false, false, false, false, false),
	}

	runComplianceTest(t, "", "Test histogram with no buckets",
		RequestOpts{Histograms: []HistogramWithLabels{hist}},
		requestParams{
			histograms: 1,
		},
		true)
}

func TestHistogramWithMetadata(t *testing.T) {
	hist := HistogramWithLabels{
		Labels:    map[string]string{"__name__": "request_duration_seconds", "job": "api"},
		Histogram: histogram(2.5, true, true, true, false, false),
	}

	metadata := MetadataWithLabels{
		Labels: basicMetric("request_duration_seconds"),
		Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
		Help:   "HTTP request duration in seconds",
		Unit:   "seconds",
	}

	runComplianceTest(t, "", "Test histogram with associated metadata",
		RequestOpts{
			Metadata:   []MetadataWithLabels{metadata},
			Histograms: []HistogramWithLabels{hist},
		},
		requestParams{
			histograms: 1,
		},
		true)
}

func TestHistogramWithExemplar(t *testing.T) {
	hist := HistogramWithLabels{
		Labels:    map[string]string{"__name__": "http_request_duration_seconds", "method": "GET"},
		Histogram: histogram(1.23, true, false, true, false, false),
	}

	exemplar := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "http_request_duration_seconds", "method": "GET"},
		ExemplarLabels: map[string]string{"trace_id": "abc123def456", "span_id": "span789"},
		Value:          1.23,
	}

	runComplianceTest(t, "", "Test histogram with exemplar",
		RequestOpts{
			Exemplars:  []ExemplarWithLabels{exemplar},
			Histograms: []HistogramWithLabels{hist},
		},
		requestParams{
			histograms: 1,
			exemplars:  1,
		},
		true)
}

func TestHistogramWithMetadataAndExemplar(t *testing.T) {
	hist := HistogramWithLabels{
		Labels:    map[string]string{"__name__": "response_size_bytes", "service": "upload"},
		Histogram: histogram(10485760, true, true, true, false, false),
	}

	metadata := MetadataWithLabels{
		Labels: basicMetric("response_size_bytes"),
		Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
		Help:   "HTTP response size in bytes",
		Unit:   "bytes",
	}

	exemplar := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "response_size_bytes", "service": "upload"},
		ExemplarLabels: map[string]string{"trace_id": "large_upload_trace", "user_id": "user456"},
		Value:          10485760,
	}

	runComplianceTest(t, "", "Test histogram with both metadata and exemplar",
		RequestOpts{
			Metadata:   []MetadataWithLabels{metadata},
			Exemplars:  []ExemplarWithLabels{exemplar},
			Histograms: []HistogramWithLabels{hist},
		},
		requestParams{
			histograms: 1,
			exemplars:  1,
		},
		true)
}

func TestMultipleHistogramsWithMetadata(t *testing.T) {
	hist1 := HistogramWithLabels{
		Labels:    map[string]string{"__name__": "cpu_usage_histogram", "instance": "server1"},
		Histogram: histogram(0.75, true, false, true, false, false),
	}

	hist2 := HistogramWithLabels{
		Labels:    map[string]string{"__name__": "memory_usage_histogram", "instance": "server1"},
		Histogram: histogram(8589934592, true, true, false, false, false),
	}

	metadata1 := MetadataWithLabels{
		Labels: basicMetric("cpu_usage_histogram"),
		Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
		Help:   "CPU usage distribution",
		Unit:   "ratio",
	}

	metadata2 := MetadataWithLabels{
		Labels: basicMetric("memory_usage_histogram"),
		Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
		Help:   "Memory usage distribution",
		Unit:   "bytes",
	}

	runComplianceTest(t, "", "Test multiple histograms each with their own metadata",
		RequestOpts{
			Metadata:   []MetadataWithLabels{metadata1, metadata2},
			Histograms: []HistogramWithLabels{hist1, hist2},
		},
		requestParams{
			histograms: 2,
		},
		true)
}

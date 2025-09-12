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

// TestMetadata tests requests with metadata attached.
func TestMetadata(t *testing.T) {
	must(t)
	testCases := []struct {
		name        string
		labels      map[string]string
		metricType  writev2.Metadata_MetricType
		help        string
		unit        string
		success     bool
		description string
	}{
		{
			name:        "counter_metadata",
			labels:      basicMetric("http_requests_total"),
			metricType:  writev2.Metadata_METRIC_TYPE_COUNTER,
			help:        "Total number of HTTP requests",
			unit:        "requests",
			success:     true,
			description: "Counter metadata with help and unit",
		},
		{
			name:        "gauge_metadata",
			labels:      basicMetric("cpu_usage_percent"),
			metricType:  writev2.Metadata_METRIC_TYPE_GAUGE,
			help:        "Current CPU usage percentage",
			unit:        "percent",
			success:     true,
			description: "Gauge metadata with help and unit",
		},
		{
			name:        "histogram_metadata",
			labels:      basicMetric("request_duration_seconds"),
			metricType:  writev2.Metadata_METRIC_TYPE_HISTOGRAM,
			help:        "Request duration in seconds",
			unit:        "seconds",
			success:     true,
			description: "Histogram metadata with help and unit",
		},
		{
			name:        "metadata_no_help_no_unit",
			labels:      basicMetric("simple_counter"),
			metricType:  writev2.Metadata_METRIC_TYPE_COUNTER,
			help:        "",
			unit:        "",
			success:     true,
			description: "Metadata without help or unit strings",
		},
		{
			name:        "metadata_with_special_chars",
			labels:      map[string]string{"__name__": "test_metric", "job": "test"},
			metricType:  writev2.Metadata_METRIC_TYPE_GAUGE,
			help:        "Test metric with\nnewlines and \"quotes\"",
			unit:        "bytes/sec",
			success:     true,
			description: "Metadata with special characters in help",
		},
		{
			name:        "unspecified_metadata_type",
			labels:      basicMetric("unknown_metric"),
			metricType:  writev2.Metadata_METRIC_TYPE_UNSPECIFIED,
			help:        "Unknown metric type",
			unit:        "",
			success:     true,
			description: "Metadata with unspecified metric type",
		},
		{
			name:        "info_metadata_type",
			labels:      basicMetric("build_info"),
			metricType:  writev2.Metadata_METRIC_TYPE_INFO,
			help:        "Build information",
			unit:        "",
			success:     true,
			description: "Info type metadata",
		},
		{
			name:        "stateset_metadata_type",
			labels:      basicMetric("node_state"),
			metricType:  writev2.Metadata_METRIC_TYPE_STATESET,
			help:        "Node state information",
			unit:        "",
			success:     true,
			description: "StateSet type metadata",
		},
		{
			name:        "summary_metadata_type",
			labels:      basicMetric("request_latency_summary"),
			metricType:  writev2.Metadata_METRIC_TYPE_SUMMARY,
			help:        "Request latency summary",
			unit:        "seconds",
			success:     true,
			description: "Summary type metadata",
		},
		{
			name:        "gauge_histogram_metadata_type",
			labels:      basicMetric("temperature_histogram"),
			metricType:  writev2.Metadata_METRIC_TYPE_GAUGEHISTOGRAM,
			help:        "Temperature distribution gauge histogram",
			unit:        "celsius",
			success:     true,
			description: "Gauge histogram type metadata",
		},
	}

	for _, tc := range testCases {
		metadata := MetadataWithLabels{Labels: tc.labels, Type: tc.metricType, Help: tc.help, Unit: tc.unit}

		opts := RequestOpts{
			Metadata: []MetadataWithLabels{metadata},
		}

		expectedParams := requestParams{}

		// Add matching metric data based on type
		if tc.metricType == writev2.Metadata_METRIC_TYPE_HISTOGRAM {
			// Add histogram for histogram metadata
			hist := HistogramWithLabels{Labels: tc.labels, Histogram: histogram(1.0, true, true, true, false, false)}
			opts.Histograms = []HistogramWithLabels{hist}
			expectedParams.histograms = 1
		} else {
			// Add sample for counter/gauge metadata
			sample := SampleWithLabels{Labels: tc.labels, Value: 1.0}
			opts.Samples = []SampleWithLabels{sample}
			expectedParams.samples = 1
		}

		runComplianceTest(t, tc.name, tc.description, opts, expectedParams, tc.success)
	}
}

// TestMetadataWithSamples tests requests with metadata and samples attached.
func TestCounterMetadataWithCreatedTimestamp(t *testing.T) {
	should(t)
	t.Attr("description", "Test counter with metadata and created timestamp")

	now := time.Now()
	createdTime := now.Add(-1 * time.Hour)

	sample := SampleWithLabels{
		Labels:           map[string]string{"__name__": "http_requests_total", "job": "api"},
		Value:            150.0,
		CreatedTimestamp: &createdTime,
	}
	metadata := MetadataWithLabels{
		Labels: basicMetric("http_requests_total"),
		Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
		Help:   "Total HTTP requests",
		Unit:   "requests",
	}

	runComplianceTest(t, "", "Counter with metadata and created timestamp",
		RequestOpts{
			Samples:  []SampleWithLabels{sample},
			Metadata: []MetadataWithLabels{metadata},
		},
		requestParams{
			samples: 1,
		},
		true)
}

func TestMetadataWithSamples(t *testing.T) {
	sample := SampleWithLabels{Labels: map[string]string{"__name__": "http_requests_total", "job": "test"}, Value: 42.0}
	metadata := MetadataWithLabels{
		Labels: basicMetric("http_requests_total"),
		Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
		Help:   "Total HTTP requests",
		Unit:   "requests",
	}

	runComplianceTest(t, "", "Test metadata sent together with samples",
		RequestOpts{
			Samples:  []SampleWithLabels{sample},
			Metadata: []MetadataWithLabels{metadata},
		},
		requestParams{
			samples: 1,
		},
		true)
}

// TestMultipleMetadata tests requests with multiple metadata entries attached.
func TestMultipleMetadata(t *testing.T) {
	// Create matching samples for each metadata
	sample1 := SampleWithLabels{Labels: basicMetric("cpu_usage"), Value: 85.5}
	metadata1 := MetadataWithLabels{
		Labels: basicMetric("cpu_usage"),
		Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
		Help:   "CPU usage percentage",
		Unit:   "percent",
	}

	sample2 := SampleWithLabels{Labels: basicMetric("memory_usage"), Value: 4096000000}
	metadata2 := MetadataWithLabels{
		Labels: basicMetric("memory_usage"),
		Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
		Help:   "Memory usage in bytes",
		Unit:   "bytes",
	}

	sample3 := SampleWithLabels{Labels: map[string]string{"__name__": "disk_usage", "device": "sda1"}, Value: 75.2}
	metadata3 := MetadataWithLabels{
		Labels: map[string]string{"__name__": "disk_usage", "device": "sda1"},
		Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
		Help:   "Disk usage percentage",
		Unit:   "percent",
	}

	runComplianceTest(t, "", "Test sending multiple metadata entries with matching samples",
		RequestOpts{
			Samples:  []SampleWithLabels{sample1, sample2, sample3},
			Metadata: []MetadataWithLabels{metadata1, metadata2, metadata3},
		},
		requestParams{
			samples: 3,
		},
		true)
}

// TestMetadataWithComplexLabels tests requests with metadata and complex label sets attached.
func TestMetadataWithComplexLabels(t *testing.T) {
	histogramLabels := map[string]string{
		"__name__": "http_request_duration_seconds",
		"job":      "api-server",
		"instance": "localhost:8080",
		"method":   "GET",
		"status":   "200",
		"endpoint": "/api/v1/users",
	}

	// Create matching histogram for the metadata.
	histogram := HistogramWithLabels{
		Labels:    histogramLabels,
		Histogram: histogram(0.250, true, true, true, false, false),
	}
	metadata := MetadataWithLabels{
		Labels: histogramLabels,
		Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
		Help:   "HTTP request duration in seconds",
		Unit:   "seconds",
	}

	runComplianceTest(t, "", "Test metadata with complex label sets and matching histogram",
		RequestOpts{
			Histograms: []HistogramWithLabels{histogram},
			Metadata:   []MetadataWithLabels{metadata},
		},
		requestParams{
			histograms: 1,
		},
		true)
}

// TestMetadataValidation tests requests with bad metadata.
func TestMetadataValidation(t *testing.T) {
	should(t)
	testCases := []struct {
		name        string
		labels      map[string]string
		metricType  writev2.Metadata_MetricType
		success     bool
		description string
	}{
		{
			name:        "metadata_without_name_label",
			labels:      map[string]string{"job": "test"},
			metricType:  writev2.Metadata_METRIC_TYPE_COUNTER,
			success:     false,
			description: "Metadata without __name__ label should be rejected",
		},
		{
			name:        "metadata_with_empty_name",
			labels:      map[string]string{"__name__": ""},
			metricType:  writev2.Metadata_METRIC_TYPE_GAUGE,
			success:     false,
			description: "Metadata with empty metric name should be rejected",
		},
		{
			name:        "metadata_with_valid_name",
			labels:      basicMetric("valid_metric_name"),
			metricType:  writev2.Metadata_METRIC_TYPE_COUNTER,
			success:     true,
			description: "Metadata with valid metric name should be accepted",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Attr("description", tc.description)
			metadata := MetadataWithLabels{Labels: tc.labels, Type: tc.metricType, Help: "Test help", Unit: "unit"}

			opts := RequestOpts{
				Metadata: []MetadataWithLabels{metadata},
			}

			sample := SampleWithLabels{Labels: tc.labels, Value: 1.0}
			opts.Samples = []SampleWithLabels{sample}

			expectedParams := requestParams{
				success: tc.success,
				samples: 1,
			}

			runRequest(t, generateRequest(opts), expectedParams)
		})
	}
}

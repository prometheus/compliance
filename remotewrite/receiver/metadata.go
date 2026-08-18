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
	"time"

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

// metadataTests returns compliance tests covering metadata attached to samples/histograms.
func metadataTests() (ret []Test) {
	cases := []struct {
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
			success:     true,
			description: "Metadata with unspecified metric type",
		},
		{
			name:        "info_metadata_type",
			labels:      basicMetric("build_info"),
			metricType:  writev2.Metadata_METRIC_TYPE_INFO,
			help:        "Build information",
			success:     true,
			description: "Info type metadata",
		},
		{
			name:        "stateset_metadata_type",
			labels:      basicMetric("node_state"),
			metricType:  writev2.Metadata_METRIC_TYPE_STATESET,
			help:        "Node state information",
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

	for _, tc := range cases {
		opts := RequestOpts{
			Metadata: []MetadataWithLabels{{Labels: tc.labels, Type: tc.metricType, Help: tc.help, Unit: tc.unit}},
		}
		expect := ExpectedResponse{}
		if tc.metricType == writev2.Metadata_METRIC_TYPE_HISTOGRAM {
			opts.Histograms = []HistogramWithLabels{{Labels: tc.labels, Histogram: Histogram(1.0, true, true, true, false, false)}}
			expect.Histograms = 1
		} else {
			opts.Samples = []SampleWithLabels{{Labels: tc.labels, Value: 1.0}}
			expect.Samples = 1
		}

		ret = append(ret, Test{
			Name:          "Metadata/" + tc.name,
			Description:   tc.description,
			Opts:          opts,
			Expect:        expect,
			ExpectSuccess: tc.success,
		})
	}

	now := time.Now()
	createdTime := now.Add(-1 * time.Hour)
	ret = append(ret, Test{
		Name:        "CounterMetadataWithCreatedTimestamp",
		Description: "Test counter with metadata and created timestamp",
		Opts: RequestOpts{
			Samples: []SampleWithLabels{{
				Labels:         map[string]string{"__name__": "http_requests_total", "job": "api"},
				Value:          150.0,
				StartTimestamp: &createdTime,
			}},
			Metadata: []MetadataWithLabels{{
				Labels: basicMetric("http_requests_total"),
				Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
				Help:   "Total HTTP requests",
				Unit:   "requests",
			}},
		},
		Expect:        ExpectedResponse{Samples: 1},
		ExpectSuccess: true,
		Raw:           true,
		RFCLevel:      ShouldLevel,
	})

	ret = append(ret, Test{
		Name:        "MetadataWithSamples",
		Description: "Test metadata sent together with samples",
		Opts: RequestOpts{
			Samples: []SampleWithLabels{{Labels: map[string]string{"__name__": "http_requests_total", "job": "test"}, Value: 42.0}},
			Metadata: []MetadataWithLabels{{
				Labels: basicMetric("http_requests_total"),
				Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
				Help:   "Total HTTP requests",
				Unit:   "requests",
			}},
		},
		Expect:        ExpectedResponse{Samples: 1},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "MultipleMetadata",
		Description: "Test sending multiple metadata entries with matching samples",
		Opts: RequestOpts{
			Samples: []SampleWithLabels{
				{Labels: basicMetric("cpu_usage"), Value: 85.5},
				{Labels: basicMetric("memory_usage"), Value: 4096000000},
				{Labels: map[string]string{"__name__": "disk_usage", "device": "sda1"}, Value: 75.2},
			},
			Metadata: []MetadataWithLabels{
				{Labels: basicMetric("cpu_usage"), Type: writev2.Metadata_METRIC_TYPE_GAUGE, Help: "CPU usage percentage", Unit: "percent"},
				{Labels: basicMetric("memory_usage"), Type: writev2.Metadata_METRIC_TYPE_GAUGE, Help: "Memory usage in bytes", Unit: "bytes"},
				{Labels: map[string]string{"__name__": "disk_usage", "device": "sda1"}, Type: writev2.Metadata_METRIC_TYPE_GAUGE, Help: "Disk usage percentage", Unit: "percent"},
			},
		},
		Expect:        ExpectedResponse{Samples: 3},
		ExpectSuccess: true,
	})

	histogramLabels := map[string]string{
		"__name__": "http_request_duration_seconds",
		"job":      "api-server",
		"instance": "localhost:8080",
		"method":   "GET",
		"status":   "200",
		"endpoint": "/api/v1/users",
	}
	ret = append(ret, Test{
		Name:        "MetadataWithComplexLabels",
		Description: "Test metadata with complex label sets and matching histogram",
		Opts: RequestOpts{
			Histograms: []HistogramWithLabels{{Labels: histogramLabels, Histogram: Histogram(0.250, true, true, true, false, false)}},
			Metadata: []MetadataWithLabels{{
				Labels: histogramLabels,
				Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
				Help:   "HTTP request duration in seconds",
				Unit:   "seconds",
			}},
		},
		Expect:        ExpectedResponse{Histograms: 1},
		ExpectSuccess: true,
	})

	validationCases := []struct {
		name          string
		labels        map[string]string
		metricType    writev2.Metadata_MetricType
		success       bool
		unsafeRequest bool
		description   string
	}{
		{
			name:        "metadata_without_name_label",
			labels:      map[string]string{"job": "test"},
			metricType:  writev2.Metadata_METRIC_TYPE_COUNTER,
			success:     false,
			description: "Metadata without __name__ label should be rejected",
		},
		{
			name:          "metadata_with_empty_name",
			labels:        map[string]string{"__name__": ""},
			metricType:    writev2.Metadata_METRIC_TYPE_GAUGE,
			success:       false,
			unsafeRequest: true, // Enable unsafe to avoid sanitization of test case requests e.g. removing metadata with empty __name__.
			description:   "Metadata with empty metric name should be rejected",
		},
		{
			name:        "metadata_with_valid_name",
			labels:      basicMetric("valid_metric_name"),
			metricType:  writev2.Metadata_METRIC_TYPE_COUNTER,
			success:     true,
			description: "Metadata with valid metric name should be accepted",
		},
	}
	for _, tc := range validationCases {
		ret = append(ret, Test{
			Name:        "MetadataValidation/" + tc.name,
			Description: tc.description,
			Opts: RequestOpts{
				Metadata:      []MetadataWithLabels{{Labels: tc.labels, Type: tc.metricType, Help: "Test help", Unit: "unit"}},
				Samples:       []SampleWithLabels{{Labels: tc.labels, Value: 1.0}},
				UnsafeRequest: tc.unsafeRequest,
			},
			Expect:        ExpectedResponse{Samples: 1},
			ExpectSuccess: tc.success,
			Raw:           true,
			RFCLevel:      ShouldLevel,
		})
	}

	return ret
}

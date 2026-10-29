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
	"net/http"
	"time"

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

func combinedTests() []Test {
	var tests []Test

	// Samples with both metadata and exemplars.
	tests = append(tests, staticComplianceTest(
		"combined/samples_with_metadata_and_exemplars",
		"Test samples with both metadata and exemplars",
		RequestOpts{
			Samples: []SampleWithLabels{
				{Labels: map[string]string{"__name__": "http_requests_total", "job": "api", "method": "POST"}, Value: 1.0},
			},
			Metadata: []MetadataWithLabels{
				{
					Labels: map[string]string{"__name__": "http_requests_total", "job": "api", "method": "POST"},
					Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
					Help:   "Total number of HTTP requests",
					Unit:   "requests",
				},
			},
			Exemplars: []ExemplarWithLabels{
				{
					Labels:         map[string]string{"__name__": "http_requests_total", "job": "api", "method": "POST"},
					ExemplarLabels: map[string]string{"trace_id": "request_trace_123", "user_id": "user789"},
					Value:          1.0,
				},
			},
		},
		requestParams{samples: 1, exemplars: 1},
		true,
	))

	// Histograms with both metadata and exemplars.
	tests = append(tests, staticComplianceTest(
		"combined/histograms_with_metadata_and_exemplars",
		"Test histograms with both metadata and exemplars",
		RequestOpts{
			Histograms: []HistogramWithLabels{
				{
					Labels:    map[string]string{"__name__": "request_duration_seconds", "service": "auth"},
					Histogram: histogram(0.456, true, true, true, false, false),
				},
			},
			Metadata: []MetadataWithLabels{
				{
					Labels: basicMetric("request_duration_seconds"),
					Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
					Help:   "HTTP request duration distribution",
					Unit:   "seconds",
				},
			},
			Exemplars: []ExemplarWithLabels{
				{
					Labels:         map[string]string{"__name__": "request_duration_seconds", "service": "auth"},
					ExemplarLabels: map[string]string{"trace_id": "slow_request_trace", "endpoint": "/login"},
					Value:          0.456,
				},
			},
		},
		requestParams{histograms: 1, exemplars: 1},
		true,
	))

	// Complex request with samples, metadata, and exemplars.
	tests = append(tests, staticComplianceTest(
		"combined/complex_samples",
		"Test complex request with samples, metadata, and exemplars",
		RequestOpts{
			Samples: []SampleWithLabels{
				{Labels: map[string]string{"__name__": "cpu_usage", "instance": "server1"}, Value: 0.85},
				{Labels: map[string]string{"__name__": "memory_usage", "instance": "server1"}, Value: 8589934592},
			},
			Metadata: []MetadataWithLabels{
				{
					Labels: basicMetric("cpu_usage"),
					Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
					Help:   "CPU usage ratio",
					Unit:   "ratio",
				},
				{
					Labels: basicMetric("memory_usage"),
					Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
					Help:   "Memory usage in bytes",
					Unit:   "bytes",
				},
			},
			Exemplars: []ExemplarWithLabels{
				{
					Labels:         map[string]string{"__name__": "cpu_usage", "instance": "server1"},
					ExemplarLabels: map[string]string{"trace_id": "high_cpu_trace", "process": "worker"},
					Value:          0.85,
				},
			},
		},
		requestParams{samples: 2, exemplars: 1},
		true,
	))

	// Complex request with histograms, metadata, and exemplars.
	tests = append(tests, staticComplianceTest(
		"combined/complex_histograms",
		"Test complex request with histograms, metadata, and exemplars",
		RequestOpts{
			Histograms: []HistogramWithLabels{
				{
					Labels:    map[string]string{"__name__": "response_time_hist", "endpoint": "/api/v1/users"},
					Histogram: histogram(0.125, true, false, true, false, false),
				},
				{
					Labels:    map[string]string{"__name__": "request_size_hist", "method": "POST"},
					Histogram: histogram(1024, false, true, true, false, false),
				},
			},
			Metadata: []MetadataWithLabels{
				{
					Labels: basicMetric("response_time_hist"),
					Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
					Help:   "Response time distribution",
					Unit:   "seconds",
				},
				{
					Labels: basicMetric("request_size_hist"),
					Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
					Help:   "Request size distribution",
					Unit:   "bytes",
				},
			},
			Exemplars: []ExemplarWithLabels{
				{
					Labels:         map[string]string{"__name__": "response_time_hist", "endpoint": "/api/v1/users"},
					ExemplarLabels: map[string]string{"trace_id": "fast_response", "user": "premium"},
					Value:          0.125,
				},
			},
		},
		requestParams{histograms: 2, exemplars: 1},
		true,
	))

	// Multiple sample metric families with metadata and exemplars.
	tests = append(tests, staticComplianceTest(
		"combined/multiple_metric_families_samples",
		"Test multiple sample metric families with comprehensive metadata",
		RequestOpts{
			Samples: []SampleWithLabels{
				{Labels: map[string]string{"__name__": "http_requests_total", "status": "200"}, Value: 1000},
				{Labels: map[string]string{"__name__": "temperature_celsius", "location": "datacenter1"}, Value: 23.5},
			},
			Metadata: []MetadataWithLabels{
				{
					Labels: basicMetric("http_requests_total"),
					Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
					Help:   "Total HTTP requests",
					Unit:   "requests",
				},
				{
					Labels: basicMetric("temperature_celsius"),
					Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
					Help:   "Temperature in Celsius",
					Unit:   "celsius",
				},
			},
			Exemplars: []ExemplarWithLabels{
				{
					Labels:         map[string]string{"__name__": "http_requests_total", "status": "200"},
					ExemplarLabels: map[string]string{"trace_id": "successful_request"},
					Value:          1000,
				},
			},
		},
		requestParams{samples: 2, exemplars: 1},
		true,
	))

	// Histogram metric family with metadata and exemplar.
	tests = append(tests, staticComplianceTest(
		"combined/multiple_metric_families_histograms",
		"Test histogram metric families with comprehensive metadata",
		RequestOpts{
			Histograms: []HistogramWithLabels{
				{
					Labels:    map[string]string{"__name__": "request_duration_seconds", "handler": "/metrics"},
					Histogram: histogram(0.003, true, false, true, false, false),
				},
			},
			Metadata: []MetadataWithLabels{
				{
					Labels: basicMetric("request_duration_seconds"),
					Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
					Help:   "Request duration in seconds",
					Unit:   "seconds",
				},
			},
			Exemplars: []ExemplarWithLabels{
				{
					Labels:         map[string]string{"__name__": "request_duration_seconds", "handler": "/metrics"},
					ExemplarLabels: map[string]string{"trace_id": "metrics_scrape", "scraper": "prometheus"},
					Value:          0.003,
				},
			},
		},
		requestParams{histograms: 1, exemplars: 1},
		true,
	))

	// Edge case combinations (SHOULD).
	edgeCases := []struct {
		name           string
		description    string
		opts           RequestOpts
		expectedParams requestParams
	}{
		{
			name:        "metadata_without_data",
			description: "Metadata sent without corresponding samples, histograms, or exemplars should be rejected",
			opts: RequestOpts{
				Metadata: []MetadataWithLabels{
					{
						Labels: basicMetric("orphan_metric"),
						Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
						Help:   "Orphaned metric metadata",
						Unit:   "count",
					},
				},
				UnsafeRequest: true,
			},
			// Per RW 2.0 spec, TimeSeries messages must specify at least one sample, histogram, or exemplar;
			// standalone metadata-only TimeSeries are invalid.
			expectedParams: requestParams{success: false},
		},
		{
			name:        "exemplars_without_data",
			description: "Exemplars sent without corresponding samples or histograms",
			opts: RequestOpts{
				Exemplars: []ExemplarWithLabels{
					{
						Labels:         basicMetric("orphan_metric"),
						ExemplarLabels: map[string]string{"trace_id": "orphan_trace"},
						Value:          42.0,
					},
				},
				UnsafeRequest: true,
			},
			expectedParams: requestParams{exemplars: 1, success: true},
		},
		{
			name:        "mixed_timestamp_data",
			description: "Mixed data with different timestamps",
			opts: RequestOpts{
				Samples: []SampleWithLabels{
					{Labels: basicMetric("old_metric"), Value: 1.0, Offset: -1 * time.Hour},
					{Labels: basicMetric("new_metric"), Value: 2.0},
				},
				Exemplars: []ExemplarWithLabels{
					{
						Labels:         basicMetric("old_metric"),
						ExemplarLabels: map[string]string{"trace_id": "old_trace"},
						Value:          1.0,
						Offset:         -30 * time.Minute,
					},
				},
			},
			expectedParams: requestParams{samples: 2, exemplars: 1, success: true},
		},
	}

	for _, ec := range edgeCases {
		opts := ec.opts
		tests = append(tests, singleRequestTest(
			"combined/edge_cases/"+ec.name,
			ec.description,
			ShouldLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				return generateRequest(opts, externalLabels)
			},
			ec.expectedParams,
		))
	}

	// Large batch of 50 counter samples with metadata and exemplars.
	manySamples := make([]SampleWithLabels, 0, 50)
	manySamplesMeta := make([]MetadataWithLabels, 0, 50)
	manySamplesEx := make([]ExemplarWithLabels, 0, 50)
	for i := range 50 {
		metricName := fmt.Sprintf("counter_metric_%d", i)
		labels := map[string]string{"__name__": metricName, "series": fmt.Sprintf("series_%d", i)}
		manySamples = append(manySamples, SampleWithLabels{Labels: labels, Value: float64(i * 100)})
		manySamplesMeta = append(manySamplesMeta, MetadataWithLabels{
			Labels: map[string]string{"__name__": metricName},
			Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
			Help:   fmt.Sprintf("Counter metric number %d", i),
			Unit:   "count",
		})
		manySamplesEx = append(manySamplesEx, ExemplarWithLabels{
			Labels:         labels,
			ExemplarLabels: map[string]string{"trace_id": fmt.Sprintf("trace_%d", i)},
			Value:          float64(i * 100),
		})
	}
	tests = append(tests, staticComplianceTest(
		"combined/many_samples",
		"Test large scale request with many samples, metadata, and exemplars",
		RequestOpts{
			Samples:   manySamples,
			Metadata:  manySamplesMeta,
			Exemplars: manySamplesEx,
		},
		requestParams{samples: 50, exemplars: 50},
		true,
	))

	// Large batch of 50 histogram samples with metadata and exemplars.
	manyHists := make([]HistogramWithLabels, 0, 50)
	manyHistsMeta := make([]MetadataWithLabels, 0, 50)
	manyHistsEx := make([]ExemplarWithLabels, 0, 50)
	for i := range 50 {
		metricName := fmt.Sprintf("histogram_metric_%d", i)
		labels := map[string]string{"__name__": metricName, "bucket": fmt.Sprintf("bucket_%d", i)}
		manyHists = append(manyHists, HistogramWithLabels{
			Labels:    labels,
			Histogram: histogram(float64(i)/10.0, true, i%2 == 0, true, false, false),
		})
		manyHistsMeta = append(manyHistsMeta, MetadataWithLabels{
			Labels: map[string]string{"__name__": metricName},
			Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
			Help:   fmt.Sprintf("Histogram metric number %d", i),
			Unit:   "seconds",
		})
		manyHistsEx = append(manyHistsEx, ExemplarWithLabels{
			Labels:         labels,
			ExemplarLabels: map[string]string{"trace_id": fmt.Sprintf("trace_%d", i)},
			Value:          float64(i) / 10.0,
		})
	}
	tests = append(tests, staticComplianceTest(
		"combined/many_histograms",
		"Test large scale request with many histograms, metadata, and exemplars",
		RequestOpts{
			Histograms: manyHists,
			Metadata:   manyHistsMeta,
			Exemplars:  manyHistsEx,
		},
		requestParams{histograms: 50, exemplars: 50},
		true,
	))

	// Special characters across labels, metadata, and exemplars.
	specialLabels := map[string]string{
		"__name__":    "test_metric",
		"job":         "test-job_with.special@chars",
		"instance":    "localhost:9090",
		"environment": "test🚀",
	}
	tests = append(tests, staticComplianceTest(
		"combined/special_characters",
		"Test combined request with special characters in labels and metadata",
		RequestOpts{
			Samples: []SampleWithLabels{{Labels: specialLabels, Value: 42.0}},
			Metadata: []MetadataWithLabels{
				{
					Labels: basicMetric("test_metric"),
					Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
					Help:   "Test metric with special characters: äöü, newlines\nand \"quotes\"",
					Unit:   "special/units",
				},
			},
			Exemplars: []ExemplarWithLabels{
				{
					Labels: specialLabels,
					ExemplarLabels: map[string]string{
						"trace_id": "special_trace_äöü-123",
						"span_id":  "span_with.dots-and_underscores",
					},
					Value: 42.0,
				},
			},
		},
		requestParams{samples: 1, exemplars: 1},
		true,
	))

	// Custom buckets histogram with exemplar.
	tests = append(tests, staticComplianceTest(
		"combined/custom_buckets_histogram_with_exemplar",
		"Test custom buckets histogram with exemplar",
		RequestOpts{
			Exemplars: []ExemplarWithLabels{
				{
					Labels:         map[string]string{"__name__": "custom_latency_hist", "version": "v2"},
					ExemplarLabels: map[string]string{"trace_id": "custom_bucket_trace", "bucket": "0.1"},
					Value:          0.05,
				},
			},
			Histograms: []HistogramWithLabels{
				{
					Labels:    map[string]string{"__name__": "custom_latency_hist", "version": "v2"},
					Histogram: histogram(0.05, false, false, false, true, false),
				},
			},
		},
		requestParams{histograms: 1, exemplars: 1},
		true,
	))

	return tests
}

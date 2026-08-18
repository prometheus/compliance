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

// combinedTests returns compliance tests combining samples/histograms with metadata and exemplars.
func combinedTests() (ret []Test) {
	ret = append(ret, Test{
		Name:        "SamplesWithMetadataAndExemplars",
		Description: "Test samples with both metadata and exemplars",
		Opts: RequestOpts{
			Samples: []SampleWithLabels{{Labels: map[string]string{"__name__": "http_requests_total", "job": "api", "method": "POST"}, Value: 1.0}},
			Metadata: []MetadataWithLabels{{
				Labels: map[string]string{"__name__": "http_requests_total", "job": "api", "method": "POST"},
				Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
				Help:   "Total number of HTTP requests",
				Unit:   "requests",
			}},
			Exemplars: []ExemplarWithLabels{{
				Labels:         map[string]string{"__name__": "http_requests_total", "job": "api", "method": "POST"},
				ExemplarLabels: map[string]string{"trace_id": "request_trace_123", "user_id": "user789"},
				Value:          1.0,
			}},
		},
		Expect:        ExpectedResponse{Samples: 1, Exemplars: 1},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "HistogramsWithMetadataAndExemplars",
		Description: "Test histograms with both metadata and exemplars",
		Opts: RequestOpts{
			Histograms: []HistogramWithLabels{{
				Labels:    map[string]string{"__name__": "request_duration_seconds", "service": "auth"},
				Histogram: Histogram(0.456, true, true, true, false, false),
			}},
			Metadata: []MetadataWithLabels{{
				Labels: basicMetric("request_duration_seconds"),
				Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
				Help:   "HTTP request duration distribution",
				Unit:   "seconds",
			}},
			Exemplars: []ExemplarWithLabels{{
				Labels:         map[string]string{"__name__": "request_duration_seconds", "service": "auth"},
				ExemplarLabels: map[string]string{"trace_id": "slow_request_trace", "endpoint": "/login"},
				Value:          0.456,
			}},
		},
		Expect:        ExpectedResponse{Histograms: 1, Exemplars: 1},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "ComplexSamplesRequest",
		Description: "Test complex request with samples, metadata, and exemplars",
		Opts: RequestOpts{
			Samples: []SampleWithLabels{
				{Labels: map[string]string{"__name__": "cpu_usage", "instance": "server1"}, Value: 0.85},
				{Labels: map[string]string{"__name__": "memory_usage", "instance": "server1"}, Value: 8589934592},
			},
			Metadata: []MetadataWithLabels{
				{Labels: basicMetric("cpu_usage"), Type: writev2.Metadata_METRIC_TYPE_GAUGE, Help: "CPU usage ratio", Unit: "ratio"},
				{Labels: basicMetric("memory_usage"), Type: writev2.Metadata_METRIC_TYPE_GAUGE, Help: "Memory usage in bytes", Unit: "bytes"},
			},
			Exemplars: []ExemplarWithLabels{{
				Labels:         map[string]string{"__name__": "cpu_usage", "instance": "server1"},
				ExemplarLabels: map[string]string{"trace_id": "high_cpu_trace", "process": "worker"},
				Value:          0.85,
			}},
		},
		Expect:        ExpectedResponse{Samples: 2, Exemplars: 1},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "ComplexHistogramsRequest",
		Description: "Test complex request with histograms, metadata, and exemplars",
		Opts: RequestOpts{
			Histograms: []HistogramWithLabels{
				{Labels: map[string]string{"__name__": "response_time_hist", "endpoint": "/api/v1/users"}, Histogram: Histogram(0.125, true, false, true, false, false)},
				{Labels: map[string]string{"__name__": "request_size_hist", "method": "POST"}, Histogram: Histogram(1024, false, true, true, false, false)},
			},
			Metadata: []MetadataWithLabels{
				{Labels: basicMetric("response_time_hist"), Type: writev2.Metadata_METRIC_TYPE_HISTOGRAM, Help: "Response time distribution", Unit: "seconds"},
				{Labels: basicMetric("request_size_hist"), Type: writev2.Metadata_METRIC_TYPE_HISTOGRAM, Help: "Request size distribution", Unit: "bytes"},
			},
			Exemplars: []ExemplarWithLabels{{
				Labels:         map[string]string{"__name__": "response_time_hist", "endpoint": "/api/v1/users"},
				ExemplarLabels: map[string]string{"trace_id": "fast_response", "user": "premium"},
				Value:          0.125,
			}},
		},
		Expect:        ExpectedResponse{Histograms: 2, Exemplars: 1},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "MultipleMetricFamiliesSamples",
		Description: "Test multiple sample metric families with comprehensive metadata",
		Opts: RequestOpts{
			Samples: []SampleWithLabels{
				{Labels: map[string]string{"__name__": "http_requests_total", "status": "200"}, Value: 1000},
				{Labels: map[string]string{"__name__": "temperature_celsius", "location": "datacenter1"}, Value: 23.5},
			},
			Metadata: []MetadataWithLabels{
				{Labels: basicMetric("http_requests_total"), Type: writev2.Metadata_METRIC_TYPE_COUNTER, Help: "Total HTTP requests", Unit: "requests"},
				{Labels: basicMetric("temperature_celsius"), Type: writev2.Metadata_METRIC_TYPE_GAUGE, Help: "Temperature in Celsius", Unit: "celsius"},
			},
			Exemplars: []ExemplarWithLabels{{
				Labels:         map[string]string{"__name__": "http_requests_total", "status": "200"},
				ExemplarLabels: map[string]string{"trace_id": "successful_request"},
				Value:          1000,
			}},
		},
		Expect:        ExpectedResponse{Samples: 2, Exemplars: 1},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "MultipleMetricFamiliesHistograms",
		Description: "Test histogram metric families with comprehensive metadata",
		Opts: RequestOpts{
			Histograms: []HistogramWithLabels{{
				Labels:    map[string]string{"__name__": "request_duration_seconds", "handler": "/metrics"},
				Histogram: Histogram(0.003, true, false, true, false, false),
			}},
			Metadata: []MetadataWithLabels{{
				Labels: basicMetric("request_duration_seconds"),
				Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
				Help:   "Request duration in seconds",
				Unit:   "seconds",
			}},
			Exemplars: []ExemplarWithLabels{{
				Labels:         map[string]string{"__name__": "request_duration_seconds", "handler": "/metrics"},
				ExemplarLabels: map[string]string{"trace_id": "metrics_scrape", "scraper": "prometheus"},
				Value:          0.003,
			}},
		},
		Expect:        ExpectedResponse{Histograms: 1, Exemplars: 1},
		ExpectSuccess: true,
	})

	ret = append(ret, edgeCaseCombinationTests()...)

	var manySamples []SampleWithLabels
	var manySampleMetadata []MetadataWithLabels
	var manySampleExemplars []ExemplarWithLabels
	for i := 0; i < 50; i++ {
		metricName := fmt.Sprintf("counter_metric_%d", i)
		labels := map[string]string{"__name__": metricName, "series": fmt.Sprintf("series_%d", i)}

		manySamples = append(manySamples, SampleWithLabels{Labels: labels, Value: float64(i * 100)})
		manySampleMetadata = append(manySampleMetadata, MetadataWithLabels{
			Labels: map[string]string{"__name__": metricName},
			Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
			Help:   fmt.Sprintf("Counter metric number %d", i),
			Unit:   "count",
		})
		manySampleExemplars = append(manySampleExemplars, ExemplarWithLabels{
			Labels:         labels,
			ExemplarLabels: map[string]string{"trace_id": fmt.Sprintf("trace_%d", i)},
			Value:          float64(i * 100),
		})
	}
	ret = append(ret, Test{
		Name:        "ManySamples",
		Description: "Test large scale request with many samples, metadata, and exemplars",
		Opts: RequestOpts{
			Samples:   manySamples,
			Metadata:  manySampleMetadata,
			Exemplars: manySampleExemplars,
		},
		Expect:        ExpectedResponse{Samples: 50, Exemplars: 50},
		ExpectSuccess: true,
	})

	var manyHistograms []HistogramWithLabels
	var manyHistogramMetadata []MetadataWithLabels
	var manyHistogramExemplars []ExemplarWithLabels
	for i := 0; i < 50; i++ {
		metricName := fmt.Sprintf("histogram_metric_%d", i)
		labels := map[string]string{"__name__": metricName, "bucket": fmt.Sprintf("bucket_%d", i)}

		manyHistograms = append(manyHistograms, HistogramWithLabels{
			Labels:    labels,
			Histogram: Histogram(float64(i)/10.0, true, i%2 == 0, true, false, false),
		})
		manyHistogramMetadata = append(manyHistogramMetadata, MetadataWithLabels{
			Labels: map[string]string{"__name__": metricName},
			Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
			Help:   fmt.Sprintf("Histogram metric number %d", i),
			Unit:   "seconds",
		})
		manyHistogramExemplars = append(manyHistogramExemplars, ExemplarWithLabels{
			Labels:         labels,
			ExemplarLabels: map[string]string{"trace_id": fmt.Sprintf("trace_%d", i)},
			Value:          float64(i) / 10.0,
		})
	}
	ret = append(ret, Test{
		Name:        "ManyHistograms",
		Description: "Test large scale request with many histograms, metadata, and exemplars",
		Opts: RequestOpts{
			Histograms: manyHistograms,
			Metadata:   manyHistogramMetadata,
			Exemplars:  manyHistogramExemplars,
		},
		Expect:        ExpectedResponse{Histograms: 50, Exemplars: 50},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "SpecialCharactersCombined",
		Description: "Test combined request with special characters in labels and metadata",
		Opts: RequestOpts{
			Samples: []SampleWithLabels{{Labels: map[string]string{
				"__name__":    "test_metric",
				"job":         "test-job_with.special@chars",
				"instance":    "localhost:9090",
				"environment": "test🚀",
			}, Value: 42.0}},
			Metadata: []MetadataWithLabels{{
				Labels: basicMetric("test_metric"),
				Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
				Help:   "Test metric with special characters: äöü, newlines\nand \"quotes\"",
				Unit:   "special/units",
			}},
			Exemplars: []ExemplarWithLabels{{
				Labels: map[string]string{
					"__name__":    "test_metric",
					"job":         "test-job_with.special@chars",
					"instance":    "localhost:9090",
					"environment": "test🚀",
				},
				ExemplarLabels: map[string]string{
					"trace_id": "special_trace_äöü-123",
					"span_id":  "span_with.dots-and_underscores",
				},
				Value: 42.0,
			}},
		},
		Expect:        ExpectedResponse{Samples: 1, Exemplars: 1},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "CustomBucketsHistogramWithExemplar",
		Description: "Test custom buckets histogram with exemplar",
		Opts: RequestOpts{
			Exemplars: []ExemplarWithLabels{{
				Labels:         map[string]string{"__name__": "custom_latency_hist", "version": "v2"},
				ExemplarLabels: map[string]string{"trace_id": "custom_bucket_trace", "bucket": "0.1"},
				Value:          0.05,
			}},
			Histograms: []HistogramWithLabels{{
				Labels:    map[string]string{"__name__": "custom_latency_hist", "version": "v2"},
				Histogram: Histogram(0.05, false, false, false, true, false), // Custom buckets.
			}},
		},
		Expect:        ExpectedResponse{Histograms: 1, Exemplars: 1},
		ExpectSuccess: true,
	})

	return ret
}

// edgeCaseCombinationTests covers edge cases that need a custom-built request
// rather than one derived from RequestOpts (e.g. orphan metadata/exemplars via
// UnsafeRequest, which RequestOpts's own safety checks would otherwise reject).
func edgeCaseCombinationTests() []Test {
	return []Test{
		{
			Name:        "EdgeCaseCombinations/metadata_without_data",
			Description: "Metadata sent without corresponding samples or histograms",
			BuildRequest: func() *http.Request {
				return generateRequest(RequestOpts{
					Metadata: []MetadataWithLabels{{
						Labels: basicMetric("orphan_metric"),
						Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
						Help:   "Orphaned metric metadata",
						Unit:   "count",
					}},
					UnsafeRequest: true,
				})
			},
			Expect:        ExpectedResponse{},
			ExpectSuccess: true,
			Raw:           true,
			RFCLevel:      ShouldLevel,
		},
		{
			Name:        "EdgeCaseCombinations/exemplars_without_data",
			Description: "Exemplars sent without corresponding samples or histograms",
			BuildRequest: func() *http.Request {
				return generateRequest(RequestOpts{
					Exemplars: []ExemplarWithLabels{{
						Labels:         basicMetric("orphan_metric"),
						ExemplarLabels: map[string]string{"trace_id": "orphan_trace"},
						Value:          42.0,
					}},
					UnsafeRequest: true,
				})
			},
			Expect:        ExpectedResponse{Exemplars: 1},
			ExpectSuccess: true,
			Raw:           true,
			RFCLevel:      ShouldLevel,
		},
		{
			Name:        "EdgeCaseCombinations/mixed_timestamp_data",
			Description: "Mixed data with different timestamps",
			BuildRequest: func() *http.Request {
				return generateRequest(RequestOpts{
					Samples: []SampleWithLabels{
						{Labels: basicMetric("old_metric"), Value: 1.0, Offset: -1 * time.Hour},
						{Labels: basicMetric("new_metric"), Value: 2.0},
					},
					Exemplars: []ExemplarWithLabels{{
						Labels:         basicMetric("old_metric"),
						ExemplarLabels: map[string]string{"trace_id": "old_trace"},
						Value:          1.0,
						Offset:         -30 * time.Minute,
					}},
				})
			},
			Expect:        ExpectedResponse{Samples: 2, Exemplars: 1},
			ExpectSuccess: true,
			Raw:           true,
			RFCLevel:      ShouldLevel,
		},
	}
}

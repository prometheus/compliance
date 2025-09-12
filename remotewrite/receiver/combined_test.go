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
	"fmt"
	"net/http"
	"testing"
	"time"

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

func TestSamplesWithMetadataAndExemplars(t *testing.T) {
	sample := SampleWithLabels{Labels: map[string]string{"__name__": "http_requests_total", "job": "api", "method": "POST"}, Value: 1.0}

	metadata := MetadataWithLabels{
		Labels: map[string]string{"__name__": "http_requests_total", "job": "api", "method": "POST"},
		Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
		Help:   "Total number of HTTP requests",
		Unit:   "requests",
	}

	exemplar := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "http_requests_total", "job": "api", "method": "POST"},
		ExemplarLabels: map[string]string{"trace_id": "request_trace_123", "user_id": "user789"},
		Value:          1.0,
	}

	runComplianceTest(t, "", "Test samples with both metadata and exemplars",
		RequestOpts{
			Samples:   []SampleWithLabels{sample},
			Metadata:  []MetadataWithLabels{metadata},
			Exemplars: []ExemplarWithLabels{exemplar},
		},
		requestParams{
			samples:   1,
			exemplars: 1,
		},
		true)
}

func TestHistogramsWithMetadataAndExemplars(t *testing.T) {
	hist := HistogramWithLabels{
		Labels:    map[string]string{"__name__": "request_duration_seconds", "service": "auth"},
		Histogram: histogram(0.456, true, true, true, false, false),
	}

	metadata := MetadataWithLabels{
		Labels: basicMetric("request_duration_seconds"),
		Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
		Help:   "HTTP request duration distribution",
		Unit:   "seconds",
	}

	exemplar := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "request_duration_seconds", "service": "auth"},
		ExemplarLabels: map[string]string{"trace_id": "slow_request_trace", "endpoint": "/login"},
		Value:          0.456,
	}

	runComplianceTest(t, "", "Test histograms with both metadata and exemplars",
		RequestOpts{
			Histograms: []HistogramWithLabels{hist},
			Metadata:   []MetadataWithLabels{metadata},
			Exemplars:  []ExemplarWithLabels{exemplar},
		},
		requestParams{
			histograms: 1,
			exemplars:  1,
			retryable:  false,
		},
		true)
}

// TestComplexSamplesRequest tests complex requests with samples, metadata, and exemplars.
func TestComplexSamplesRequest(t *testing.T) {
	sample1 := SampleWithLabels{Labels: map[string]string{"__name__": "cpu_usage", "instance": "server1"}, Value: 0.85}
	sample2 := SampleWithLabels{Labels: map[string]string{"__name__": "memory_usage", "instance": "server1"}, Value: 8589934592}

	metadata1 := MetadataWithLabels{
		Labels: basicMetric("cpu_usage"),
		Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
		Help:   "CPU usage ratio",
		Unit:   "ratio",
	}

	metadata2 := MetadataWithLabels{
		Labels: basicMetric("memory_usage"),
		Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
		Help:   "Memory usage in bytes",
		Unit:   "bytes",
	}

	exemplar1 := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "cpu_usage", "instance": "server1"},
		ExemplarLabels: map[string]string{"trace_id": "high_cpu_trace", "process": "worker"},
		Value:          0.85,
	}

	runComplianceTest(t, "", "Test complex request with samples, metadata, and exemplars",
		RequestOpts{
			Samples:   []SampleWithLabels{sample1, sample2},
			Metadata:  []MetadataWithLabels{metadata1, metadata2},
			Exemplars: []ExemplarWithLabels{exemplar1},
		},
		requestParams{
			samples:   2,
			exemplars: 1,
		},
		true)
}

func TestComplexHistogramsRequest(t *testing.T) {
	hist1 := HistogramWithLabels{
		Labels:    map[string]string{"__name__": "response_time_hist", "endpoint": "/api/v1/users"},
		Histogram: histogram(0.125, true, false, true, false, false),
	}

	hist2 := HistogramWithLabels{
		Labels:    map[string]string{"__name__": "request_size_hist", "method": "POST"},
		Histogram: histogram(1024, false, true, true, false, false),
	}

	metadata3 := MetadataWithLabels{
		Labels: basicMetric("response_time_hist"),
		Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
		Help:   "Response time distribution",
		Unit:   "seconds",
	}

	metadata4 := MetadataWithLabels{
		Labels: basicMetric("request_size_hist"),
		Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
		Help:   "Request size distribution",
		Unit:   "bytes",
	}

	exemplar2 := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "response_time_hist", "endpoint": "/api/v1/users"},
		ExemplarLabels: map[string]string{"trace_id": "fast_response", "user": "premium"},
		Value:          0.125,
	}

	runComplianceTest(t, "", "Test complex request with histograms, metadata, and exemplars",
		RequestOpts{
			Histograms: []HistogramWithLabels{hist1, hist2},
			Metadata:   []MetadataWithLabels{metadata3, metadata4},
			Exemplars:  []ExemplarWithLabels{exemplar2},
		},
		requestParams{
			histograms: 2,
			exemplars:  1,
			retryable:  false,
		},
		true)
}

// TestMultipleMetricFamiliesSamples tests multiple sample metric families with comprehensive metadata.
func TestMultipleMetricFamiliesSamples(t *testing.T) {
	counterSample := SampleWithLabels{Labels: map[string]string{"__name__": "http_requests_total", "status": "200"}, Value: 1000}
	counterMetadata := MetadataWithLabels{
		Labels: basicMetric("http_requests_total"),
		Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
		Help:   "Total HTTP requests",
		Unit:   "requests",
	}
	counterExemplar := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "http_requests_total", "status": "200"},
		ExemplarLabels: map[string]string{"trace_id": "successful_request"},
		Value:          1000,
	}

	gaugeSample := SampleWithLabels{Labels: map[string]string{"__name__": "temperature_celsius", "location": "datacenter1"}, Value: 23.5}
	gaugeMetadata := MetadataWithLabels{
		Labels: basicMetric("temperature_celsius"),
		Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
		Help:   "Temperature in Celsius",
		Unit:   "celsius",
	}

	runComplianceTest(t, "", "Test multiple sample metric families with comprehensive metadata",
		RequestOpts{
			Samples:   []SampleWithLabels{counterSample, gaugeSample},
			Metadata:  []MetadataWithLabels{counterMetadata, gaugeMetadata},
			Exemplars: []ExemplarWithLabels{counterExemplar},
		},
		requestParams{
			samples:   2,
			exemplars: 1,
		},
		true)
}

func TestMultipleMetricFamiliesHistograms(t *testing.T) {
	histogramData := HistogramWithLabels{
		Labels:    map[string]string{"__name__": "request_duration_seconds", "handler": "/metrics"},
		Histogram: histogram(0.003, true, false, true, false, false),
	}
	histogramMetadata := MetadataWithLabels{
		Labels: basicMetric("request_duration_seconds"),
		Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
		Help:   "Request duration in seconds",
		Unit:   "seconds",
	}
	histogramExemplar := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "request_duration_seconds", "handler": "/metrics"},
		ExemplarLabels: map[string]string{"trace_id": "metrics_scrape", "scraper": "prometheus"},
		Value:          0.003,
	}

	runComplianceTest(t, "", "Test histogram metric families with comprehensive metadata",
		RequestOpts{
			Histograms: []HistogramWithLabels{histogramData},
			Metadata:   []MetadataWithLabels{histogramMetadata},
			Exemplars:  []ExemplarWithLabels{histogramExemplar},
		},
		requestParams{
			histograms: 1,
			exemplars:  1,
			retryable:  false,
		},
		true)
}

func TestEdgeCaseCombinations(t *testing.T) {
	should(t)
	testCases := []struct {
		name           string
		description    string
		buildRequest   func() *http.Request
		expectedParams requestParams
	}{
		{
			name:        "metadata_without_data",
			description: "Metadata sent without corresponding samples or histograms",
			buildRequest: func() *http.Request {
				metadata := MetadataWithLabels{
					Labels: basicMetric("orphan_metric"),
					Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
					Help:   "Orphaned metric metadata",
					Unit:   "count",
				}
				return generateRequest(RequestOpts{
					Metadata:      []MetadataWithLabels{metadata},
					UnsafeRequest: true,
				})
			},
			expectedParams: requestParams{
				success: true,
			},
		},
		{
			name:        "exemplars_without_data",
			description: "Exemplars sent without corresponding samples or histograms",
			buildRequest: func() *http.Request {
				exemplar := ExemplarWithLabels{
					Labels:         basicMetric("orphan_metric"),
					ExemplarLabels: map[string]string{"trace_id": "orphan_trace"},
					Value:          42.0,
				}
				return generateRequest(RequestOpts{
					Exemplars:     []ExemplarWithLabels{exemplar},
					UnsafeRequest: true,
				})
			},
			expectedParams: requestParams{
				exemplars: 1,
				success:   true,
			},
		},
		{
			name:        "mixed_timestamp_data",
			description: "Mixed data with different timestamps",
			buildRequest: func() *http.Request {
				oldSample := SampleWithLabels{Labels: basicMetric("old_metric"), Value: 1.0, Offset: -1 * time.Hour}
				newSample := SampleWithLabels{Labels: basicMetric("new_metric"), Value: 2.0}

				oldExemplar := ExemplarWithLabels{
					Labels:         basicMetric("old_metric"),
					ExemplarLabels: map[string]string{"trace_id": "old_trace"},
					Value:          1.0,
					Offset:         -30 * time.Minute,
				}

				return generateRequest(RequestOpts{
					Samples:   []SampleWithLabels{oldSample, newSample},
					Exemplars: []ExemplarWithLabels{oldExemplar},
				})
			},
			expectedParams: requestParams{
				samples:   2,
				exemplars: 1,
				success:   true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Attr("description", tc.description)
			runRequest(t, tc.buildRequest(), tc.expectedParams)
		})
	}
}

func TestManySamples(t *testing.T) {
	var samples []SampleWithLabels
	var metadata []MetadataWithLabels
	var exemplars []ExemplarWithLabels

	// Generate 50 counter samples with metadata and exemplars.
	for i := 0; i < 50; i++ {
		metricName := fmt.Sprintf("counter_metric_%d", i)
		labels := map[string]string{"__name__": metricName, "series": fmt.Sprintf("series_%d", i)}

		samples = append(samples, SampleWithLabels{Labels: labels, Value: float64(i * 100)})

		metadata = append(metadata, MetadataWithLabels{
			Labels: map[string]string{"__name__": metricName},
			Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
			Help:   fmt.Sprintf("Counter metric number %d", i),
			Unit:   "count",
		})

		exemplars = append(exemplars, ExemplarWithLabels{
			Labels:         labels,
			ExemplarLabels: map[string]string{"trace_id": fmt.Sprintf("trace_%d", i)},
			Value:          float64(i * 100),
		})
	}

	runComplianceTest(t, "", "Test large scale request with many samples, metadata, and exemplars",
		RequestOpts{
			Samples:   samples,
			Metadata:  metadata,
			Exemplars: exemplars,
		},
		requestParams{
			samples:   50,
			exemplars: 50,
		},
		true)
}

func TestManyHistograms(t *testing.T) {
	var histograms []HistogramWithLabels
	var metadata []MetadataWithLabels
	var exemplars []ExemplarWithLabels

	// Generate 50 histogram samples with metadata and exemplars.
	for i := 0; i < 50; i++ {
		metricName := fmt.Sprintf("histogram_metric_%d", i)
		labels := map[string]string{"__name__": metricName, "bucket": fmt.Sprintf("bucket_%d", i)}

		histograms = append(histograms, HistogramWithLabels{
			Labels:    labels,
			Histogram: histogram(float64(i)/10.0, true, i%2 == 0, true, false, false),
		})

		metadata = append(metadata, MetadataWithLabels{
			Labels: map[string]string{"__name__": metricName},
			Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
			Help:   fmt.Sprintf("Histogram metric number %d", i),
			Unit:   "seconds",
		})

		exemplars = append(exemplars, ExemplarWithLabels{
			Labels:         labels,
			ExemplarLabels: map[string]string{"trace_id": fmt.Sprintf("trace_%d", i)},
			Value:          float64(i) / 10.0,
		})
	}

	runComplianceTest(t, "", "Test large scale request with many histograms, metadata, and exemplars",
		RequestOpts{
			Histograms: histograms,
			Metadata:   metadata,
			Exemplars:  exemplars,
		},
		requestParams{
			histograms: 50,
			exemplars:  50,
			retryable:  false,
		},
		true)
}

func TestSpecialCharactersCombined(t *testing.T) {
	sample := SampleWithLabels{Labels: map[string]string{
		"__name__":    "test_metric",
		"job":         "test-job_with.special@chars",
		"instance":    "localhost:9090",
		"environment": "test🚀",
	}, Value: 42.0}

	metadata := MetadataWithLabels{
		Labels: basicMetric("test_metric"),
		Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
		Help:   "Test metric with special characters: äöü, newlines\nand \"quotes\"",
		Unit:   "special/units",
	}

	exemplar := ExemplarWithLabels{
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
	}

	runComplianceTest(t, "", "Test combined request with special characters in labels and metadata",
		RequestOpts{
			Samples:   []SampleWithLabels{sample},
			Metadata:  []MetadataWithLabels{metadata},
			Exemplars: []ExemplarWithLabels{exemplar},
		},
		requestParams{
			samples:   1,
			exemplars: 1,
		},
		true)
}

func TestCustomBucketsHistogramWithExemplar(t *testing.T) {
	hist := HistogramWithLabels{
		Labels:    map[string]string{"__name__": "custom_latency_hist", "version": "v2"},
		Histogram: histogram(0.05, false, false, false, true, false), // Custom buckets
	}

	exemplar := ExemplarWithLabels{
		Labels:         map[string]string{"__name__": "custom_latency_hist", "version": "v2"},
		ExemplarLabels: map[string]string{"trace_id": "custom_bucket_trace", "bucket": "0.1"},
		Value:          0.05,
	}

	runComplianceTest(t, "", "Test custom buckets histogram with exemplar",
		RequestOpts{
			Exemplars:  []ExemplarWithLabels{exemplar},
			Histograms: []HistogramWithLabels{hist},
		},
		requestParams{
			histograms: 1,
			exemplars:  1,
			retryable:  false,
		},
		true)
}

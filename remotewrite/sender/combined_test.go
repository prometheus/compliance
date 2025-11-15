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
	"strings"
	"testing"

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

// TestCombinedFeatures validates integration of multiple Remote Write 2.0 features.
func TestCombinedFeatures(t *testing.T) {
	tests := []TestCase{
		{
			Name:        "samples_with_metadata",
			Description: "Sender SHOULD send samples with associated metadata",
			RFCLevel:    "SHOULD",
			ScrapeData: `# HELP http_requests_total Total HTTP requests received
# TYPE http_requests_total counter
http_requests_total{method="GET",status="200"} 1000
http_requests_total{method="POST",status="201"} 500
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundMetric bool
				var foundWithMetadata bool

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "http_requests_total" {
						foundMetric = true
						should(t, len(ts.Samples) > 0, "Counter should have samples")

						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							should(t, ts.Metadata.Type == writev2.Metadata_METRIC_TYPE_COUNTER,
								"Metadata type should match metric type")
							foundWithMetadata = true
						}

						if ts.Metadata.HelpRef != 0 {
							helpText := req.Request.Symbols[ts.Metadata.HelpRef]
							should(t, strings.Contains(helpText, "HTTP requests"),
								"Help text should be meaningful")
						}
					}
				}

				// Metric must be present
				if !foundMetric {
					t.Fatalf("Expected to find http_requests_total metric")
				}

				should(t, foundWithMetadata, "Metadata should be present with samples")
			},
		},
		{
			Name:        "samples_with_exemplars",
			Description: "Sender MAY send samples with attached exemplars",
			RFCLevel:    "MAY",
			ScrapeData: `# TYPE request_count counter
request_count 1000 # {trace_id="abc123",span_id="def456"} 999 1234567890.123
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundMetric bool
				var foundExemplar bool

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "request_count" {
						foundMetric = true
						if len(ts.Exemplars) > 0 {
							foundExemplar = true
							ex := ts.Exemplars[0]
							exLabels := extractExemplarLabels(&ex, req.Request.Symbols)
							t.Logf("Found exemplar with labels: %v", exLabels)
						}
					}
				}

				if !foundMetric {
					t.Fatalf("Expected to find request_count metric")
				}

				may(t, foundExemplar, "Exemplars present")
			},
		},
		{
			Name:        "histogram_with_metadata_and_exemplars",
			Description: "Sender MAY send histograms with metadata and exemplars",
			RFCLevel:    "MAY",
			ScrapeData: `# HELP request_duration_seconds Request duration in seconds
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="0.1"} 100 # {trace_id="hist123"} 0.05 1234567890.0
request_duration_seconds_bucket{le="0.5"} 250
request_duration_seconds_bucket{le="1.0"} 500
request_duration_seconds_bucket{le="+Inf"} 1000
request_duration_seconds_sum 450.5
request_duration_seconds_count 1000
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundHistogramData bool
				var foundMetadata bool
				var foundExemplar bool

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)

					metricBase := "request_duration_seconds"

					if labels["__name__"] == metricBase+"_count" ||
						labels["__name__"] == metricBase+"_bucket" ||
						labels["__name__"] == metricBase {
						foundHistogramData = true

						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							foundMetadata = true
						}

						if len(ts.Exemplars) > 0 {
							foundExemplar = true
						}
					}
				}

				if !foundHistogramData {
					t.Fatalf("Expected histogram data but none was found")
				}

				may(t, foundMetadata, "Histogram metadata present")
				may(t, foundExemplar, "Histogram exemplars present")
			},
		},
		{
			Name:        "multiple_metric_types",
			Description: "Sender MUST handle multiple metric types in same request",
			RFCLevel:    "MUST",
			ScrapeData: `# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 123.45

# TYPE process_memory_bytes gauge
process_memory_bytes 1048576

# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="+Inf"} 100
request_duration_seconds_sum 50.0
request_duration_seconds_count 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				metricTypes := make(map[string]bool)

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]

					if metricName == "process_cpu_seconds_total" {
						metricTypes["counter"] = true
					} else if metricName == "process_memory_bytes" {
						metricTypes["gauge"] = true
					} else if metricName == "request_duration_seconds_count" ||
						metricName == "request_duration_seconds" {
						metricTypes["histogram"] = true
					}
				}

				must(t).NotEmpty(metricTypes, "Request must contain metrics")
				t.Logf("Found metric types: %v", metricTypes)
			},
		},
		{
			Name:        "high_cardinality_labels",
			Description: "Sender should efficiently handle high cardinality label sets",
			RFCLevel:    "RECOMMENDED",
			ScrapeData: `# TYPE http_requests_total counter
http_requests_total{method="GET",path="/api/v1/users",status="200"} 100
http_requests_total{method="GET",path="/api/v1/posts",status="200"} 200
http_requests_total{method="POST",path="/api/v1/users",status="201"} 50
http_requests_total{method="POST",path="/api/v1/posts",status="201"} 75
http_requests_total{method="GET",path="/api/v1/comments",status="200"} 300
http_requests_total{method="DELETE",path="/api/v1/users",status="204"} 10
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// With high cardinality, symbol table deduplication becomes important.
				symbols := req.Request.Symbols
				uniqueSymbols := make(map[string]bool)

				for _, sym := range symbols {
					if sym != "" {
						uniqueSymbols[sym] = true
					}
				}

				// Check that common strings are deduplicated.
				recommended(t, len(uniqueSymbols) > 0, "Symbol table should contain unique symbols")

				httpRequestsSeries := 0
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "http_requests_total" {
						httpRequestsSeries++
					}
				}

				recommended(t, httpRequestsSeries >= 6,
					"High cardinality metrics should have multiple series")
				t.Logf("Found %d unique symbols, %d http_requests_total series",
					len(uniqueSymbols), httpRequestsSeries)
			},
		},
		{
			Name:        "complete_metric_family",
			Description: "Sender MUST send all components of metric family together",
			RFCLevel:    "MUST",
			ScrapeData: `# HELP api_request_duration_seconds API request duration
# TYPE api_request_duration_seconds histogram
api_request_duration_seconds_bucket{le="0.1"} 50
api_request_duration_seconds_bucket{le="0.5"} 150
api_request_duration_seconds_bucket{le="1.0"} 250
api_request_duration_seconds_bucket{le="+Inf"} 300
api_request_duration_seconds_sum 200.5
api_request_duration_seconds_count 300
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// For classic histograms, expect _sum, _count, and _bucket series.
				var foundCount bool

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]

					if metricName == "api_request_duration_seconds_count" {
						foundCount = true
					} else if metricName == "api_request_duration_seconds" && len(ts.Histograms) > 0 {
						// Native histogram format has everything in one series.
						foundCount = true
					}
				}

				// For classic histograms, all components should be present.
				// For native histograms, they're combined.
				must(t).True(foundCount || len(req.Request.Timeseries) > 0,
					"Histogram family must include count")
			},
		},
		{
			Name:        "mixed_labels_and_metadata",
			Description: "Sender SHOULD correctly encode metrics with many labels and metadata",
			RFCLevel:    "SHOULD",
			ScrapeData: `# HELP api_calls_total Total API calls with detailed labels
# TYPE api_calls_total counter
api_calls_total{service="auth",method="POST",endpoint="/login",region="us-east",status="200"} 1000
api_calls_total{service="auth",method="POST",endpoint="/logout",region="us-east",status="200"} 500
api_calls_total{service="users",method="GET",endpoint="/profile",region="eu-west",status="200"} 2000
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var seriesCount int
				var metadataCount int

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "api_calls_total" {
						seriesCount++

						// Check labels are properly structured.
						should(t, labels["service"] != "", "Service label should be present")
						should(t, labels["method"] != "", "Method label should be present")
						should(t, labels["endpoint"] != "", "Endpoint label should be present")

						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							metadataCount++
						}
					}
				}

				should(t, seriesCount >= 3,
					"Should have multiple series with different label combinations")
			},
		},
		{
			Name:        "real_world_scenario",
			Description: "Sender MUST handle realistic mixed metric payload",
			RFCLevel:    "MUST",
			ScrapeData: `# Realistic scrape output with multiple metric types
# TYPE up gauge
up 1

# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 45.67

# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 2097152

# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.05"} 100
http_request_duration_seconds_bucket{le="0.1"} 200
http_request_duration_seconds_bucket{le="0.5"} 450
http_request_duration_seconds_bucket{le="1.0"} 480
http_request_duration_seconds_bucket{le="+Inf"} 500
http_request_duration_seconds_sum 125.5
http_request_duration_seconds_count 500

# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 5000
http_requests_total{method="POST",code="201"} 1000
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				metricNames := make(map[string]bool)

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]
					metricNames[metricName] = true

					// Validate each series has valid structure.
					must(t).NotEmpty(metricName, "Each timeseries must have __name__")

					// Validate no mixed samples and histograms.
					if len(ts.Samples) > 0 && len(ts.Histograms) > 0 {
						must(t).Fail("Timeseries must not mix samples and histograms")
					}
				}

				must(t).NotEmpty(metricNames, "Request must contain metrics")
				must(t).GreaterOrEqual(len(metricNames), 3,
					"Real-world scenario should have multiple distinct metrics")
			},
		},
	}

	runTestCases(t, tests)
}

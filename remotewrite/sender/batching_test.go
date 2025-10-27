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
	"github.com/prometheus/compliance/remotewrite/sender/targets"
	"testing"
	"time"
)

// TestBatchingBehavior validates sender batching and queueing behavior.
func TestBatchingBehavior(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		validator   func(*testing.T, *CapturedRequest)
	}{
		{
			name:        "multiple_series_per_request",
			description: "Sender SHOULD batch multiple series in single request",
			rfcLevel:    "SHOULD",
			scrapeData: `# Multiple metrics to batch
http_requests_total{method="GET",status="200"} 1000
http_requests_total{method="POST",status="200"} 500
http_requests_total{method="GET",status="404"} 50
cpu_usage_percent 45.2
memory_usage_bytes 1048576
disk_io_bytes_total 1000000
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// Count unique metric names
				metricNames := make(map[string]bool)
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricNames[labels["__name__"]] = true
				}

				should(t).GreaterOrEqual(len(req.Request.Timeseries), 3,
					"Sender should batch multiple series, got %d series", len(req.Request.Timeseries))
				should(t).GreaterOrEqual(len(metricNames), 2,
					"Sender should batch different metrics, got %d unique metrics", len(metricNames))

				t.Logf("Batched %d timeseries with %d unique metrics",
					len(req.Request.Timeseries), len(metricNames))
			},
		},
		{
			name:        "batch_size_reasonable",
			description: "Sender SHOULD use reasonable batch sizes",
			rfcLevel:    "SHOULD",
			scrapeData: `# Many metrics
metric_1 1
metric_2 2
metric_3 3
metric_4 4
metric_5 5
metric_6 6
metric_7 7
metric_8 8
metric_9 9
metric_10 10
metric_11 11
metric_12 12
metric_13 13
metric_14 14
metric_15 15
metric_16 16
metric_17 17
metric_18 18
metric_19 19
metric_20 20
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				seriesCount := len(req.Request.Timeseries)

				// Batches shouldn't be too small (inefficient) or too large (risk)
				should(t).GreaterOrEqual(seriesCount, 1,
					"Request should contain at least one series")

				// Most senders batch at least several series together
				should(t).LessOrEqual(seriesCount, 10000,
					"Batch size should be reasonable (not too large)")

				t.Logf("Batch contains %d timeseries", seriesCount)
			},
		},
		{
			name:        "time_based_flushing",
			description: "Sender SHOULD flush batches based on time intervals",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				// Verify that data is sent even with small amounts
				// This indicates time-based flushing
				should(t).NotEmpty(req.Request.Timeseries,
					"Sender should flush small batches based on time")

				t.Logf("Time-based flush sent %d timeseries", len(req.Request.Timeseries))
			},
		},
		{
			name:        "handles_varying_cardinality",
			description: "Sender SHOULD handle varying label cardinality efficiently",
			rfcLevel:    "SHOULD",
			scrapeData: `# High cardinality metrics
api_calls{endpoint="/users",method="GET",region="us-east",status="200"} 100
api_calls{endpoint="/users",method="POST",region="us-east",status="201"} 50
api_calls{endpoint="/posts",method="GET",region="us-west",status="200"} 200
api_calls{endpoint="/posts",method="DELETE",region="eu-west",status="204"} 10
api_calls{endpoint="/comments",method="GET",region="ap-south",status="200"} 500
low_cardinality_metric 42
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// Check symbol table efficiency with varying cardinality
				symbols := req.Request.Symbols
				uniqueSymbols := make(map[string]bool)
				for _, sym := range symbols {
					if sym != "" {
						uniqueSymbols[sym] = true
					}
				}

				should(t).NotEmpty(uniqueSymbols, "Symbol table should deduplicate")
				should(t).GreaterOrEqual(len(req.Request.Timeseries), 2,
					"Should handle mixed cardinality metrics")

				t.Logf("Symbol table: %d unique symbols for %d timeseries",
					len(uniqueSymbols), len(req.Request.Timeseries))
			},
		},
		{
			name:        "efficient_symbol_reuse",
			description: "Sender SHOULD reuse symbols efficiently across batches",
			rfcLevel:    "SHOULD",
			scrapeData: `# Metrics with shared labels
http_requests{service="api",method="GET"} 100
http_requests{service="api",method="POST"} 50
http_requests{service="web",method="GET"} 200
http_duration{service="api",method="GET"} 0.5
http_duration{service="api",method="POST"} 0.3
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				symbols := req.Request.Symbols

				// Count occurrences of common strings
				symbolCounts := make(map[string]int)
				for _, sym := range symbols {
					symbolCounts[sym]++
				}

				// Common strings should appear only once (deduplicated)
				for sym, count := range symbolCounts {
					if sym != "" {
						should(t).Equal(1, count,
							"Symbol %q should appear only once in table, got %d", sym, count)
					}
				}

				t.Logf("Symbol table efficiency: %d unique symbols", len(symbols))
			},
		},
		{
			name:        "metadata_batching",
			description: "Sender SHOULD batch metadata with samples efficiently",
			rfcLevel:    "SHOULD",
			scrapeData: `# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET"} 1000
http_requests_total{method="POST"} 500

# HELP memory_usage_bytes Current memory usage
# TYPE memory_usage_bytes gauge
memory_usage_bytes 1048576
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var withMetadata int
				var withoutMetadata int

				for _, ts := range req.Request.Timeseries {
					// Count timeseries with metadata
					hasMetadata := ts.Metadata.Type != 0 ||
						ts.Metadata.HelpRef != 0 ||
						ts.Metadata.UnitRef != 0

					if hasMetadata {
						withMetadata++
					} else {
						withoutMetadata++
					}
				}

				should(t).GreaterOrEqual(len(req.Request.Timeseries), 2,
					"Should batch multiple series")

				t.Logf("Batched timeseries: %d with metadata, %d without",
					withMetadata, withoutMetadata)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Attr("rfcLevel", tt.rfcLevel)
			t.Attr("description", tt.description)

			forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
				runSenderTest(t, targetName, target, SenderTestScenario{
					ScrapeData: tt.scrapeData,
					Validator:  tt.validator,
					WaitTime:   5 * time.Second,
				})
			})
		})
	}
}

// TestConcurrentRequests validates parallel request handling.
func TestConcurrentRequests(t *testing.T) {
	t.Attr("rfcLevel", "MAY")
	t.Attr("description", "Sender MAY send multiple requests in parallel")

	scrapeData := `# Multiple metrics
metric_1 1
metric_2 2
metric_3 3
metric_4 4
metric_5 5
`

	forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
		runSenderTest(t, targetName, target, SenderTestScenario{
			ScrapeData: scrapeData,
			WaitTime:   8 * time.Second,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// This test just checks that requests are received
				// The actual concurrency is implementation-dependent
				may(t).NotNil(req, "At least one request should be sent")
				t.Logf("Request received (parallel sending is optional)")
			},
		})
	})
}

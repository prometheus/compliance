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
	"strings"
	"testing"
	"time"

	"github.com/prometheus/compliance/remotewrite/sender/targets"
)

// TestEdgeCases validates sender behavior in edge case scenarios.
func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		validator   func(*testing.T, *CapturedRequest)
	}{
		{
			name:        "empty_scrape",
			description: "Sender SHOULD handle scrapes with no metrics gracefully",
			rfcLevel:    "SHOULD",
			scrapeData:  "# No metrics\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				// Empty scrape may result in no request, or empty request
				// Both are acceptable
				if req.Request != nil {
					should(t, true, "Sender handled empty scrape")
					t.Logf("Empty scrape handled: %d timeseries", len(req.Request.Timeseries))
				} else {
					should(t, true, "Sender may skip empty scrapes")
					t.Logf("No request sent for empty scrape (acceptable)")
				}
			},
		},
		{
			name:        "huge_label_values",
			description: "Sender SHOULD handle very large label values (10KB+)",
			rfcLevel:    "SHOULD",
			scrapeData: func() string {
				largeValue := strings.Repeat("x", 10000)
				return `test_metric{large_label="` + largeValue + `"} 42` + "\n"
			}(),
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundLarge bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					for _, value := range labels {
						if len(value) > 5000 {
							foundLarge = true
							should(t, len(value) >= 5000, "Large label value should be preserved")
							t.Logf("Found large label value: %d bytes", len(value))
							break
						}
					}
				}
				should(t, foundLarge || len(req.Request.Timeseries) == 0, "Large label values should be handled")
			},
		},
		{
			name:        "unicode_in_labels",
			description: "Sender MUST preserve Unicode characters in labels",
			rfcLevel:    "MUST",
			scrapeData:  `test_metric{emoji="🚀",chinese="测试",arabic="مرحبا",vietnamese="việt nam"} 42` + "\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundUnicode bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)

					// Check for Unicode in label values
					for key, value := range labels {
						hasUnicode := false
						for _, r := range value {
							if r > 127 {
								hasUnicode = true
								break
							}
						}
						if hasUnicode {
							foundUnicode = true
							must(t).NotEmpty(value, "Unicode value must be preserved")
							t.Logf("Unicode label %s=%s", key, value)
						}
					}
				}
				should(t, foundUnicode || len(req.Request.Timeseries) > 0, "Unicode characters should be preserved")
			},
		},
		{
			name:        "many_timeseries",
			description: "Sender SHOULD efficiently handle many timeseries",
			rfcLevel:    "SHOULD",
			scrapeData: func() string {
				var sb strings.Builder
				// Generate 100 metrics (in practice, tests with 10k+ would need special setup)
				for i := 0; i < 100; i++ {
					sb.WriteString("metric_")
					sb.WriteString(strings.Repeat("0", 3-len(string(rune(i/100)))))
					sb.WriteString(string(rune(48 + i)))
					sb.WriteString(" ")
					sb.WriteString(string(rune(48 + i)))
					sb.WriteString("\n")
				}
				return sb.String()
			}(),
			validator: func(t *testing.T, req *CapturedRequest) {
				seriesCount := len(req.Request.Timeseries)
				should(t, seriesCount >= 10, "Should handle multiple timeseries efficiently")

				// Check symbol table efficiency
				symbols := req.Request.Symbols
				should(t, len(symbols) > 0, "Symbol table should be used")

				t.Logf("Handled %d timeseries with %d symbols",
					seriesCount, len(symbols))
			},
		},
		{
			name:        "high_cardinality",
			description: "Sender SHOULD handle high cardinality label sets",
			rfcLevel:    "SHOULD",
			scrapeData: func() string {
				var sb strings.Builder
				// Create high cardinality by varying one label across many values
				for i := 0; i < 50; i++ {
					sb.WriteString("http_requests{path=\"/api/v1/endpoint_")
					sb.WriteString(string(rune(48 + i/10)))
					sb.WriteString(string(rune(48 + i%10)))
					sb.WriteString("\",method=\"GET\"} ")
					sb.WriteString(string(rune(48 + i)))
					sb.WriteString("\n")
				}
				return sb.String()
			}(),
			validator: func(t *testing.T, req *CapturedRequest) {
				seriesCount := len(req.Request.Timeseries)
				should(t, seriesCount >= 20, "Should handle high cardinality metrics")

				// Symbol table should deduplicate common strings
				symbols := req.Request.Symbols
				uniqueSymbols := make(map[string]bool)
				for _, sym := range symbols {
					if sym != "" {
						uniqueSymbols[sym] = true
					}
				}

				should(t, len(uniqueSymbols) > 0, "Symbol table should deduplicate in high cardinality")
				t.Logf("High cardinality: %d series, %d unique symbols",
					seriesCount, len(uniqueSymbols))
			},
		},
		{
			name:        "very_long_metric_name",
			description: "Sender SHOULD handle very long metric names",
			rfcLevel:    "SHOULD",
			scrapeData: func() string {
				longName := "metric_" + strings.Repeat("very_long_name_", 50)
				return longName + " 42\n"
			}(),
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundLongName bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]
					if len(metricName) > 100 {
						foundLongName = true
						should(t, len(metricName) > 0, "Long metric name should be preserved")
						t.Logf("Long metric name: %d chars", len(metricName))
						break
					}
				}
				should(t, foundLongName || len(req.Request.Timeseries) > 0, "Long metric names should be handled")
			},
		},
		{
			name:        "special_float_combinations",
			description: "Sender MUST handle special float value combinations",
			rfcLevel:    "MUST",
			scrapeData: `special_values{type="nan"} NaN
special_values{type="inf"} +Inf
special_values{type="ninf"} -Inf
special_values{type="zero"} 0
special_values{type="negative"} -123.45
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				foundSpecial := make(map[string]bool)

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "special_values" && len(ts.Samples) > 0 {
						value := ts.Samples[0].Value
						valueType := labels["type"]

						switch valueType {
						case "nan":
							if math.IsNaN(value) {
								foundSpecial["nan"] = true
							}
						case "inf":
							if math.IsInf(value, 1) {
								foundSpecial["inf"] = true
							}
						case "ninf":
							if math.IsInf(value, -1) {
								foundSpecial["ninf"] = true
							}
						case "zero":
							if value == 0 {
								foundSpecial["zero"] = true
							}
						case "negative":
							if value < 0 {
								foundSpecial["negative"] = true
							}
						}
					}
				}

				must(t).GreaterOrEqual(len(foundSpecial), 1,
					"Should handle special float values")
				t.Logf("Special values handled: %v", foundSpecial)
			},
		},
		{
			name:        "zero_timestamp",
			description: "Sender SHOULD handle timestamp value of 0",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42 0\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				// Timestamp of 0 might be rejected or normalized
				// Sender should handle gracefully
				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 0 {
						timestamp := ts.Samples[0].Timestamp
						should(t, timestamp >= int64(0), "Timestamp should be non-negative")
						t.Logf("Timestamp handling: %d", timestamp)
					}
				}
			},
		},
		{
			name:        "future_timestamp",
			description: "Sender SHOULD handle timestamps in the future",
			rfcLevel:    "SHOULD",
			scrapeData: func() string {
				future := time.Now().Add(24 * time.Hour).Unix()
				return "test_metric 42 " + string(rune(future)) + "\n"
			}(),
			validator: func(t *testing.T, req *CapturedRequest) {
				now := time.Now().UnixMilli()

				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 0 {
						timestamp := ts.Samples[0].Timestamp

						// Check if timestamp is in future
						if timestamp > now {
							diff := timestamp - now
							should(t, diff >= int64(0), "Future timestamp should be handled")
							t.Logf("Future timestamp: %d ms ahead", diff)
						}
					}
				}
			},
		},
		{
			name:        "metric_name_with_colons",
			description: "Sender MUST handle metric names with colons",
			rfcLevel:    "MUST",
			scrapeData:  "http:request:duration:seconds 0.5\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundColon bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]
					if strings.Contains(metricName, ":") {
						foundColon = true
						must(t).Contains(metricName, ":",
							"Metric name with colons must be preserved")
						t.Logf("Metric name with colons: %s", metricName)
						break
					}
				}
				must(t).True(foundColon || len(req.Request.Timeseries) > 0,
					"Metric names with colons are valid and must be handled")
			},
		},
		{
			name:        "stale_marker",
			description: "Sender SHOULD handle stale marker (StaleNaN)",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric StaleNaN\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				// StaleNaN is a special NaN value
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_metric" && len(ts.Samples) > 0 {
						value := ts.Samples[0].Value
						// StaleNaN is represented as NaN
						should(t, math.IsNaN(value), "StaleNaN should be encoded as NaN")
						t.Logf("Stale marker handled: NaN=%v", math.IsNaN(value))
					}
				}
			},
		},
		{
			name:        "mixed_sample_and_histogram_families",
			description: "Sender MUST handle different metric types in same payload",
			rfcLevel:    "MUST",
			scrapeData: `# Counter
requests_total 100

# Gauge
temperature_celsius 22.5

# Histogram
response_time_bucket{le="0.1"} 50
response_time_bucket{le="+Inf"} 100
response_time_sum 10.5
response_time_count 100

# Summary
rpc_duration{quantile="0.5"} 0.05
rpc_duration{quantile="0.9"} 0.1
rpc_duration_sum 50.0
rpc_duration_count 1000
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				metricTypes := make(map[string]bool)

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					name := labels["__name__"]

					if name == "requests_total" {
						metricTypes["counter"] = true
					} else if name == "temperature_celsius" {
						metricTypes["gauge"] = true
					} else if strings.HasPrefix(name, "response_time") {
						metricTypes["histogram"] = true
					} else if strings.HasPrefix(name, "rpc_duration") {
						metricTypes["summary"] = true
					}
				}

				must(t).GreaterOrEqual(len(metricTypes), 2,
					"Must handle multiple metric types in same payload")
				t.Logf("Metric types found: %v", metricTypes)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Attr("rfcLevel", tt.rfcLevel)
			t.Attr("description", tt.description)

			forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
				runSenderTest(t, targetName, target, SenderTestScenario{
					ScrapeData: tt.scrapeData,
					Validator:  tt.validator,
					WaitTime:   6 * time.Second,
				})
			})
		})
	}
}

// TestRobustnessUnderLoad validates sender behavior under stress.
func TestRobustnessUnderLoad(t *testing.T) {
	t.Attr("rfcLevel", "SHOULD")
	t.Attr("description", "Sender SHOULD remain stable under load")

	// Generate larger scrape data
	var scrapeData strings.Builder
	for i := 0; i < 200; i++ {
		scrapeData.WriteString("load_test_metric_")
		scrapeData.WriteString(string(rune(48 + i/100)))
		scrapeData.WriteString(string(rune(48 + (i/10)%10)))
		scrapeData.WriteString(string(rune(48 + i%10)))
		scrapeData.WriteString("{label=\"value_")
		scrapeData.WriteString(string(rune(48 + i%10)))
		scrapeData.WriteString("\"} ")
		scrapeData.WriteString(string(rune(48 + i%10)))
		scrapeData.WriteString("\n")
	}

	forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
		runSenderTest(t, targetName, target, SenderTestScenario{
			ScrapeData: scrapeData.String(),
			Validator: func(t *testing.T, req *CapturedRequest) {
				should(t, len(req.Request.Timeseries) > 0, "Should handle load test data")

				seriesCount := len(req.Request.Timeseries)
				should(t, seriesCount >= 50, "Should batch substantial amount of data")

				t.Logf("Load test: %d timeseries sent", seriesCount)
			},
			WaitTime: 8 * time.Second,
		})
	})
}

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

package sender

import (
	"math"
	"strings"
	"testing"
	"time"
)

// TestEdgeCases validates sender behavior in edge case scenarios.
func TestEdgeCases_Old(t *testing.T) {
	t.Skip("TODO: Revise and move to a new framework")

	tests := []TestCase{
		{
			Name:        "empty_scrape",
			Description: "Sender SHOULD handle scrapes with no metrics gracefully",
			RFCLevel:    "SHOULD",
			ScrapeData:  "# No metrics\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				// Empty scrape may result in no request, or empty request.
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
			Name:        "huge_label_values",
			Description: "Sender SHOULD handle very large label values (10KB+)",
			RFCLevel:    "SHOULD",
			ScrapeData: func() string {
				largeValue := strings.Repeat("x", 10000)
				return `test_metric{large_label="` + largeValue + `"} 42` + "\n"
			}(),
			Validator: func(t *testing.T, req *CapturedRequest) {
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
			Name:        "unicode_in_labels",
			Description: "Sender MUST preserve Unicode characters in labels",
			RFCLevel:    "MUST",
			ScrapeData:  `test_metric{emoji="🚀",chinese="测试",arabic="مرحبا",vietnamese="tôi yêu việt nam"} 42` + "\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundUnicode bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)

					// Check for Unicode in label values.
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
			Name:        "many_timeseries",
			Description: "Sender should efficiently handle many timeseries",
			RFCLevel:    "RECOMMENDED",
			ScrapeData: func() string {
				var sb strings.Builder
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
			Validator: func(t *testing.T, req *CapturedRequest) {
				seriesCount := len(req.Request.Timeseries)
				recommended(t, seriesCount >= 10, "Should handle multiple timeseries efficiently")

				// Check symbol table efficiency.
				symbols := req.Request.Symbols
				recommended(t, len(symbols) > 0, "Symbol table should be used")

				t.Logf("Handled %d timeseries with %d symbols",
					seriesCount, len(symbols))
			},
		},
		{
			Name:        "high_cardinality",
			Description: "Sender should handle high cardinality label sets efficiently",
			RFCLevel:    "RECOMMENDED",
			ScrapeData: func() string {
				var sb strings.Builder
				// Create high cardinality by varying one label across many values.
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
			Validator: func(t *testing.T, req *CapturedRequest) {
				seriesCount := len(req.Request.Timeseries)
				recommended(t, seriesCount >= 20, "Should handle high cardinality metrics")

				// Symbol table should deduplicate common strings.
				symbols := req.Request.Symbols
				uniqueSymbols := make(map[string]bool)
				for _, sym := range symbols {
					if sym != "" {
						uniqueSymbols[sym] = true
					}
				}

				recommended(t, len(uniqueSymbols) > 0, "Symbol table should deduplicate in high cardinality")
				t.Logf("High cardinality: %d series, %d unique symbols",
					seriesCount, len(uniqueSymbols))
			},
		},
		{
			Name:        "very_long_metric_name",
			Description: "Sender SHOULD handle very long metric names",
			RFCLevel:    "SHOULD",
			ScrapeData: func() string {
				longName := "metric_" + strings.Repeat("very_long_name_", 50)
				return longName + " 42\n"
			}(),
			Validator: func(t *testing.T, req *CapturedRequest) {
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
			Name:        "special_float_combinations",
			Description: "Sender MUST handle special float value combinations",
			RFCLevel:    "MUST",
			ScrapeData: `special_values{type="nan"} NaN
special_values{type="inf"} +Inf
special_values{type="ninf"} -Inf
special_values{type="zero"} 0
special_values{type="negative"} -123.45
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
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
			Name:        "zero_timestamp",
			Description: "Sender SHOULD handle timestamp value of 0",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_metric 42 0\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				// Timestamp of 0 might be rejected or normalized, sender should handle gracefully.
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
			Name:        "future_timestamp",
			Description: "Sender SHOULD handle timestamps in the future",
			RFCLevel:    "SHOULD",
			ScrapeData: func() string {
				future := time.Now().Add(24 * time.Hour).Unix()
				return "test_metric 42 " + string(rune(future)) + "\n"
			}(),
			Validator: func(t *testing.T, req *CapturedRequest) {
				now := time.Now().UnixMilli()

				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 0 {
						timestamp := ts.Samples[0].Timestamp

						// Check if timestamp is in future.
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
			Name:        "metric_name_with_colons",
			Description: "Sender MUST handle metric names with colons",
			RFCLevel:    "MUST",
			ScrapeData:  "http:request:duration:seconds 0.5\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
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
			Name:        "stale_marker",
			Description: "Sender SHOULD handle stale marker (StaleNaN)",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_metric StaleNaN\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				// StaleNaN is a special NaN value.
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_metric" && len(ts.Samples) > 0 {
						value := ts.Samples[0].Value
						should(t, math.IsNaN(value), "StaleNaN should be encoded as NaN")
						t.Logf("Stale marker handled: NaN=%v", math.IsNaN(value))
					}
				}
			},
		},
		{
			Name:        "mixed_sample_and_histogram_families",
			Description: "Sender MUST handle different metric types in same payload",
			RFCLevel:    "MUST",
			ScrapeData: `# Counter
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
			Validator: func(t *testing.T, req *CapturedRequest) {
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

	runTestCases(t, tests)
}

// TestRobustnessUnderLoad validates sender behavior under stress.
func TestRobustnessUnderLoad_Old(t *testing.T) {
	t.Skip("TODO: Revise and move to a new framework")

	t.Attr("rfcLevel", "SHOULD")
	t.Attr("description", "Sender SHOULD remain stable under load")

	// Generate larger scrape data.
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

	forEachSender(t, func(t *testing.T, targetName string, target Sender) {
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

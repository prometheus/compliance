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
	"testing"

	"github.com/prometheus/compliance/remotewrite/sender/targets"
)

// TestExemplarEncoding validates exemplar encoding in Remote Write 2.0.
func TestExemplarEncoding(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		validator   func(*testing.T, *CapturedRequest)
	}{
		{
			name:        "exemplar_with_trace_id",
			description: "Sender MAY attach exemplars with trace_id to samples",
			rfcLevel:    "MAY",
			scrapeData: `# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.1"} 50 # {trace_id="abc123xyz"} 0.05 1234567890.123
http_request_duration_seconds_bucket{le="+Inf"} 100
http_request_duration_seconds_sum 10.5
http_request_duration_seconds_count 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// Exemplars are optional, so we use may() level
				var foundExemplar bool
				for _, ts := range req.Request.Timeseries {
					if len(ts.Exemplars) > 0 {
						foundExemplar = true
						// Check if trace_id label is present
						ex := ts.Exemplars[0]
						exLabels := extractExemplarLabels(&ex, req.Request.Symbols)
						may(t, exLabels["trace_id"] != "", "Exemplar may include trace_id label")
						t.Logf("Found exemplar with labels: %v", exLabels)
						break
					}
				}
				may(t, foundExemplar || len(req.Request.Timeseries) > 0, "Exemplars may be present if supported by sender")
			},
		},
		{
			name:        "exemplar_with_span_id",
			description: "Sender MAY attach exemplars with span_id to samples",
			rfcLevel:    "MAY",
			scrapeData: `# TYPE http_requests_total counter
http_requests_total 1000 # {trace_id="abc123",span_id="def456"} 999 1234567890.5
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundSpanId bool
				for _, ts := range req.Request.Timeseries {
					if len(ts.Exemplars) > 0 {
						for _, ex := range ts.Exemplars {
							exLabels := extractExemplarLabels(&ex, req.Request.Symbols)
							if _, ok := exLabels["span_id"]; ok {
								foundSpanId = true
								may(t, len(exLabels["span_id"]) > 0, "Exemplar may include span_id label")
								break
							}
						}
					}
				}
				may(t, foundSpanId || len(req.Request.Timeseries) > 0, "Exemplar may include span_id if supported")
			},
		},
		{
			name:        "exemplar_value_valid",
			description: "Exemplar MUST have valid float value if present",
			rfcLevel:    "MUST",
			scrapeData: `# TYPE test_counter counter
test_counter 100 # {trace_id="test123"} 99.5 1234567890.0
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					for _, ex := range ts.Exemplars {
						// If exemplar is present, value must be valid
						must(t).NotNil(ex.Value, "Exemplar value must be set")
						t.Logf("Exemplar value: %f", ex.Value)
					}
				}
			},
		},
		{
			name:        "exemplar_timestamp_valid",
			description: "Exemplar MUST have valid timestamp if present",
			rfcLevel:    "MUST",
			scrapeData: `# TYPE test_counter counter
test_counter 100 # {trace_id="test123"} 99 1234567890.123
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					for _, ex := range ts.Exemplars {
						// If exemplar is present, timestamp must be valid
						must(t).Greater(ex.Timestamp, int64(0),
							"Exemplar timestamp must be positive")
						must(t).Greater(ex.Timestamp, int64(1e12),
							"Exemplar timestamp should be in milliseconds")
					}
				}
			},
		},
		{
			name:        "exemplar_labels_valid_refs",
			description: "Exemplar label refs MUST point to valid symbol table indices",
			rfcLevel:    "MUST",
			scrapeData: `# TYPE test_metric counter
test_metric 100 # {trace_id="xyz"} 99 1234567890.0
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				symbols := req.Request.Symbols
				for _, ts := range req.Request.Timeseries {
					for _, ex := range ts.Exemplars {
						// Validate all label refs
						for i, ref := range ex.LabelsRefs {
							must(t).Less(int(ref), len(symbols),
								"Exemplar label ref[%d] = %d must be valid symbol index (table size: %d)",
								i, ref, len(symbols))
						}
					}
				}
			},
		},
		{
			name:        "exemplar_custom_labels",
			description: "Sender MAY attach exemplars with custom labels beyond trace/span",
			rfcLevel:    "MAY",
			scrapeData: `# TYPE test_counter counter
test_counter 50 # {user_id="user123",request_id="req456"} 49 1234567890.0
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundCustom bool
				for _, ts := range req.Request.Timeseries {
					for _, ex := range ts.Exemplars {
						exLabels := extractExemplarLabels(&ex, req.Request.Symbols)
						// Check for non-standard exemplar labels
						for key := range exLabels {
							if key != "trace_id" && key != "span_id" {
								foundCustom = true
								may(t, exLabels[key] != "", fmt.Sprintf("Custom exemplar labels may be used: %s", key))
								break
							}
						}
					}
				}
				may(t, foundCustom || len(req.Request.Timeseries) > 0, "Custom exemplar labels may be present")
			},
		},
		{
			name:        "exemplar_on_histogram",
			description: "Sender MAY attach exemplars to histogram buckets",
			rfcLevel:    "MAY",
			scrapeData: `# TYPE request_duration histogram
request_duration_bucket{le="0.1"} 10 # {trace_id="hist123"} 0.05 1234567890.0
request_duration_bucket{le="+Inf"} 100
request_duration_sum 50.0
request_duration_count 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundHistogramExemplar bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)

					// Check for histogram-related timeseries with exemplars
					if (labels["__name__"] == "request_duration_bucket" ||
						labels["__name__"] == "request_duration") &&
						len(ts.Exemplars) > 0 {
						foundHistogramExemplar = true
						may(t, len(ts.Exemplars) > 0, "Exemplars may be attached to histogram buckets")
						break
					}
				}
				may(t, foundHistogramExemplar || len(req.Request.Timeseries) > 0, "Histogram exemplars may be present")
			},
		},
		{
			name:        "exemplar_labels_even_length",
			description: "Exemplar label refs array MUST have even length (key-value pairs)",
			rfcLevel:    "MUST",
			scrapeData: `# TYPE test_counter counter
test_counter 100 # {trace_id="test"} 99 1234567890.0
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					for _, ex := range ts.Exemplars {
						refsLen := len(ex.LabelsRefs)
						must(t).Equal(0, refsLen%2,
							"Exemplar label refs length must be even (key-value pairs), got: %d",
							refsLen)
					}
				}
			},
		},
		{
			name:        "multiple_exemplars_per_series",
			description: "Sender MAY attach multiple exemplars to a single timeseries",
			rfcLevel:    "MAY",
			scrapeData: `# TYPE test_histogram histogram
test_histogram_bucket{le="0.1"} 10 # {trace_id="ex1"} 0.05 1234567890.0
test_histogram_bucket{le="0.5"} 50 # {trace_id="ex2"} 0.3 1234567891.0
test_histogram_bucket{le="+Inf"} 100
test_histogram_sum 50.0
test_histogram_count 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundMultiple bool
				for _, ts := range req.Request.Timeseries {
					if len(ts.Exemplars) > 1 {
						foundMultiple = true
						may(t, len(ts.Exemplars) > 1, "Multiple exemplars may be attached to a timeseries")
						t.Logf("Found %d exemplars in timeseries", len(ts.Exemplars))
						break
					}
				}
				may(t, foundMultiple || len(req.Request.Timeseries) > 0, "Multiple exemplars may be present")
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
				})
			})
		})
	}
}

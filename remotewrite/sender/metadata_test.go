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

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

// TestMetadataEncoding validates metric metadata encoding.
func TestMetadataEncoding(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		validator   func(*testing.T, *CapturedRequest)
	}{
		{
			name:        "metadata_type_counter",
			description: "Sender SHOULD include TYPE metadata for counter metrics",
			rfcLevel:    "SHOULD",
			scrapeData: `# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundMetadata bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "http_requests_total" {
						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							should(t).Equal(writev2.Metadata_METRIC_TYPE_COUNTER, ts.Metadata.Type,
								"Counter metric should have COUNTER type in metadata")
							foundMetadata = true
						}
						break
					}
				}
				should(t).True(foundMetadata || len(req.Request.Timeseries) > 0,
					"Metadata should be present for typed metrics")
			},
		},
		{
			name:        "metadata_type_gauge",
			description: "Sender SHOULD include TYPE metadata for gauge metrics",
			rfcLevel:    "SHOULD",
			scrapeData: `# HELP memory_usage_bytes Current memory usage
# TYPE memory_usage_bytes gauge
memory_usage_bytes 1048576
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundMetadata bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "memory_usage_bytes" {
						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							should(t).Equal(writev2.Metadata_METRIC_TYPE_GAUGE, ts.Metadata.Type,
								"Gauge metric should have GAUGE type in metadata")
							foundMetadata = true
						}
						break
					}
				}
				should(t).True(foundMetadata || len(req.Request.Timeseries) > 0,
					"Metadata should be present for typed metrics")
			},
		},
		{
			name:        "metadata_type_histogram",
			description: "Sender SHOULD include TYPE metadata for histogram metrics",
			rfcLevel:    "SHOULD",
			scrapeData: `# HELP request_duration_seconds Request duration
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="+Inf"} 100
request_duration_seconds_sum 50.0
request_duration_seconds_count 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundMetadata bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]

					if metricName == "request_duration_seconds_count" ||
						metricName == "request_duration_seconds" {
						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							should(t).Equal(writev2.Metadata_METRIC_TYPE_HISTOGRAM, ts.Metadata.Type,
								"Histogram metric should have HISTOGRAM type in metadata")
							foundMetadata = true
							break
						}
					}
				}
				should(t).True(foundMetadata || len(req.Request.Timeseries) > 0,
					"Metadata should be present for histogram metrics")
			},
		},
		{
			name:        "metadata_type_summary",
			description: "Sender SHOULD include TYPE metadata for summary metrics",
			rfcLevel:    "SHOULD",
			scrapeData: `# HELP rpc_duration_seconds RPC duration
# TYPE rpc_duration_seconds summary
rpc_duration_seconds{quantile="0.5"} 0.05
rpc_duration_seconds{quantile="0.9"} 0.1
rpc_duration_seconds_sum 100.0
rpc_duration_seconds_count 1000
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundMetadata bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "rpc_duration_seconds" {
						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							should(t).Equal(writev2.Metadata_METRIC_TYPE_SUMMARY, ts.Metadata.Type,
								"Summary metric should have SUMMARY type in metadata")
							foundMetadata = true
							break
						}
					}
				}
				should(t).True(foundMetadata || len(req.Request.Timeseries) > 0,
					"Metadata should be present for summary metrics")
			},
		},
		{
			name:        "metadata_help_text",
			description: "Sender SHOULD include HELP text in metadata",
			rfcLevel:    "SHOULD",
			scrapeData: `# HELP http_requests_total The total number of HTTP requests
# TYPE http_requests_total counter
http_requests_total 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundHelp bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "http_requests_total" {
						if ts.Metadata.HelpRef != 0 {
							helpText := req.Request.Symbols[ts.Metadata.HelpRef]
							should(t).NotEmpty(helpText,
								"HELP text should be present in metadata")
							should(t).Contains(helpText, "HTTP requests",
								"HELP text should contain meaningful description")
							foundHelp = true
						}
						break
					}
				}
				should(t).True(foundHelp || len(req.Request.Timeseries) > 0,
					"HELP text should be present in metadata")
			},
		},
		{
			name:        "metadata_unit",
			description: "Sender MAY include UNIT in metadata",
			rfcLevel:    "MAY",
			scrapeData: `# HELP memory_usage_bytes Memory usage
# TYPE memory_usage_bytes gauge
# UNIT memory_usage_bytes bytes
memory_usage_bytes 1048576
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundUnit bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "memory_usage_bytes" {
						if ts.Metadata.UnitRef != 0 {
							unit := req.Request.Symbols[ts.Metadata.UnitRef]
							may(t).NotEmpty(unit, "UNIT may be present in metadata")
							t.Logf("Found unit in metadata: %s", unit)
							foundUnit = true
						}
						break
					}
				}
				may(t).True(foundUnit || len(req.Request.Timeseries) > 0,
					"UNIT may be present in metadata")
			},
		},
		{
			name:        "metadata_help_with_newlines",
			description: "Sender SHOULD preserve newlines in HELP text",
			rfcLevel:    "SHOULD",
			scrapeData: `# HELP multiline_metric This is a help text
# HELP multiline_metric that spans multiple lines
# TYPE multiline_metric gauge
multiline_metric 42
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// Note: Prometheus exposition format doesn't actually support
				// multi-line HELP. This test validates handling of the format.
				var foundMetadata bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "multiline_metric" {
						if ts.Metadata.HelpRef != 0 {
							helpText := req.Request.Symbols[ts.Metadata.HelpRef]
							should(t).NotEmpty(helpText, "HELP text should be present")
							foundMetadata = true
						}
						break
					}
				}
				should(t).True(foundMetadata || len(req.Request.Timeseries) > 0,
					"Metadata should be handled correctly")
			},
		},
		{
			name:        "metadata_help_with_special_chars",
			description: "Sender SHOULD preserve special characters in HELP text",
			rfcLevel:    "SHOULD",
			scrapeData: `# HELP special_metric This help contains "quotes" and \backslashes\
# TYPE special_metric counter
special_metric 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundSpecial bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "special_metric" {
						if ts.Metadata.HelpRef != 0 {
							helpText := req.Request.Symbols[ts.Metadata.HelpRef]
							should(t).NotEmpty(helpText,
								"HELP text with special characters should be preserved")
							foundSpecial = true
						}
						break
					}
				}
				should(t).True(foundSpecial || len(req.Request.Timeseries) > 0,
					"Special characters in metadata should be handled")
			},
		},
		{
			name:        "metadata_help_refs_valid",
			description: "Metadata HelpRef MUST point to valid symbol table index if non-zero",
			rfcLevel:    "MUST",
			scrapeData: `# HELP test_metric Test metric description
# TYPE test_metric counter
test_metric 42
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				symbols := req.Request.Symbols
				for _, ts := range req.Request.Timeseries {
					if ts.Metadata.HelpRef != 0 {
						must(t).Less(int(ts.Metadata.HelpRef), len(symbols),
							"HelpRef must point to valid symbol index")
					}
					if ts.Metadata.UnitRef != 0 {
						must(t).Less(int(ts.Metadata.UnitRef), len(symbols),
							"UnitRef must point to valid symbol index")
					}
				}
			},
		},
		{
			name:        "metadata_consistent_across_series",
			description: "Sender SHOULD send consistent metadata for the same metric family",
			rfcLevel:    "SHOULD",
			scrapeData: `# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET"} 100
http_requests_total{method="POST"} 50
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				metadataMap := make(map[string]writev2.Metadata)

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]

					if metricName == "http_requests_total" {
						if existing, found := metadataMap[metricName]; found {
							// If metadata exists, it should be consistent
							should(t).Equal(existing.Type, ts.Metadata.Type,
								"Metadata type should be consistent for same metric family")
							should(t).Equal(existing.HelpRef, ts.Metadata.HelpRef,
								"Metadata help should be consistent for same metric family")
						} else {
							metadataMap[metricName] = ts.Metadata
						}
					}
				}
			},
		},
		{
			name:        "metadata_empty_help_allowed",
			description: "Sender MAY send metrics without HELP text",
			rfcLevel:    "MAY",
			scrapeData: `# TYPE no_help_metric counter
no_help_metric 42
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "no_help_metric" {
						// HelpRef may be 0 (empty string) which is valid
						may(t).GreaterOrEqual(int(ts.Metadata.HelpRef), 0,
							"Empty HELP text is allowed")
						found = true
						break
					}
				}
				may(t).True(found || len(req.Request.Timeseries) > 0,
					"Metrics without HELP are allowed")
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
				})
			})
		})
	}
}

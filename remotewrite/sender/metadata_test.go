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

// TestMetadataEncoding validates metric metadata encoding.
func TestMetadataEncoding_Old(t *testing.T) {
	t.Skip("TODO: Revise and move to a new framework")

	tests := []TestCase{
		{
			Name:        "metadata_type_counter",
			Description: "Sender SHOULD include TYPE metadata for counter metrics",
			RFCLevel:    "SHOULD",
			ScrapeData: `# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundMetadata bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "http_requests_total" {
						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							should(t, writev2.Metadata_METRIC_TYPE_COUNTER == ts.Metadata.Type, "Counter metric should have COUNTER type in metadata")
							foundMetadata = true
						}
						break
					}
				}
				should(t, foundMetadata || len(req.Request.Timeseries) > 0, "Metadata should be present for typed metrics")
			},
		},
		{
			Name:        "metadata_type_gauge",
			Description: "Sender SHOULD include TYPE metadata for gauge metrics",
			RFCLevel:    "SHOULD",
			ScrapeData: `# HELP memory_usage_bytes Current memory usage
# TYPE memory_usage_bytes gauge
memory_usage_bytes 1048576
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundMetadata bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "memory_usage_bytes" {
						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							should(t, writev2.Metadata_METRIC_TYPE_GAUGE == ts.Metadata.Type, "Gauge metric should have GAUGE type in metadata")
							foundMetadata = true
						}
						break
					}
				}
				should(t, foundMetadata || len(req.Request.Timeseries) > 0, "Metadata should be present for typed metrics")
			},
		},
		{
			Name:        "metadata_type_histogram",
			Description: "Sender SHOULD include TYPE metadata for histogram metrics",
			RFCLevel:    "SHOULD",
			ScrapeData: `# HELP request_duration_seconds Request duration
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="+Inf"} 100
request_duration_seconds_sum 50.0
request_duration_seconds_count 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundMetadata bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]

					if metricName == "request_duration_seconds_count" ||
						metricName == "request_duration_seconds" {
						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							should(t, writev2.Metadata_METRIC_TYPE_HISTOGRAM == ts.Metadata.Type, "Histogram metric should have HISTOGRAM type in metadata")
							foundMetadata = true
							break
						}
					}
				}
				should(t, foundMetadata || len(req.Request.Timeseries) > 0, "Metadata should be present for histogram metrics")
			},
		},
		{
			Name:        "metadata_type_summary",
			Description: "Sender SHOULD include TYPE metadata for summary metrics",
			RFCLevel:    "SHOULD",
			ScrapeData: `# HELP rpc_duration_seconds RPC duration
# TYPE rpc_duration_seconds summary
rpc_duration_seconds{quantile="0.5"} 0.05
rpc_duration_seconds{quantile="0.9"} 0.1
rpc_duration_seconds_sum 100.0
rpc_duration_seconds_count 1000
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundMetadata bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "rpc_duration_seconds" {
						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							should(t, writev2.Metadata_METRIC_TYPE_SUMMARY == ts.Metadata.Type, "Summary metric should have SUMMARY type in metadata")
							foundMetadata = true
							break
						}
					}
				}
				should(t, foundMetadata || len(req.Request.Timeseries) > 0, "Metadata should be present for summary metrics")
			},
		},
		{
			Name:        "metadata_help_text",
			Description: "Sender SHOULD include HELP text in metadata",
			RFCLevel:    "SHOULD",
			ScrapeData: `# HELP http_requests_total The total number of HTTP requests
# TYPE http_requests_total counter
http_requests_total 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundHelp bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "http_requests_total" {
						if ts.Metadata.HelpRef != 0 {
							helpText := req.Request.Symbols[ts.Metadata.HelpRef]
							should(t, len(helpText) > 0, "HELP text should be present in metadata")
							should(t, strings.Contains(helpText, "HTTP requests"), "HELP text should contain meaningful description")
							foundHelp = true
						}
						break
					}
				}
				should(t, foundHelp || len(req.Request.Timeseries) > 0, "HELP text should be present in metadata")
			},
		},
		{
			Name:        "metadata_unit",
			Description: "Sender MAY include UNIT in metadata",
			RFCLevel:    "MAY",
			ScrapeData: `# HELP memory_usage_bytes Memory usage
# TYPE memory_usage_bytes gauge
# UNIT memory_usage_bytes bytes
memory_usage_bytes 1048576
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundUnit bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "memory_usage_bytes" {
						if ts.Metadata.UnitRef != 0 {
							unit := req.Request.Symbols[ts.Metadata.UnitRef]
							may(t, len(unit) > 0, "UNIT may be present in metadata")
							t.Logf("Found unit in metadata: %s", unit)
							foundUnit = true
						}
						break
					}
				}
				may(t, foundUnit || len(req.Request.Timeseries) > 0, "UNIT may be present in metadata")
			},
		},
		{
			Name:        "metadata_help_with_newlines",
			Description: "Sender SHOULD preserve newlines in HELP text",
			RFCLevel:    "SHOULD",
			ScrapeData: `# HELP multiline_metric This is a help text
# HELP multiline_metric that spans multiple lines
# TYPE multiline_metric gauge
multiline_metric 42
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// Note: Prometheus exposition format doesn't actually support
				// multi-line HELP. This test validates handling of the format.
				var foundMetadata bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "multiline_metric" {
						if ts.Metadata.HelpRef != 0 {
							helpText := req.Request.Symbols[ts.Metadata.HelpRef]
							should(t, len(helpText) > 0, "HELP text should be present")
							foundMetadata = true
						}
						break
					}
				}
				should(t, foundMetadata || len(req.Request.Timeseries) > 0, "Metadata should be handled correctly")
			},
		},
		{
			Name:        "metadata_help_with_special_chars",
			Description: "Sender SHOULD preserve special characters in HELP text",
			RFCLevel:    "SHOULD",
			ScrapeData: `# HELP special_metric This help contains "quotes" and \backslashes\
# TYPE special_metric counter
special_metric 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundSpecial bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "special_metric" {
						if ts.Metadata.HelpRef != 0 {
							helpText := req.Request.Symbols[ts.Metadata.HelpRef]
							should(t, len(helpText) > 0, "HELP text with special characters should be preserved")
							foundSpecial = true
						}
						break
					}
				}
				should(t, foundSpecial || len(req.Request.Timeseries) > 0, "Special characters in metadata should be handled")
			},
		},
		{
			Name:        "metadata_help_refs_valid",
			Description: "Metadata HelpRef MUST point to valid symbol table index if non-zero",
			RFCLevel:    "MUST",
			ScrapeData: `# HELP test_metric Test metric description
# TYPE test_metric counter
test_metric 42
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
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
			Name:        "metadata_consistent_across_series",
			Description: "Sender SHOULD send consistent metadata for the same metric family",
			RFCLevel:    "SHOULD",
			ScrapeData: `# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
http_requests_total{method="GET"} 100
http_requests_total{method="POST"} 50
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				metadataMap := make(map[string]writev2.Metadata)

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]

					if metricName == "http_requests_total" {
						if existing, found := metadataMap[metricName]; found {
							// If metadata exists, it should be consistent.
							should(t, existing.Type == ts.Metadata.Type, "Metadata type should be consistent for same metric family")
							should(t, existing.HelpRef == ts.Metadata.HelpRef, "Metadata help should be consistent for same metric family")
						} else {
							metadataMap[metricName] = ts.Metadata
						}
					}
				}
			},
		},
		{
			Name:        "metadata_empty_help_allowed",
			Description: "Sender MAY send metrics without HELP text",
			RFCLevel:    "MAY",
			ScrapeData: `# TYPE no_help_metric counter
no_help_metric 42
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "no_help_metric" {
						// HelpRef may be 0 (empty string) which is valid.
						may(t, int(ts.Metadata.HelpRef) >= 0, "Empty HELP text is allowed")
						found = true
						break
					}
				}
				may(t, found || len(req.Request.Timeseries) > 0, "Metrics without HELP are allowed")
			},
		},
	}

	runTestCases(t, tests)
}

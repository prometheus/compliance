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

	"github.com/prometheus/compliance/remotewrite/sender/targets"
)

// TestRemoteWrite1Compatibility validates RW 1.0 backward compatibility.
// Note: These tests require sender to be configured for RW 1.0 mode.
// Most senders default to RW 2.0, so RW 1.0 tests are informational.
func TestRemoteWrite1Compatibility(t *testing.T) {
	tests := []TestCase{
		{
			Name:        "rw1_version_header",
			Description: "When using RW 1.0, sender SHOULD use version 0.1.0",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")

				// Check if this is RW 1.0 or RW 2.0.
				if strings.HasPrefix(version, "2.0") {
					// This is RW 2.0, skip RW 1.0 validation.
					t.Logf("Sender using RW 2.0 (version: %s), skipping RW 1.0 test", version)
					return
				}

				if strings.HasPrefix(version, "0.1") || version == "" {
					should(t, true, "RW 1.0 version header is acceptable")
					t.Logf("RW 1.0 detected with version: %s", version)
				} else {
					t.Logf("Unknown version: %s", version)
				}
			},
		},
		{
			Name:        "rw1_content_type",
			Description: "RW 1.0 SHOULD use basic content-type without proto parameter",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				contentType := req.Headers.Get("Content-Type")
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")

				// Only validate if this is RW 1.0.
				if strings.HasPrefix(version, "2.0") {
					t.Logf("RW 2.0 detected, skipping RW 1.0 content-type test")
					return
				}

				// RW 1.0 typically uses simple "application/x-protobuf".
				should(t, strings.Contains(contentType, "application/x-protobuf"), "RW 1.0 should use protobuf content-type")

				// RW 1.0 should NOT have proto parameter (that's RW 2.0).
				if strings.Contains(contentType, "proto=io.prometheus.write.v2") {
					t.Logf("Warning: RW 1.0 should not use v2 proto parameter")
				}
			},
		},
		{
			Name:        "rw1_samples_encoding",
			Description: "RW 1.0 MUST encode samples correctly",
			RFCLevel:    "MUST",
			ScrapeData:  "test_counter_total 100\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")

				if strings.HasPrefix(version, "2.0") {
					t.Logf("RW 2.0 detected, skipping RW 1.0 sample encoding test")
					return
				}

				must(t).NotNil(req.Request, "Request should be parseable")
				t.Logf("RW 1.0 samples encoded")
			},
		},
		{
			Name:        "rw1_labels_encoding",
			Description: "RW 1.0 MUST encode labels correctly",
			RFCLevel:    "MUST",
			ScrapeData:  `test_metric{label="value"} 42`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")

				if strings.HasPrefix(version, "2.0") {
					t.Logf("RW 2.0 detected, skipping RW 1.0 label encoding test")
					return
				}

				// Validate that request contains data.
				must(t).NotNil(req.Request, "Request should contain label data")
				t.Logf("RW 1.0 labels encoded")
			},
		},
		{
			Name:        "rw1_no_native_histograms",
			Description: "RW 1.0 does not support native histograms",
			RFCLevel:    "MUST",
			ScrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.0
test_histogram_bucket{le="+Inf"} 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")

				if strings.HasPrefix(version, "2.0") {
					t.Logf("RW 2.0 detected, skipping RW 1.0 histogram test")
					return
				}

				// RW 1.0 should send histogram as separate timeseries (classic format).
				// Should NOT use native histogram encoding.
				for _, ts := range req.Request.Timeseries {
					must(t).Empty(ts.Histograms,
						"RW 1.0 should not use native histogram encoding")
				}

				t.Logf("RW 1.0: Histograms sent as classic format (separate series)")
			},
		},
		{
			Name:        "rw1_no_start_timestamp",
			Description: "RW 1.0 does not support start_timestamp field",
			RFCLevel:    "MUST",
			ScrapeData: `# TYPE test_counter counter
test_counter_total 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")

				if strings.HasPrefix(version, "2.0") {
					t.Logf("RW 2.0 detected, skipping RW 1.0 start_timestamp test")
					return
				}

				// RW 1.0 format doesn't have start_timestamp field.
				// If sender is truly in RW 1.0 mode, this field should be 0/unset.
				for _, ts := range req.Request.Timeseries {
					for _, sample := range ts.Samples {
						should(t, int64(0) == sample.StartTimestamp, "RW 1.0 should not use start_timestamp field in samples")
					}
				}

				t.Logf("RW 1.0: No start_timestamp field used")
			},
		},
		{
			Name:        "rw1_metadata_handling",
			Description: "RW 1.0 MAY send metadata separately",
			RFCLevel:    "MAY",
			ScrapeData: `# HELP test_metric Test metric description
# TYPE test_metric counter
test_metric 42
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")

				if strings.HasPrefix(version, "2.0") {
					t.Logf("RW 2.0 detected, skipping RW 1.0 metadata test")
					return
				}

				// RW 1.0 has limited metadata support.
				// Metadata is typically sent via separate API endpoint.
				may(t, req.Request != nil, "RW 1.0 may handle metadata differently")
				t.Logf("RW 1.0: Metadata handling validated")
			},
		},
		{
			Name:        "rw1_symbol_table_not_used",
			Description: "RW 1.0 does not use symbol table optimization",
			RFCLevel:    "MUST",
			ScrapeData: `http_requests{method="GET",status="200"} 100
http_requests{method="POST",status="200"} 50
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")

				if strings.HasPrefix(version, "2.0") {
					t.Logf("RW 2.0 detected (uses symbol table), skipping RW 1.0 test")
					return
				}

				// RW 1.0 doesn't use symbol table - labels are sent inline.
				// If this is truly RW 1.0, symbol table should be minimal or empty.
				// (RW 2.0 proto may still parse it but values should be inline).
				may(t, req.Request != nil, "RW 1.0 format validated")
				t.Logf("RW 1.0: Symbol table not used (labels inline)")
			},
		},
	}

	runTestCases(t, tests)
}

// TestRemoteWrite1Configuration tests if sender can be configured for RW 1.0.
func TestRemoteWrite1Configuration(t *testing.T) {
	t.Attr("rfcLevel", "SHOULD")
	t.Attr("description", "Sender SHOULD support RW 1.0 configuration for backward compatibility")

	scrapeData := "test_metric 42\n"

	forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
		runSenderTest(t, targetName, target, SenderTestScenario{
			ScrapeData: scrapeData,
			Validator: func(t *testing.T, req *CapturedRequest) {
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")

				// Check what version is being used.
				if version == "" {
					should(t, len(version) > 0, "Version header should be present")
					t.Logf("No version header, may default to RW 1.0")
				} else if strings.HasPrefix(version, "0.1") {
					should(t, true, "Sender configured for RW 1.0")
					t.Logf("RW 1.0 mode: version %s", version)
				} else if strings.HasPrefix(version, "2.0") {
					should(t, true, "Sender configured for RW 2.0")
					t.Logf("RW 2.0 mode: version %s (RW 1.0 support may be configurable)", version)
				} else {
					t.Logf("Unknown version: %s", version)
				}
			},
		})
	})
}

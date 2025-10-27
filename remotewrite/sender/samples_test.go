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
	"math"
	"testing"
	"time"
)

// TestSampleEncoding validates that senders correctly encode float samples.
func TestSampleEncoding(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		validator   func(*testing.T, *CapturedRequest)
	}{
		{
			name:        "float_value_encoding",
			description: "Sender MUST correctly encode regular float values",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 123.45\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				must(t).NotEmpty(req.Request.Timeseries, "Request must contain timeseries")

				// Find the test_metric timeseries
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_metric" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
						must(t).Equal(123.45, ts.Samples[0].Value,
							"Sample value must be correctly encoded")
						found = true
						break
					}
				}
				must(t).True(found, "test_metric timeseries must be present")
			},
		},
		{
			name:        "integer_value_encoding",
			description: "Sender MUST correctly encode integer values as floats",
			rfcLevel:    "MUST",
			scrapeData:  "test_counter_total 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_counter_total" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
						must(t).Equal(42.0, ts.Samples[0].Value,
							"Integer value must be encoded as float")
						found = true
						break
					}
				}
				must(t).True(found, "test_counter_total timeseries must be present")
			},
		},
		{
			name:        "zero_value_encoding",
			description: "Sender MUST correctly encode zero values",
			rfcLevel:    "MUST",
			scrapeData:  "test_gauge 0\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_gauge" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
						must(t).Equal(0.0, ts.Samples[0].Value,
							"Zero value must be correctly encoded")
						found = true
						break
					}
				}
				must(t).True(found, "test_gauge timeseries must be present")
			},
		},
		{
			name:        "negative_value_encoding",
			description: "Sender MUST correctly encode negative values",
			rfcLevel:    "MUST",
			scrapeData:  "temperature_celsius -15.5\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "temperature_celsius" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
						must(t).Equal(-15.5, ts.Samples[0].Value,
							"Negative value must be correctly encoded")
						found = true
						break
					}
				}
				must(t).True(found, "temperature_celsius timeseries must be present")
			},
		},
		{
			name:        "positive_infinity_encoding",
			description: "Sender MUST correctly encode +Inf values",
			rfcLevel:    "MUST",
			scrapeData:  "test_gauge +Inf\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_gauge" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
						must(t).True(math.IsInf(ts.Samples[0].Value, 1),
							"Positive infinity must be correctly encoded")
						found = true
						break
					}
				}
				must(t).True(found, "test_gauge timeseries must be present")
			},
		},
		{
			name:        "negative_infinity_encoding",
			description: "Sender MUST correctly encode -Inf values",
			rfcLevel:    "MUST",
			scrapeData:  "test_gauge -Inf\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_gauge" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
						must(t).True(math.IsInf(ts.Samples[0].Value, -1),
							"Negative infinity must be correctly encoded")
						found = true
						break
					}
				}
				must(t).True(found, "test_gauge timeseries must be present")
			},
		},
		{
			name:        "nan_encoding",
			description: "Sender MUST correctly encode NaN values",
			rfcLevel:    "MUST",
			scrapeData:  "test_gauge NaN\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_gauge" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
						must(t).True(math.IsNaN(ts.Samples[0].Value),
							"NaN must be correctly encoded")
						found = true
						break
					}
				}
				must(t).True(found, "test_gauge timeseries must be present")
			},
		},
		{
			name:        "timestamp_milliseconds_format",
			description: "Sender MUST encode timestamps as milliseconds since Unix epoch",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_metric" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")

						timestamp := ts.Samples[0].Timestamp
						// Timestamp should be reasonable (not in microseconds, nanoseconds, or seconds)
						// Current time in milliseconds is around 1.7e12
						must(t).Greater(timestamp, int64(1e12),
							"Timestamp should be in milliseconds, not seconds")
						must(t).Less(timestamp, int64(1e16),
							"Timestamp should be in milliseconds, not nanoseconds")

						found = true
						break
					}
				}
				must(t).True(found, "test_metric timeseries must be present")
			},
		},
		{
			name:        "timestamp_recent",
			description: "Sender SHOULD send timestamps close to current time for fresh scrapes",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_metric" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")

						timestamp := ts.Samples[0].Timestamp
						now := time.Now().UnixMilli()

						// Timestamp should be within reasonable range of now (±5 minutes)
						diff := now - timestamp
						if diff < 0 {
							diff = -diff
						}
						should(t).Less(diff, int64(5*60*1000),
							"Timestamp should be recent (within 5 minutes), diff: %dms", diff)

						found = true
						break
					}
				}
				must(t).True(found, "test_metric timeseries must be present")
			},
		},
		{
			name:        "multiple_samples_same_series",
			description: "Sender MAY send multiple samples for the same series in different requests",
			rfcLevel:    "MAY",
			scrapeData:  "test_counter_total 100\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				// This is informational - just validate the structure is correct
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_counter_total" {
						may(t).NotEmpty(ts.Samples, "Timeseries may contain samples")
						found = true
						break
					}
				}
				may(t).True(found, "test_counter_total may be present")
			},
		},
		{
			name:        "large_float_values",
			description: "Sender MUST handle very large float values",
			rfcLevel:    "MUST",
			scrapeData:  "test_large 1.7976931348623157e+308\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_large" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
						must(t).Greater(ts.Samples[0].Value, 1e307,
							"Large float value must be correctly encoded")
						found = true
						break
					}
				}
				must(t).True(found, "test_large timeseries must be present")
			},
		},
		{
			name:        "small_float_values",
			description: "Sender MUST handle very small float values",
			rfcLevel:    "MUST",
			scrapeData:  "test_small 2.2250738585072014e-308\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_small" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
						must(t).Less(ts.Samples[0].Value, 1e-307,
							"Small float value must be correctly encoded")
						must(t).Greater(ts.Samples[0].Value, 0.0,
							"Small float value must be positive")
						found = true
						break
					}
				}
				must(t).True(found, "test_small timeseries must be present")
			},
		},
		{
			name:        "scientific_notation",
			description: "Sender MUST handle values in scientific notation",
			rfcLevel:    "MUST",
			scrapeData:  "test_scientific 1.23e-4\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_scientific" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
						must(t).InDelta(0.000123, ts.Samples[0].Value, 0.0000001,
							"Scientific notation value must be correctly parsed and encoded")
						found = true
						break
					}
				}
				must(t).True(found, "test_scientific timeseries must be present")
			},
		},
		{
			name:        "precision_preservation",
			description: "Sender SHOULD preserve float precision",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_precision 0.123456789012345\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_precision" {
						must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
						// float64 should preserve this precision
						should(t).InDelta(0.123456789012345, ts.Samples[0].Value, 1e-15,
							"Float precision should be preserved")
						found = true
						break
					}
				}
				must(t).True(found, "test_precision timeseries must be present")
			},
		},
		{
			name:        "job_instance_labels_present",
			description: "Sender SHOULD include job and instance labels in samples",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				var found bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_metric" {
						should(t).NotEmpty(labels["job"],
							"Sample should include 'job' label")
						should(t).NotEmpty(labels["instance"],
							"Sample should include 'instance' label")
						found = true
						break
					}
				}
				must(t).True(found, "test_metric timeseries must be present")
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

// TestSampleOrdering validates timestamp ordering in samples.
func TestSampleOrdering(t *testing.T) {
	t.Attr("rfcLevel", "MUST")
	t.Attr("description", "Sender MUST send samples with older timestamps before newer ones within a series")

	// This test would require multiple scrapes over time to validate ordering
	// For now, we validate that within a single request, if multiple samples exist,
	// they are properly ordered
	scrapeData := `# Multiple metrics
metric_a 1
metric_b 2
metric_c 3
`

	forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
		runSenderTest(t, targetName, target, SenderTestScenario{
			ScrapeData: scrapeData,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// Verify that all samples in the request have valid timestamps
				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 1 {
						// If multiple samples in one timeseries, they must be ordered
						for i := 1; i < len(ts.Samples); i++ {
							must(t).LessOrEqual(ts.Samples[i-1].Timestamp, ts.Samples[i].Timestamp,
								"Samples within a timeseries must be ordered by timestamp")
						}
					}
				}
			},
		})
	})
}

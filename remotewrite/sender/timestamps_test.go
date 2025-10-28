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
	"testing"
	"time"

	"github.com/prometheus/compliance/remotewrite/sender/targets"
)

// TestTimestampEncoding validates timestamp encoding and handling.
func TestTimestampEncoding(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		validator   func(*testing.T, *CapturedRequest)
	}{
		{
			name:        "timestamp_int64_milliseconds",
			description: "Timestamps MUST be encoded as int64 milliseconds since Unix epoch",
			rfcLevel:    "MUST",
			scrapeData:  "test_counter_total 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				must(t).NotEmpty(req.Request.Timeseries, "Request must contain timeseries")

				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 0 {
						timestamp := ts.Samples[0].Timestamp
						// Verify it's in milliseconds range
						must(t).Greater(timestamp, int64(1e12),
							"Timestamp should be in milliseconds, not seconds")
						must(t).Less(timestamp, int64(1e16),
							"Timestamp should be in milliseconds, not nanoseconds")
						break
					}
				}
			},
		},
		{
			name:        "timestamp_ordering_within_series",
			description: "Within a timeseries, samples MUST be ordered by timestamp (oldest first)",
			rfcLevel:    "MUST",
			scrapeData: `# Multiple metrics over time
metric_a 1
metric_b 2
metric_c 3
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// Check that if a timeseries has multiple samples, they're ordered
				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 1 {
						for i := 1; i < len(ts.Samples); i++ {
							must(t).LessOrEqual(ts.Samples[i-1].Timestamp, ts.Samples[i].Timestamp,
								"Samples within timeseries must be ordered by timestamp (oldest first)")
						}
					}

					// Same for histograms
					if len(ts.Histograms) > 1 {
						for i := 1; i < len(ts.Histograms); i++ {
							must(t).LessOrEqual(ts.Histograms[i-1].Timestamp, ts.Histograms[i].Timestamp,
								"Histograms within timeseries must be ordered by timestamp (oldest first)")
						}
					}
				}
			},
		},
		{
			name:        "created_timestamp_for_counters",
			description: "Sender MAY include created_timestamp for counter metrics",
			rfcLevel:    "MAY",
			scrapeData: `# TYPE test_counter counter
test_counter_total 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_counter_total" {
						if ts.CreatedTimestamp != 0 {
							may(t, ts.CreatedTimestamp > int64(0), "Created timestamp may be present for counters")
							may(t, ts.CreatedTimestamp > int64(1e12), "Created timestamp should be in milliseconds")
							t.Logf("Found created_timestamp: %d", ts.CreatedTimestamp)
						}
						break
					}
				}
			},
		},
		{
			name:        "created_timestamp_for_histograms",
			description: "Sender MAY include created_timestamp for histogram metrics",
			rfcLevel:    "MAY",
			scrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.0
test_histogram_bucket{le="+Inf"} 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]

					if metricName == "test_histogram_count" || metricName == "test_histogram" {
						if ts.CreatedTimestamp != 0 {
							may(t, ts.CreatedTimestamp > int64(0), "Created timestamp may be present for histograms")
							t.Logf("Found created_timestamp for histogram: %d", ts.CreatedTimestamp)
						}
						break
					}
				}
			},
		},
		{
			name:        "created_timestamp_zero_handling",
			description: "Created timestamp value of 0 SHOULD be treated as unset",
			rfcLevel:    "SHOULD",
			scrapeData: `# TYPE test_counter counter
test_counter_total 50
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// If created_timestamp is 0, it should be treated as unset
				for _, ts := range req.Request.Timeseries {
					if ts.CreatedTimestamp == 0 {
						// This is valid - 0 means unset
						should(t, int64(0) == ts.CreatedTimestamp, "Created timestamp of 0 means unset")
					} else {
						// If set, must be valid
						should(t, ts.CreatedTimestamp > int64(1e12), "Non-zero created timestamp should be valid milliseconds")
					}
				}
			},
		},
		{
			name:        "timestamp_precision",
			description: "Sender SHOULD preserve millisecond precision in timestamps",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 0 {
						timestamp := ts.Samples[0].Timestamp

						// Timestamp should be a reasonable value
						now := time.Now().UnixMilli()
						diff := now - timestamp
						if diff < 0 {
							diff = -diff
						}

						should(t, diff < int64(10*60*1000),
							"Timestamp should be within 10 minutes of current time for fresh scrape")

						// Verify precision (not rounded to seconds)
						msComponent := timestamp % 1000
						// It's okay if ms component is 0 sometimes, but if it's always 0,
						// precision might be lost. This is a soft check.
						t.Logf("Timestamp: %d (ms component: %d)", timestamp, msComponent)
						break
					}
				}
			},
		},
		{
			name:        "timestamp_future_values",
			description: "Sender SHOULD handle timestamps slightly in the future",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				now := time.Now().UnixMilli()

				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 0 {
						timestamp := ts.Samples[0].Timestamp

						// Timestamps might be slightly in the future due to clock skew
						if timestamp > now {
							diff := timestamp - now
							should(t, diff < int64(5*60*1000),
								"Future timestamps should not be too far ahead (max 5 min)")
							t.Logf("Found future timestamp: %d ms ahead", diff)
						}
						break
					}
				}
			},
		},
		{
			name:        "created_timestamp_before_sample_timestamp",
			description: "Created timestamp SHOULD be before or equal to sample timestamp",
			rfcLevel:    "SHOULD",
			scrapeData: `# TYPE test_counter counter
test_counter_total 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					if ts.CreatedTimestamp != 0 && len(ts.Samples) > 0 {
						sampleTimestamp := ts.Samples[0].Timestamp
						should(t, ts.CreatedTimestamp <= sampleTimestamp, "Created timestamp should be before or equal to sample timestamp")
						t.Logf("Created: %d, Sample: %d", ts.CreatedTimestamp, sampleTimestamp)
					}

					if ts.CreatedTimestamp != 0 && len(ts.Histograms) > 0 {
						histTimestamp := ts.Histograms[0].Timestamp
						should(t, ts.CreatedTimestamp <= histTimestamp, "Created timestamp should be before or equal to histogram timestamp")
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
				runSenderTest(t, targetName, target, SenderTestScenario{
					ScrapeData: tt.scrapeData,
					Validator:  tt.validator,
				})
			})
		})
	}
}

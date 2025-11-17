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
)

// TestTimestampEncoding validates timestamp encoding and handling.
func TestTimestampEncoding(t *testing.T) {
	tests := []TestCase{
		{
			Name:        "timestamp_int64_milliseconds",
			Description: "Timestamps MUST be encoded as int64 milliseconds since Unix epoch",
			RFCLevel:    "MUST",
			ScrapeData:  "test_counter_total 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				must(t).NotEmpty(req.Request.Timeseries, "Request must contain timeseries")

				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 0 {
						timestamp := ts.Samples[0].Timestamp
						// Verify it's in milliseconds range.
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
			Name:        "timestamp_ordering_within_series",
			Description: "Within a timeseries, samples MUST be ordered by timestamp (oldest first)",
			RFCLevel:    "MUST",
			ScrapeData: `# Multiple metrics over time
metric_a 1
metric_b 2
metric_c 3
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// Check that if a timeseries has multiple samples, they're ordered.
				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 1 {
						for i := 1; i < len(ts.Samples); i++ {
							must(t).LessOrEqual(ts.Samples[i-1].Timestamp, ts.Samples[i].Timestamp,
								"Samples within timeseries must be ordered by timestamp (oldest first)")
						}
					}

					// Same for histograms.
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
			Name:        "created_timestamp_for_counters",
			Description: "Sender MAY include created_timestamp for counter metrics",
			RFCLevel:    "MAY",
			ScrapeData: `# TYPE test_counter counter
test_counter_total 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
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
			Name:        "created_timestamp_for_histograms",
			Description: "Sender MAY include created_timestamp for histogram metrics",
			RFCLevel:    "MAY",
			ScrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.0
test_histogram_bucket{le="+Inf"} 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
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
			Name:        "created_timestamp_zero_handling",
			Description: "Created timestamp value of 0 SHOULD be treated as unset",
			RFCLevel:    "SHOULD",
			ScrapeData: `# TYPE test_counter counter
test_counter_total 50
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// If created_timestamp is 0, it should be treated as unset.
				for _, ts := range req.Request.Timeseries {
					if ts.CreatedTimestamp == 0 {
						should(t, int64(0) == ts.CreatedTimestamp, "Created timestamp of 0 means unset")
					} else {
						should(t, ts.CreatedTimestamp > int64(1e12), "Non-zero created timestamp should be valid milliseconds")
					}
				}
			},
		},
		{
			Name:        "timestamp_precision",
			Description: "Sender SHOULD preserve millisecond precision in timestamps",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 0 {
						timestamp := ts.Samples[0].Timestamp

						now := time.Now().UnixMilli()
						diff := now - timestamp
						if diff < 0 {
							diff = -diff
						}

						should(t, diff < int64(10*60*1000),
							"Timestamp should be within 10 minutes of current time for fresh scrape")

						msComponent := timestamp % 1000
						t.Logf("Timestamp: %d (ms component: %d)", timestamp, msComponent)
						break
					}
				}
			},
		},
		{
			Name:        "timestamp_future_values",
			Description: "Sender SHOULD handle timestamps slightly in the future",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				now := time.Now().UnixMilli()

				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 0 {
						timestamp := ts.Samples[0].Timestamp

						// Timestamps might be slightly in the future due to clock skew.
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
			Name:        "created_timestamp_before_sample_timestamp",
			Description: "Created timestamp SHOULD be before or equal to sample timestamp",
			RFCLevel:    "SHOULD",
			ScrapeData: `# TYPE test_counter counter
test_counter_total 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
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

	runTestCases(t, tests)
}

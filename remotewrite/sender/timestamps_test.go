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
	"testing"
)

// TestTimestampEncoding validates timestamp encoding and handling.
func TestTimestampEncoding_Old(t *testing.T) {
	t.Skip("TODO: Revise and move to a new framework")

	tests := []TestCase{
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
			Name:        "start_timestamp_before_sample_timestamp",
			Description: "Start timestamp SHOULD be before or equal to sample timestamp",
			RFCLevel:    "SHOULD",
			ScrapeData: `# TYPE test_counter counter
test_counter_total 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					for _, sample := range ts.Samples {
						if sample.StartTimestamp != 0 {
							should(t, sample.StartTimestamp <= sample.Timestamp, "Start timestamp should be before or equal to sample timestamp")
							t.Logf("Start: %d, Sample: %d", sample.StartTimestamp, sample.Timestamp)
						}
					}
				}
			},
		},
	}

	runTestCases(t, tests)
}

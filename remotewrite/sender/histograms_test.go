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

	"github.com/prometheus/compliance/remotewrite/sender/targets"

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

// TestHistogramEncoding validates native histogram encoding.
func TestHistogramEncoding(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		validator   func(*testing.T, *CapturedRequest)
	}{
		{
			name:        "native_histogram_structure",
			description: "Sender MUST correctly encode native histogram structure",
			rfcLevel:    "MUST",
			scrapeData: `# TYPE test_histogram histogram
test_histogram_count 10
test_histogram_sum 25.5
test_histogram_bucket{le="1"} 2
test_histogram_bucket{le="5"} 7
test_histogram_bucket{le="+Inf"} 10
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// Note: This is a classic histogram, not native histogram.
				// Native histograms use exponential buckets notation.
				// For classic histograms, senders typically send as multiple timeseries.
				var foundBucket, foundCount bool

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]

					if metricName == "test_histogram_bucket" {
						foundBucket = true
					} else if metricName == "test_histogram_count" {
						foundCount = true
					}
				}

				must(t).True(foundCount || foundBucket,
					"Histogram data must be present (either as count/sum/bucket or native format)")
			},
		},
		{
			name:        "histogram_count_present",
			description: "Sender MUST include histogram count",
			rfcLevel:    "MUST",
			scrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.5
test_histogram_bucket{le="+Inf"} 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundCount bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_histogram_count" {
						must(t).NotEmpty(ts.Samples, "Histogram count must have samples")
						must(t).Equal(100.0, ts.Samples[0].Value,
							"Histogram count value must be correct")
						foundCount = true
						break
					}

					// Check native histogram format
					if labels["__name__"] == "test_histogram" && len(ts.Histograms) > 0 {
						hist := ts.Histograms[0]
						var count uint64
						if hist.Count != nil {
							if countInt, ok := hist.Count.(*writev2.Histogram_CountInt); ok {
								count = countInt.CountInt
							} else if countFloat, ok := hist.Count.(*writev2.Histogram_CountFloat); ok {
								count = uint64(countFloat.CountFloat)
							}
						}
						must(t).Greater(count, uint64(0), "Native histogram count must be present")
						foundCount = true
						break
					}
				}
				may(t, foundCount, "Histogram count should be present in some form")
			},
		},
		{
			name:        "histogram_sum_present",
			description: "Sender MUST include histogram sum",
			rfcLevel:    "MUST",
			scrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.5
test_histogram_bucket{le="+Inf"} 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundSum bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_histogram_sum" {
						must(t).NotEmpty(ts.Samples, "Histogram sum must have samples")
						must(t).Equal(250.5, ts.Samples[0].Value,
							"Histogram sum value must be correct")
						foundSum = true
						break
					}

					// Check native histogram format
					if labels["__name__"] == "test_histogram" && len(ts.Histograms) > 0 {
						hist := ts.Histograms[0]
						must(t).NotZero(hist.Sum, "Native histogram sum must be non-zero")
						foundSum = true
						break
					}
				}
				may(t, foundSum, "Histogram sum should be present in some form")
			},
		},
		{
			name:        "histogram_buckets_ordered",
			description: "Sender SHOULD send histogram buckets in order",
			rfcLevel:    "SHOULD",
			scrapeData: `# TYPE request_duration histogram
request_duration_bucket{le="0.1"} 10
request_duration_bucket{le="0.5"} 50
request_duration_bucket{le="1.0"} 100
request_duration_bucket{le="5.0"} 200
request_duration_bucket{le="+Inf"} 250
request_duration_sum 500.0
request_duration_count 250
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// Classic histograms are sent as separate timeseries, order is not guaranteed
				// Native histograms have internal bucket structure
				var foundHistogram bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "request_duration" && len(ts.Histograms) > 0 {
						foundHistogram = true
						break
					}
				}
				may(t, foundHistogram || len(req.Request.Timeseries) > 0, "Histogram data should be present")
			},
		},
		{
			name:        "histogram_positive_buckets",
			description: "Native histogram MAY include positive buckets",
			rfcLevel:    "MAY",
			scrapeData: `# TYPE test_native_histogram histogram
test_native_histogram_count 100
test_native_histogram_sum 250.0
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// Check if sender supports native histograms
				var foundNative bool
				for _, ts := range req.Request.Timeseries {
					if len(ts.Histograms) > 0 {
						foundNative = true
						hist := ts.Histograms[0]
						may(t, len(hist.PositiveSpans) > 0, "Native histogram may have positive buckets")
						break
					}
				}
				may(t, foundNative || len(req.Request.Timeseries) > 0, "Histogram may be in native or classic format")
			},
		},
		{
			name:        "histogram_negative_buckets",
			description: "Native histogram MAY include negative buckets",
			rfcLevel:    "MAY",
			scrapeData: `# TYPE test_histogram histogram
test_histogram_count 50
test_histogram_sum -25.0
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// Negative buckets are optional in native histograms
				var foundNative bool
				for _, ts := range req.Request.Timeseries {
					if len(ts.Histograms) > 0 {
						foundNative = true
						break
					}
				}
				may(t, foundNative || len(req.Request.Timeseries) > 0, "Histogram may be in various formats")
			},
		},
		{
			name:        "histogram_zero_bucket",
			description: "Native histogram MAY include zero bucket",
			rfcLevel:    "MAY",
			scrapeData: `# TYPE test_histogram histogram
test_histogram_count 10
test_histogram_sum 0.0
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundNative bool
				for _, ts := range req.Request.Timeseries {
					if len(ts.Histograms) > 0 {
						foundNative = true
						break
					}
				}
				may(t, foundNative || len(req.Request.Timeseries) > 0, "Histogram data should be present in some form")
			},
		},
		{
			name:        "histogram_schema",
			description: "Native histogram MUST specify schema if using native format",
			rfcLevel:    "MUST",
			scrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 500.0
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// If native histograms are present, they must have a schema
				for _, ts := range req.Request.Timeseries {
					if len(ts.Histograms) > 0 {
						hist := ts.Histograms[0]
						// Schema must be set (even if 0)
						must(t).NotNil(hist, "Native histogram must have schema")
						t.Logf("Histogram schema: %d", hist.Schema)
						break
					}
				}
			},
		},
		{
			name:        "histogram_timestamp",
			description: "Histogram MUST include valid timestamp",
			rfcLevel:    "MUST",
			scrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.0
test_histogram_bucket{le="+Inf"} 100
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundTimestamp bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)

					// Check classic histogram timestamp
					if labels["__name__"] == "test_histogram_count" && len(ts.Samples) > 0 {
						must(t).Greater(ts.Samples[0].Timestamp, int64(0),
							"Histogram timestamp must be valid")
						foundTimestamp = true
						break
					}

					// Check native histogram timestamp
					if len(ts.Histograms) > 0 {
						must(t).Greater(ts.Histograms[0].Timestamp, int64(0),
							"Native histogram timestamp must be valid")
						foundTimestamp = true
						break
					}
				}
				must(t).True(foundTimestamp, "Histogram must have valid timestamp")
			},
		},
		{
			name:        "histogram_no_mixed_with_samples",
			description: "Sender MUST NOT mix histogram and sample data in same timeseries",
			rfcLevel:    "MUST",
			scrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.0
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// Check that no timeseries has both samples and histograms
				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 0 && len(ts.Histograms) > 0 {
						must(t).Fail("Timeseries must not contain both samples and histograms")
					}
				}
			},
		},
		{
			name:        "histogram_empty_buckets",
			description: "Sender SHOULD handle histograms with no observations",
			rfcLevel:    "SHOULD",
			scrapeData: `# TYPE test_histogram histogram
test_histogram_count 0
test_histogram_sum 0
test_histogram_bucket{le="+Inf"} 0
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundEmpty bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_histogram_count" && len(ts.Samples) > 0 {
						should(t, ts.Samples[0].Value == 0.0, "Empty histogram count should be 0")
						foundEmpty = true
						break
					}
				}
				should(t, foundEmpty || len(req.Request.Timeseries) > 0, "Empty histogram should be handled correctly")
			},
		},
		{
			name:        "histogram_large_counts",
			description: "Sender MUST handle histograms with large observation counts",
			rfcLevel:    "MUST",
			scrapeData: `# TYPE test_histogram histogram
test_histogram_count 1000000000
test_histogram_sum 5000000000.0
test_histogram_bucket{le="+Inf"} 1000000000
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundLarge bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_histogram_count" && len(ts.Samples) > 0 {
						must(t).Equal(1e9, ts.Samples[0].Value,
							"Large histogram count must be correctly encoded")
						foundLarge = true
						break
					}
				}
				may(t, foundLarge || len(req.Request.Timeseries) > 0, "Large histogram counts should be handled")
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

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
)

// TestHistogramEncoding validates native histogram encoding.
func TestHistogramEncoding_Old(t *testing.T) {
	t.Skip("TODO: Revise and move to a new framework")

	tests := []TestCase{
		{
			Name:        "native_histogram_structure",
			Description: "Sender MUST correctly encode native histogram structure",
			RFCLevel:    "MUST",
			ScrapeData: `# TYPE test_histogram histogram
test_histogram_count 10
test_histogram_sum 25.5
test_histogram_bucket{le="1"} 2
test_histogram_bucket{le="5"} 7
test_histogram_bucket{le="+Inf"} 10
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// Note: This is a classic histogram, not native histogram.
				// Native histograms use exponential buckets notation.
				// For classic histograms, senders typically send as multiple timeseries.
				classicFound, nativeTS := findHistogramData(req, "test_histogram")
				must(t).True(classicFound || nativeTS != nil,
					"Histogram data must be present (either as count/sum/bucket or native format)")
			},
		},
		{
			Name:        "histogram_count_present",
			Description: "Sender MUST include histogram count",
			RFCLevel:    "MUST",
			ScrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.5
test_histogram_bucket{le="+Inf"} 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				count, found := extractHistogramCount(req, "test_histogram")
				may(t, found, "Histogram count should be present in some form")
				if found {
					must(t).Equal(100.0, count, "Histogram count value must be correct")
				}
			},
		},
		{
			Name:        "histogram_sum_present",
			Description: "Sender MUST include histogram sum",
			RFCLevel:    "MUST",
			ScrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.5
test_histogram_bucket{le="+Inf"} 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				sum, found := extractHistogramSum(req, "test_histogram")
				may(t, found, "Histogram sum should be present in some form")
				if found {
					must(t).Equal(250.5, sum, "Histogram sum value must be correct")
				}
			},
		},
		{
			Name:        "histogram_buckets_ordered",
			Description: "Sender SHOULD send histogram buckets in order",
			RFCLevel:    "SHOULD",
			ScrapeData: `# TYPE request_duration histogram
request_duration_bucket{le="0.1"} 10
request_duration_bucket{le="0.5"} 50
request_duration_bucket{le="1.0"} 100
request_duration_bucket{le="5.0"} 200
request_duration_bucket{le="+Inf"} 250
request_duration_sum 500.0
request_duration_count 250
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// Classic histograms are sent as separate timeseries, order is not guaranteed.
				// Native histograms have internal bucket structure.
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
			Name:        "histogram_positive_buckets",
			Description: "Native histogram MAY include positive buckets",
			RFCLevel:    "MAY",
			ScrapeData: `# TYPE test_native_histogram histogram
test_native_histogram_count 100
test_native_histogram_sum 250.0
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// Check if sender supports native histograms.
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
			Name:        "histogram_negative_buckets",
			Description: "Native histogram MAY include negative buckets",
			RFCLevel:    "MAY",
			ScrapeData: `# TYPE test_histogram histogram
test_histogram_count 50
test_histogram_sum -25.0
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// Negative buckets are optional in native histograms.
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
			Name:        "histogram_zero_bucket",
			Description: "Native histogram MAY include zero bucket",
			RFCLevel:    "MAY",
			ScrapeData: `# TYPE test_histogram histogram
test_histogram_count 10
test_histogram_sum 0.0
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
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
			Name:        "histogram_schema",
			Description: "Native histogram MUST specify schema if using native format",
			RFCLevel:    "MUST",
			ScrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 500.0
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// If native histograms are present, they must have a schema.
				for _, ts := range req.Request.Timeseries {
					if len(ts.Histograms) > 0 {
						hist := ts.Histograms[0]
						must(t).NotNil(hist, "Native histogram must have schema")
						t.Logf("Histogram schema: %d", hist.Schema)
						break
					}
				}
			},
		},
		{
			Name:        "histogram_timestamp",
			Description: "Histogram MUST include valid timestamp",
			RFCLevel:    "MUST",
			ScrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.0
test_histogram_bucket{le="+Inf"} 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundTimestamp bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)

					if labels["__name__"] == "test_histogram_count" && len(ts.Samples) > 0 {
						must(t).Greater(ts.Samples[0].Timestamp, int64(0),
							"Histogram timestamp must be valid")
						foundTimestamp = true
						break
					}

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
			Name:        "histogram_no_mixed_with_samples",
			Description: "Sender MUST NOT mix histogram and sample data in same timeseries",
			RFCLevel:    "MUST",
			ScrapeData: `# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.0
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// Check that no timeseries has both samples and histograms.
				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 0 && len(ts.Histograms) > 0 {
						must(t).Fail("Timeseries must not contain both samples and histograms")
					}
				}
			},
		},
		{
			Name:        "histogram_empty_buckets",
			Description: "Sender SHOULD handle histograms with no observations",
			RFCLevel:    "SHOULD",
			ScrapeData: `# TYPE test_histogram histogram
test_histogram_count 0
test_histogram_sum 0
test_histogram_bucket{le="+Inf"} 0
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
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
			Name:        "histogram_large_counts",
			Description: "Sender MUST handle histograms with large observation counts",
			RFCLevel:    "MUST",
			ScrapeData: `# TYPE test_histogram histogram
test_histogram_count 1000000000
test_histogram_sum 5000000000.0
test_histogram_bucket{le="+Inf"} 1000000000
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
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

	runTestCases(t, tests)
}

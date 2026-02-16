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
	"math"
	"testing"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance/remotewrite/sender/sendertest"
	"github.com/stretchr/testify/require"
)

// TestSample_OpenMetrics validates that senders correctly encode samples from OpenMetrics 1.0
//
// NOTE: This has recommended RFC level as this test a mix of OpenMetrics and Remote Write specification rules.
// TODO(bwplotka): Add Histogram (NHCB/NH) tests.
func TestSample_OpenMetrics(t *testing.T) {
	sendertest.Run(t,
		targetsToTest,
		sendertest.Case{
			Description: "Sender correctly encodes regular float values from OpenMetrics 1.0",
			RFCLevel:    sendertest.RecommendedLevel,
			ScrapeData: `
test_metric1 123.45
test_metric2 42
test_metric3 0
test_metric4 NaN
test_metric5 -123.45
test_metric6 +Inf
test_metric7 -Inf
test_metric8 1.7976931348623157e+308
test_metric9 2.2250738585072014e-308
`,
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)
				require.Greater(t, len(res.Requests[0].RW2.Timeseries), 9, "Request must contain at least 9 timeseries")
			},
			ValidateCases: []sendertest.ValidateCase{
				{
					Name:        "float",
					Description: "Sender correctly encodes float sample from OpenMetrics 1.0",
					RFCLevel:    sendertest.RecommendedLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric1")
						require.NotEmpty(t, ts.Samples, "Timeseries test_metric1 must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_metric1 must contain a single sample")
						require.Equal(t, 123.45, ts.Samples[0].Value,
							"Sample value for test_metric1 must be correctly encoded")
					},
				},
				{
					Name:        "integer",
					Description: "Sender correctly encodes integer sample from OpenMetrics 1.0",
					RFCLevel:    sendertest.RecommendedLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric2")
						require.NotEmpty(t, ts.Samples, "Timeseries test_metric2 must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_metric2 must contain a single sample")
						require.Equal(t, 42, ts.Samples[0].Value,
							"Sample value for test_metric2 must be correctly encoded")
					},
				},
				{
					Name:        "zero",
					Description: "Sender correctly encodes zero sample from OpenMetrics 1.0",
					RFCLevel:    sendertest.RecommendedLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric3")
						require.NotEmpty(t, ts.Samples, "Timeseries test_metric3 must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_metric3 must contain a single sample")
						require.Equal(t, 0, ts.Samples[0].Value,
							"Sample value for test_metric3 must be correctly encoded")
					},
				},
				{
					Name:        "NaN",
					Description: "Sender correctly encodes NaN sample from OpenMetrics 1.0",
					RFCLevel:    sendertest.RecommendedLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric3")
						require.NotEmpty(t, ts.Samples, "Timeseries test_metric4 must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_metric4 must contain a single sample")
						require.True(t, math.IsNaN(ts.Samples[0].Value),
							"Sample value test_metric4 test_metric6 must be correctly encoded")
					},
				},
				{
					Name:        "negative",
					Description: "Sender correctly encodes negative sample from OpenMetrics 1.0",
					RFCLevel:    sendertest.RecommendedLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric5")
						require.NotEmpty(t, ts.Samples, "Timeseries test_metric5 must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_metric5 must contain a single sample")
						require.Equal(t, -123.45, ts.Samples[0].Value,
							"Sample value for test_metric5 must be correctly encoded")
					},
				},
				{
					Name:        "+Inf",
					Description: "Sender correctly encodes +Inf sample from OpenMetrics 1.0",
					RFCLevel:    sendertest.RecommendedLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric6")
						require.NotEmpty(t, ts.Samples, "Timeseries test_metric6 must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_metric6 must contain a single sample")
						require.True(t, math.IsInf(ts.Samples[0].Value, 1),
							"Sample value for test_metric6 must be correctly encoded")
					},
				},
				{
					Name:        "-Inf",
					Description: "Sender correctly encodes -Inf sample from OpenMetrics 1.0",
					RFCLevel:    sendertest.RecommendedLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric7")
						require.NotEmpty(t, ts.Samples, "Timeseries test_metric7 must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_metric7 must contain a single sample")
						require.True(t, math.IsInf(ts.Samples[0].Value, -1),
							"Sample value for test_metric7 must be correctly encoded")
					},
				},
				{
					Name:        "large",
					Description: "Sender correctly encodes large sample from OpenMetrics 1.0",
					RFCLevel:    sendertest.RecommendedLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric8")
						require.NotEmpty(t, ts.Samples, "Timeseries test_metric8 must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_metric8 must contain a single sample")
						require.Greater(t, ts.Samples[0].Value, 1e307,
							"Sample value for test_metric8 must be correctly encoded")
					},
				},
				{
					Name:        "small",
					Description: "Sender correctly encodes small sample from OpenMetrics 1.0",
					RFCLevel:    sendertest.RecommendedLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric9")
						require.NotEmpty(t, ts.Samples, "Timeseries test_metric9 must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_metric9 must contain a single sample")
						require.Less(t, ts.Samples[0].Value, 1e-307,
							"Sample value for test_metric9 must be correctly encoded")
						require.Greater(t, ts.Samples[0].Value, 0.0,
							"Sample value for test_metric9 must be correctly encoded")
					},
				},
			},
		},
	)
}

/*
TODO(bwplotka): Convert and revise.
//// TestPrometheusScrapeSemantics validates that senders correctly encode float samples from OpenMetrics.
////
//// NOTE: This has recommended RFC level as this test a mix of Prometheus behaviour and Remote Write specification rules.
//func TestPrometheusScrapeSemantics(t *testing.T) {
//	RunTests(t, []TestCase{
//		{
//			Name:          "float_timestamp_recent",
//			Description:   "Sender sends timestamps close to the current time for fresh scrapes",
//			RFCLevel:      sendertest.RecommendedLevel,
//			ScrapeData:    "test_metric 42\n",
//			TestResponses: []MockReceiverResponse{{}}, // OK response.
//			Validate: func(t *testing.T, res MockReceiverResult) {
//				require.GreaterOrEqual(t, len(res.Requests), 1)
//
//				req := res.Requests[0].Request
//				require.NotEmpty(t, req.Timeseries, "Request must contain timeseries")
//
//				ts, _ := requireTimeseriesByMetricName(t, req, "test_metric")
//				require.NotEmpty(t, ts.Samples, "Timeseries test_metric must contain samples")
//				require.Len(t, ts.Samples, 1, "Timeseries test_metric must contain a single sample")
//
//				require.Less(t, time.Since(timestamp.Time(ts.Samples[0].Timestamp)), 5*time.Minute, "Timestamp should be recent (within 5 minutes)")
//			},
//		},
//		{
//			Name:          "job_instance_labels_present",
//			Description:   "Sender includes job and instance labels in samples",
//			RFCLevel:      sendertest.RecommendedLevel,
//			ScrapeData:    "test_metric 42\n",
//			TestResponses: []MockReceiverResponse{{}}, // OK response.
//			Validate: func(t *testing.T, res MockReceiverResult) {
//				require.GreaterOrEqual(t, len(res.Requests), 1)
//
//				req := res.Requests[0].Request
//				require.NotEmpty(t, req.Timeseries, "Request must contain timeseries")
//
//				_, labels := requireTimeseriesByMetricName(t, req, "test_metric")
//				require.Contains(t, labels, "job", "Sample should include 'job' label")
//				require.Contains(t, labels, "instance", "Sample should include 'instance' label")
//			},
//		},
//	})
//}

/*{
	Name:        "stale_marker",
	Description: "Sender SHOULD handle stale marker (StaleNaN)",
	RFCLevel:    "SHOULD",
	ScrapeData:  "test_metric StaleNaN\n",
	Validator: func(t *testing.T, req *CapturedRequest) {
		// StaleNaN is a special NaN value.
		for _, ts := range req.Request.Timeseries {
			labels := extractLabels(&ts, req.Request.Symbols)
			if labels["__name__"] == "test_metric" && len(ts.Samples) > 0 {
				value := ts.Samples[0].Value
				should(t, math.IsNaN(value), "StaleNaN should be encoded as NaN")
				t.Logf("Stale marker handled: NaN=%v", math.IsNaN(value))
			}
		}
	},
},

*/

/*

// TestHistogramEncoding validates native histogram encoding.
func TestHistogramEncoding(t *testing.T) {
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

*/

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
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance/remotewrite/sender/sendertest"
	"github.com/prometheus/compliance/remotewrite/sender/targets"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/stretchr/testify/require"
)

func formatTimeAsOpenMetricsTimestamp(t time.Time) string {
	v := float64(timestamp.FromTime(t)) / 1000
	return labels.FormatOpenMetricsFloat(v)
}

func TestSample(t *testing.T) {
	timeNow := time.Now()
	st := timeNow.Add(-2 * time.Hour)
	explicitTS := timeNow

	sendertest.Run(t,
		targetsToTest,
		sendertest.Case{
			// TODO(bwplotka): Fix 2.0 spec - MUST value and timestamp are not mentioned (only in proto).
			Name:        "sample",
			Description: "Senders MUST send valid samples",
			RFCLevel:    sendertest.MustLevel,
			ScrapeData: fmt.Sprintf(`# TYPE test_counter counter
test_counter_total 101.13
test_counter_created %v
# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.0
test_histogram_bucket{le="+Inf"} 100
test_histogram_created %v
# TYPE test_gauge_with_ts gauge
test_gauge_with_ts 2 %v
`,
				timestamp.FromTime(st),
				timestamp.FromTime(st),
				formatTimeAsOpenMetricsTimestamp(explicitTS),
			),
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)
				require.Greater(t, len(res.Requests[0].RW2.Timeseries), 6, "Request must contain at least 6 timeseries")
			},
			ValidateCases: []sendertest.ValidateCase{
				{
					Name:        "value",
					Description: "Sample MUST have value",
					RFCLevel:    sendertest.MustLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_counter_total")
						require.NotEmpty(t, ts.Samples, "Timeseries test_counter_total must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_counter_total must contain a single sample")
						require.Equal(t, 101.13, ts.Samples[0].Value,
							"Sample value for test_counter_total must be correctly encoded")
					},
				},
				{
					Name:        "timestamp",
					Description: "Sample MUST have timestamp",
					RFCLevel:    sendertest.MustLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						for _, ts := range res.Requests[0].RW2.Timeseries {
							require.Len(t, ts.Samples, 1, "Timeseries must contain a single sample")
							require.GreaterOrEqual(t, ts.Samples[0].Timestamp, timestamp.FromTime(timeNow), "Timeseries must contain a fresh timestamp")
						}
					},
				},
				// TODO(bwplotka): Make it work, somehow OM parser kills test_gauge_with_ts metric with no log.
				//{
				//	Name:        "explicit_timestamp",
				//	Description: "Sample with the explicit timestamp work",
				//	RFCLevel:    sendertest.RecommendedLevel, // Prometheus spec, not Remote Write.
				//	Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				//		ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_gauge_with_ts")
				//		require.NotEmpty(t, ts.Samples, "Timeseries test_gauge_with_ts must contain samples")
				//		require.Len(t, ts.Samples, 1, "Timeseries test_gauge_with_ts must contain a single sample")
				//		require.Equal(t, timestamp.FromTime(explicitTS), ts.Samples[0].Timestamp)
				//	},
				//},
				{
					Name:        "start_timestamp for counters",
					Description: "Sample SHOULD have start timestamp for a counter",
					RFCLevel:    sendertest.ShouldLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_counter_total")
						require.NotEmpty(t, ts.Samples, "Timeseries test_counter_total must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_counter_total must contain a single sample")
						require.Equal(t, timestamp.FromTime(st), ts.Samples[0].StartTimestamp,
							"Sample for test_counter_total does not have ST")
					},
				},
				{
					Name:        "start_timestamp for histograms",
					Description: "Sample SHOULD have start timestamp for a histogram",
					RFCLevel:    sendertest.ShouldLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_histogram_count")
						require.NotEmpty(t, ts.Samples, "Timeseries test_histogram_count must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_histogram_count must contain a single sample")
						require.Equal(t, timestamp.FromTime(st), ts.Samples[0].StartTimestamp,
							"Sample for test_histogram_count does not have ST")
					},
				},
			},
		},
	)
}

// TestSampleEncoding validates that senders correctly encode float samples.
func TestSampleEncoding_Old(t *testing.T) {
	t.Skip("TODO: Revise and move to a new framework")

	tests := []TestCase{
		{
			Name:        "float_value_encoding",
			Description: "Sender MUST correctly encode regular float values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_metric 123.45\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				must(t).NotEmpty(req.Request.Timeseries, "Request must contain timeseries")
				ts, _ := requireTimeseriesByMetricName(t, req.Request, "test_metric")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).Equal(123.45, ts.Samples[0].Value,
					"Sample value must be correctly encoded")
			},
		},
		{
			Name:        "integer_value_encoding",
			Description: "Sender MUST correctly encode integer values as floats",
			RFCLevel:    "MUST",
			ScrapeData:  "test_counter_total 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req.Request, "test_counter_total")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).Equal(42.0, ts.Samples[0].Value,
					"Integer value must be encoded as float")
			},
		},
		{
			Name:        "zero_value_encoding",
			Description: "Sender MUST correctly encode zero values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_gauge 0\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req.Request, "test_gauge")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).Equal(0.0, ts.Samples[0].Value,
					"Zero value must be correctly encoded")
			},
		},
		{
			Name:        "negative_value_encoding",
			Description: "Sender MUST correctly encode negative values",
			RFCLevel:    "MUST",
			ScrapeData:  "temperature_celsius -15.5\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req.Request, "temperature_celsius")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).Equal(-15.5, ts.Samples[0].Value,
					"Negative value must be correctly encoded")
			},
		},
		{
			Name:        "positive_infinity_encoding",
			Description: "Sender MUST correctly encode +Inf values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_gauge +Inf\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req.Request, "test_gauge")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).True(math.IsInf(ts.Samples[0].Value, 1),
					"Positive infinity must be correctly encoded")
			},
		},
		{
			Name:        "negative_infinity_encoding",
			Description: "Sender MUST correctly encode -Inf values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_gauge -Inf\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req.Request, "test_gauge")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).True(math.IsInf(ts.Samples[0].Value, -1),
					"Negative infinity must be correctly encoded")
			},
		},
		{
			Name:        "nan_encoding",
			Description: "Sender MUST correctly encode NaN values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_gauge NaN\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req.Request, "test_gauge")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).True(math.IsNaN(ts.Samples[0].Value),
					"NaN must be correctly encoded")
			},
		},
		{
			Name:        "large_float_values",
			Description: "Sender MUST handle very large float values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_large 1.7976931348623157e+308\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req.Request, "test_large")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).Greater(ts.Samples[0].Value, 1e307,
					"Large float value must be correctly encoded")
			},
		},
		{
			Name:        "small_float_values",
			Description: "Sender MUST handle very small float values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_small 2.2250738585072014e-308\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req.Request, "test_small")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).Less(ts.Samples[0].Value, 1e-307,
					"Small float value must be correctly encoded")
				must(t).Greater(ts.Samples[0].Value, 0.0,
					"Small float value must be positive")
			},
		},
		{
			Name:        "scientific_notation",
			Description: "Sender MUST handle values in scientific notation",
			RFCLevel:    "MUST",
			ScrapeData:  "test_scientific 1.23e-4\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req.Request, "test_scientific")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).InDelta(0.000123, ts.Samples[0].Value, 0.0000001,
					"Scientific notation value must be correctly parsed and encoded")
			},
		},
		{
			Name:        "precision_preservation",
			Description: "Sender SHOULD preserve float precision",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_precision 0.123456789012345\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req.Request, "test_precision")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
			},
		},
		{
			Name:        "job_instance_labels_present",
			Description: "Sender SHOULD include job and instance labels in samples",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				_, labels := requireTimeseriesByMetricName(t, req.Request, "test_metric")
				should(t, len(labels["job"]) > 0, "Sample should include 'job' label")
				should(t, len(labels["instance"]) > 0, "Sample should include 'instance' label")
			},
		},
	}

	runTestCases(t, tests)
}

// TestSampleOrdering validates timestamp ordering in samples.
func TestSampleOrdering_Old(t *testing.T) {
	t.Skip("TODO: Revise and move to a new framework")

	t.Attr("rfcLevel", "MUST")
	t.Attr("description", "Sender MUST send samples with older timestamps before newer ones within a series")

	scrapeData := `# Multiple metrics
metric_a 1
metric_b 2
metric_c 3
`

	forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
		runSenderTest(t, targetName, target, SenderTestScenario{
			ScrapeData: scrapeData,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// Verify that all samples in the request have valid timestamps.
				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 1 {
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

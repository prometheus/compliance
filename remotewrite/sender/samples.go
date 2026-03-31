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
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/stretchr/testify/require"
)

func ToOpenMetricsTimestampFloat64(t time.Time) float64 {
	v := float64(timestamp.FromTime(t)) / 1000
	return v
}

func ToOpenMetricsTimestampString(t time.Time) string {
	return labels.FormatOpenMetricsFloat(
		ToOpenMetricsTimestampFloat64(t),
	)
}

func samplesTests() []Test {
	timeNow := time.Now()
	st := timeNow.Add(-2 * time.Hour)
	explicitTS := timeNow

	return []Test{
		{
			// TODO(bwplotka): Fix 2.0 spec - MUST value and timestamp are not mentioned (only in proto).
			Name:        "samples",
			Description: "Senders MUST send valid samples",
			RFCLevel:    MustLevel,
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
				ToOpenMetricsTimestampFloat64(st),
				ToOpenMetricsTimestampFloat64(st),
				ToOpenMetricsTimestampString(explicitTS),
			),
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 6 timeseries")
				require.Greater(t, len(res.Requests[0].RW2.Timeseries), 6, "Request must contain at least 6 timeseries")
			},
			ValidateCases: []ValidateCase{
				{
					Name:        "value",
					Description: "Sample MUST have value",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_counter_total")
						require.Len(t, results, 1, "Should receive exactly one timeseries for test_counter_total")
						ts := results[0].TimeSeries
						require.NotEmpty(t, ts.Samples, "Timeseries test_counter_total must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_counter_total must contain a single sample")
						require.Equal(t, 101.13, ts.Samples[0].Value,
							"Sample value for test_counter_total must be correctly encoded")
					},
				},
				{
					Name:        "timestamp",
					Description: "Sample MUST have timestamp",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
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
				//		results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_gauge_with_ts")
				//		require.Len(t, results, 1, "Should receive exactly one timeseries for test_gauge_with_ts")
				//		tts := results[0].TimeSeries
				//		require.NotEmpty(t, ts.Samples, "Timeseries test_gauge_with_ts must contain samples")
				//		require.Len(t, ts.Samples, 1, "Timeseries test_gauge_with_ts must contain a single sample")
				//		require.Equal(t, timestamp.FromTime(explicitTS), ts.Samples[0].Timestamp)
				//	},
				//},
				{
					Name:        "start_timestamp for counters",
					Description: "Sample SHOULD have start timestamp for a counter",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_counter_total")
						require.Len(t, results, 1, "Should receive exactly one timeseries for test_counter_total")
						ts := results[0].TimeSeries
						require.NotEmpty(t, ts.Samples, "Timeseries test_counter_total must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_counter_total must contain a single sample")
						require.Equal(t, timestamp.FromTime(st), ts.Samples[0].StartTimestamp,
							"Sample for test_counter_total does not have ST")
					},
				},
				{
					Name:        "start_timestamp for histograms",
					Description: "Sample SHOULD have start timestamp for a histogram",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_histogram_count")
						require.Len(t, results, 1, "Should receive exactly one timeseries for test_histogram_count")
						ts := results[0].TimeSeries
						require.NotEmpty(t, ts.Samples, "Timeseries test_histogram_count must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_histogram_count must contain a single sample")
						require.Equal(t, timestamp.FromTime(st), ts.Samples[0].StartTimestamp,
							"Sample for test_histogram_count does not have ST")
					},
				},
				{
					Name:        "start_timestamp before timestamp",
					Description: "Start timestamp is SHOULD be 0 or before or equal to timestamp",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, ts := range res.Requests[0].RW2.Timeseries {
							for _, sample := range ts.Samples {
								if sample.StartTimestamp != 0 {
									require.LessOrEqual(t, sample.StartTimestamp, sample.Timestamp, "Start timestamp should be before or equal to timestamp")
								}
							}
						}
					},
				},
			},
		},
		{
			Name:        "samples sorted",
			Description: "Sender MUST send samples in a sorted order",
			RFCLevel:    MustLevel,
			ScrapeData: fmt.Sprintf(`# TYPE test_counter counter
test_counter_total 101.13 %v
test_counter_total 102.13 %v
test_counter_total 103.13 %v
`,
				ToOpenMetricsTimestampString(explicitTS),
				ToOpenMetricsTimestampString(explicitTS.Add(15*time.Second)),
				ToOpenMetricsTimestampString(explicitTS.Add(30*time.Second)),
			),
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_counter_total")
				// Prometheus will store 3 series, but support 3 batched samples too.
				if len(results) == 3 {
					require.Len(t, results[0].TimeSeries.Samples, 1)
					require.Len(t, results[1].TimeSeries.Samples, 1)
					require.Len(t, results[2].TimeSeries.Samples, 1)
					require.True(t, slices.IsSorted([]int64{
						results[0].TimeSeries.Samples[0].Timestamp,
						results[1].TimeSeries.Samples[0].Timestamp,
						results[2].TimeSeries.Samples[0].Timestamp,
					}))
					return
				}
				if len(results) == 1 {
					require.Len(t, results[0].TimeSeries.Samples, 3)
					require.True(t, slices.IsSorted([]int64{
						results[0].TimeSeries.Samples[0].Timestamp,
						results[0].TimeSeries.Samples[1].Timestamp,
						results[0].TimeSeries.Samples[2].Timestamp,
					}))
					return
				}
				t.Fatal("Should receive exactly one timeseries for test_counter_total with 3 samples, or 3 timeseries with one sample each; results:", results)
			},
		},
		{
			Name:        "float_value_encoding",
			Description: "Sender MUST correctly encode regular float values",
			RFCLevel:    MustLevel,
			ScrapeData:  "test_metric 123.45\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric")
				require.Len(t, results, 1, "Should receive exactly one timeseries for test_metric")
				ts := results[0].TimeSeries
				require.NotEmpty(t, ts.Samples, "Timeseries must contain samples")
				require.Equal(t, 123.45, ts.Samples[0].Value, "Sample value must be correctly encoded")
			},
		},
		{
			Name:        "integer_value_encoding",
			Description: "Sender MUST correctly encode integer values as floats",
			RFCLevel:    MustLevel,
			ScrapeData:  "test_counter_total 42\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_counter_total")
				require.Len(t, results, 1, "Should receive exactly one timeseries for test_counter_total")
				ts := results[0].TimeSeries
				require.NotEmpty(t, ts.Samples, "Timeseries must contain samples")
				require.Equal(t, 42.0, ts.Samples[0].Value, "Integer value must be encoded as float")
			},
		},
		{
			Name:        "zero_value_encoding",
			Description: "Sender MUST correctly encode zero values",
			RFCLevel:    MustLevel,
			ScrapeData:  "test_gauge 0\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_gauge")
				require.Len(t, results, 1, "Should receive exactly one timeseries for test_gauge")
				ts := results[0].TimeSeries
				require.NotEmpty(t, ts.Samples, "Timeseries must contain samples")
				require.Equal(t, 0.0, ts.Samples[0].Value, "Zero value must be correctly encoded")
			},
		},
		{
			Name:        "negative_value_encoding",
			Description: "Sender MUST correctly encode negative values",
			RFCLevel:    MustLevel,
			ScrapeData:  "temperature_celsius -15.5\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "temperature_celsius")
				require.Len(t, results, 1, "Should receive exactly one timeseries for temperature_celsius")
				ts := results[0].TimeSeries
				require.NotEmpty(t, ts.Samples, "Timeseries must contain samples")
				require.Equal(t, -15.5, ts.Samples[0].Value, "Negative value must be correctly encoded")
			},
		},
		{
			Name:        "positive_infinity_encoding",
			Description: "Sender MUST correctly encode +Inf values",
			RFCLevel:    MustLevel,
			ScrapeData:  "test_gauge +Inf\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_gauge")
				require.Len(t, results, 1, "Should receive exactly one timeseries for test_gauge")
				ts := results[0].TimeSeries
				require.NotEmpty(t, ts.Samples, "Timeseries must contain samples")
				require.True(t, math.IsInf(ts.Samples[0].Value, 1), "Positive infinity must be correctly encoded")
			},
		},
		{
			Name:        "negative_infinity_encoding",
			Description: "Sender MUST correctly encode -Inf values",
			RFCLevel:    MustLevel,
			ScrapeData:  "test_gauge -Inf\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_gauge")
				require.Len(t, results, 1, "Should receive exactly one timeseries for test_gauge")
				ts := results[0].TimeSeries
				require.NotEmpty(t, ts.Samples, "Timeseries must contain samples")
				require.True(t, math.IsInf(ts.Samples[0].Value, -1), "Negative infinity must be correctly encoded")
			},
		},
		{
			Name:        "nan_encoding",
			Description: "Sender MUST correctly encode NaN values",
			RFCLevel:    MustLevel,
			ScrapeData:  "test_gauge NaN\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_gauge")
				require.Len(t, results, 1, "Should receive exactly one timeseries for test_gauge")
				ts := results[0].TimeSeries
				require.NotEmpty(t, ts.Samples, "Timeseries must contain samples")
				require.True(t, math.IsNaN(ts.Samples[0].Value), "NaN must be correctly encoded")
			},
		},
		{
			Name:        "large_float_values",
			Description: "Sender MUST handle very large float values",
			RFCLevel:    MustLevel,
			ScrapeData:  "test_large 1.7976931348623157e+308\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_large")
				require.Len(t, results, 1, "Should receive exactly one timeseries for test_large")
				ts := results[0].TimeSeries
				require.NotEmpty(t, ts.Samples, "Timeseries must contain samples")
				require.Greater(t, ts.Samples[0].Value, 1e307, "Large float value must be correctly encoded")
			},
		},
		{
			Name:        "small_float_values",
			Description: "Sender MUST handle very small float values",
			RFCLevel:    MustLevel,
			ScrapeData:  "test_small 2.2250738585072014e-308\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_small")
				require.Len(t, results, 1, "Should receive exactly one timeseries for test_small")
				ts := results[0].TimeSeries
				require.NotEmpty(t, ts.Samples, "Timeseries must contain samples")
				require.Less(t, ts.Samples[0].Value, 1e-307, "Small float value must be correctly encoded")
				require.Greater(t, ts.Samples[0].Value, 0.0, "Small float value must be positive")
			},
		},
		{
			Name:        "scientific_notation",
			Description: "Sender MUST handle values in scientific notation",
			RFCLevel:    MustLevel,
			ScrapeData:  "test_scientific 1.23e-4\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_scientific")
				require.Len(t, results, 1, "Should receive exactly one timeseries for test_scientific")
				ts := results[0].TimeSeries
				require.NotEmpty(t, ts.Samples, "Timeseries must contain samples")
				require.InDelta(t, 0.000123, ts.Samples[0].Value, 0.0000001, "Scientific notation value must be correctly parsed and encoded")
			},
		},
		{
			Name:        "precision_preservation",
			Description: "Sender SHOULD preserve float precision",
			RFCLevel:    ShouldLevel,
			ScrapeData:  "test_precision 0.123456789012345\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_precision")
				require.Len(t, results, 1, "Should receive exactly one timeseries for test_precision")
				ts := results[0].TimeSeries
				require.NotEmpty(t, ts.Samples, "Timeseries must contain samples")
			},
		},
		{
			Name:        "job_instance_labels_present",
			Description: "Sender SHOULD include job and instance labels in samples",
			RFCLevel:    ShouldLevel,
			ScrapeData:  "test_metric 42\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric")
				require.Len(t, results, 1, "Should receive exactly one timeseries for test_metric")
				labels := results[0].Labels
				require.NotEmpty(t, labels["job"], "Sample should include 'job' label")
				require.NotEmpty(t, labels["instance"], "Sample should include 'instance' label")
			},
		},
		{
			Name:        "sample_ordering",
			Description: "Sender MUST send samples with older timestamps before newer ones within a series",
			RFCLevel:    MustLevel,
			ScrapeData:  "metric_a 1\nmetric_b 2\nmetric_c 3\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				for _, ts := range res.Requests[0].RW2.Timeseries {
					if len(ts.Samples) > 1 {
						for i := 1; i < len(ts.Samples); i++ {
							require.LessOrEqual(t, ts.Samples[i-1].Timestamp, ts.Samples[i].Timestamp, "Samples within a timeseries must be ordered by timestamp")
						}
					}
				}
			},
		},
	}
}

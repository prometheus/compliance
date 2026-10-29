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
	"cmp"
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/model/value"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
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

func samplesTests(mode remote.WriteMode) []Test {
	timeNow := time.Now()
	st := timeNow.Add(-2 * time.Hour)
	// NOTE: Keep explicitTS in the future so WAL watchers started during parallel subtests do not skip it as
	// historical, while staying below Prometheus's 10m maxAheadTime when adding +15s/+30s offsets.
	explicitTS := timeNow.Add(9 * time.Minute)

	stLevel := compliance.ShouldLevel
	if mode == remote.WriteModeStatelessST {
		stLevel = compliance.MustLevel
	}

	return []Test{
		{
			// TODO(bwplotka): Fix 2.0 spec - MUST value and timestamp are not mentioned (only in proto).
			Name:        "samples",
			Description: "Senders MUST send valid samples",
			RFCLevel:    compliance.MustLevel,
			ScrapeData: fmt.Sprintf(`# TYPE test_counter counter
test_counter_total 101.13
test_counter_created %v
# TYPE test_histogram histogram
test_histogram_bucket{le="0.5"} 50
test_histogram_bucket{le="+Inf"} 100
test_histogram_sum 250.0
test_histogram_count 100
test_histogram_created %v
# TYPE test_summary summary
test_summary{quantile="0.5"} 1.5
test_summary_sum 150.0
test_summary_count 100
test_summary_created %v
# TYPE test_gauge_with_ts gauge
test_gauge_with_ts 2 %v
`,
				ToOpenMetricsTimestampFloat64(st),
				ToOpenMetricsTimestampFloat64(st),
				ToOpenMetricsTimestampFloat64(st),
				ToOpenMetricsTimestampString(explicitTS),
			),
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				var totalSeries int
				for _, req := range res.Requests {
					require.NotNil(t, req.RW2)
					totalSeries += len(req.RW2.Timeseries)
				}
				require.GreaterOrEqual(t, totalSeries, 6, "Requests must contain at least 6 timeseries")
			},
			ValidateCases: []compliance.ValidateCase[ReceiverResult]{
				{
					Name:        "value",
					Description: "Sample MUST have value",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_counter_total")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_counter_total")
						ts := results[0].TimeSeries
						require.NotEmpty(t, ts.Samples, "Timeseries test_counter_total must contain samples")
						require.Equal(t, 101.13, ts.Samples[0].Value,
							"Sample value for test_counter_total must be correctly encoded")
					},
				},
				{
					Name:        "timestamp",
					Description: "Sample MUST have timestamp",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							for _, ts := range req.RW2.Timeseries {
								if len(ts.Histograms) > 0 {
									continue
								}
								require.NotEmpty(t, ts.Samples, "Timeseries must contain samples")
								for _, sample := range ts.Samples {
									require.GreaterOrEqual(t, sample.Timestamp, timestamp.FromTime(timeNow), "Timeseries must contain a fresh timestamp")
								}
							}
						}
					},
				},
				{
					Name:        "start_timestamp for counters",
					Description: "Sample SHOULD have start timestamp for a counter (MUST in stateless-st mode)",
					RFCLevel:    stLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_counter_total")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_counter_total")
						ts := results[0].TimeSeries
						require.NotEmpty(t, ts.Samples, "Timeseries test_counter_total must contain samples")
						require.Equal(t, timestamp.FromTime(st), ts.Samples[0].StartTimestamp,
							"Sample for test_counter_total does not have ST")
					},
				},
				{
					Name:        "start_timestamp for histograms",
					Description: "Sample/Histogram SHOULD have start timestamp for a histogram (MUST in stateless-st mode)",
					RFCLevel:    stLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						if mode != remote.WriteModeNone {
							hist := findHistogramDataAcrossRequests(res.Requests, "test_histogram")
							require.NotNil(t, hist, "Histogram for test_histogram must be present")
							require.Equal(t, timestamp.FromTime(st), hist.StartTimestamp,
								"Histogram for test_histogram does not have ST")
							return
						}
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_histogram_count")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_histogram_count")
						ts := results[0].TimeSeries
						require.NotEmpty(t, ts.Samples, "Timeseries test_histogram_count must contain samples")
						require.Equal(t, timestamp.FromTime(st), ts.Samples[0].StartTimestamp,
							"Sample for test_histogram_count does not have ST")
					},
				},
				{
					Name:        "start_timestamp for summaries",
					Description: "Summary _sum and _count samples SHOULD have start timestamp (MUST in stateless-st mode)",
					RFCLevel:    stLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, metricName := range []string{"test_summary_sum", "test_summary_count"} {
							results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, metricName)
							require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for %s", metricName)
							ts := results[0].TimeSeries
							require.NotEmpty(t, ts.Samples, "Timeseries %s must contain samples", metricName)
							require.Equal(t, timestamp.FromTime(st), ts.Samples[0].StartTimestamp,
								"Sample for %s does not have ST", metricName)
						}
					},
				},
				{
					Name:        "start_timestamp before timestamp",
					Description: "Start timestamp is SHOULD be 0 or before or equal to timestamp",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							for _, ts := range req.RW2.Timeseries {
								for _, sample := range ts.Samples {
									if sample.StartTimestamp != 0 {
										require.LessOrEqual(t, sample.StartTimestamp, sample.Timestamp, "Start timestamp should be before or equal to timestamp")
									}
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
			RFCLevel:    compliance.MustLevel,
			ScrapeData: fmt.Sprintf(`# TYPE test_counter counter
test_counter_total 101.13 %v
test_counter_total 102.13 %v
test_counter_total 103.13 %v
test_counter_created %v
`,
				ToOpenMetricsTimestampString(explicitTS),
				ToOpenMetricsTimestampString(explicitTS.Add(15*time.Second)),
				ToOpenMetricsTimestampString(explicitTS.Add(30*time.Second)),
				ToOpenMetricsTimestampFloat64(st),
			),
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
				results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_counter_total")
				ts := flattenTimeseriesResult(t, results)
				require.GreaterOrEqual(t, len(ts.TimeSeries.Samples), 3)
				require.True(t, slices.IsSortedFunc(ts.TimeSeries.Samples, func(a, b writev2.Sample) int {
					return cmp.Compare(a.T(), b.T())
				}))
			},
		},
		{
			Name:        "encoding_cases",
			Description: "Sender MUST correctly encode various value types and special cases",
			RFCLevel:    compliance.MustLevel,
			ScrapeData: `# TYPE test_float gauge
test_float 123.45
# TYPE test_integer gauge
test_integer 42
# TYPE test_zero gauge
test_zero 0
# TYPE test_negative gauge
test_negative -15.5
# TYPE test_pos_inf gauge
test_pos_inf +Inf
# TYPE test_neg_inf gauge
test_neg_inf -Inf
# TYPE test_nan gauge
test_nan NaN
# TYPE test_large gauge
test_large 1.7976931348623157e+308
# TYPE test_small gauge
test_small 2.2250738585072014e-308
# TYPE test_scientific gauge
test_scientific 1.23e-4
# TYPE test_precision gauge
test_precision 0.123456789012345
`,
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")
			},
			ValidateCases: []compliance.ValidateCase[ReceiverResult]{
				{
					Name:        "float_value_encoding",
					Description: "Sender MUST correctly encode regular float values",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_float")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_float")
						require.Equal(t, 123.45, results[0].TimeSeries.Samples[0].Value, "Sample value must be correctly encoded")
					},
				},
				{
					Name:        "integer_value_encoding",
					Description: "Sender MUST correctly encode integer values as floats",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_integer")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_integer")
						require.Equal(t, 42.0, results[0].TimeSeries.Samples[0].Value, "Integer value must be encoded as float")
					},
				},
				{
					Name:        "zero_value_encoding",
					Description: "Sender MUST correctly encode zero values",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_zero")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_zero")
						require.Equal(t, 0.0, results[0].TimeSeries.Samples[0].Value, "Zero value must be correctly encoded")
					},
				},
				{
					Name:        "negative_value_encoding",
					Description: "Sender MUST correctly encode negative values",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_negative")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_negative")
						require.Equal(t, -15.5, results[0].TimeSeries.Samples[0].Value, "Negative value must be correctly encoded")
					},
				},
				{
					Name:        "positive_infinity_encoding",
					Description: "Sender MUST correctly encode +Inf values",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_pos_inf")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_pos_inf")
						require.True(t, math.IsInf(results[0].TimeSeries.Samples[0].Value, 1), "Positive infinity must be correctly encoded")
					},
				},
				{
					Name:        "negative_infinity_encoding",
					Description: "Sender MUST correctly encode -Inf values",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_neg_inf")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_neg_inf")
						require.True(t, math.IsInf(results[0].TimeSeries.Samples[0].Value, -1), "Negative infinity must be correctly encoded")
					},
				},
				{
					Name:        "nan_encoding",
					Description: "Sender MUST correctly encode NaN values and not use the reserved StaleNaN bit pattern",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_nan")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_nan")
						require.True(t, math.IsNaN(results[0].TimeSeries.Samples[0].Value), "NaN must be correctly encoded")
						require.False(t, value.IsStaleNaN(results[0].TimeSeries.Samples[0].Value), "Regular NaN must not use the reserved StaleNaN bit pattern")
					},
				},
				{
					Name:        "large_float_values",
					Description: "Sender MUST handle very large float values",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_large")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_large")
						require.Greater(t, results[0].TimeSeries.Samples[0].Value, 1e307, "Large float value must be correctly encoded")
					},
				},
				{
					Name:        "small_float_values",
					Description: "Sender MUST handle very small float values",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_small")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_small")
						require.Less(t, results[0].TimeSeries.Samples[0].Value, 1e-307, "Small float value must be correctly encoded")
						require.Greater(t, results[0].TimeSeries.Samples[0].Value, 0.0, "Small float value must be positive")
					},
				},
				{
					Name:        "scientific_notation",
					Description: "Sender MUST handle values in scientific notation",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_scientific")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_scientific")
						require.InDelta(t, 0.000123, results[0].TimeSeries.Samples[0].Value, 0.0000001, "Scientific notation value must be correctly parsed and encoded")
					},
				},
				{
					Name:        "precision_preservation",
					Description: "Sender SHOULD preserve float precision",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_precision")
						require.GreaterOrEqual(t, len(results), 1, "Should receive at least one timeseries for test_precision")
						require.NotEmpty(t, results[0].TimeSeries.Samples, "Timeseries must contain samples")
						require.Equal(t, 0.123456789012345, results[0].TimeSeries.Samples[0].Value, "Float precision must be preserved")
					},
				},
			},
		},
		{
			Name:        "stale_markers",
			Description: "Stale markers MUST be signalled by the special NaN value 0x7ff0000000000002 when a series disappears",
			RFCLevel:    compliance.MustLevel,
			ScrapeData: `# TYPE disappearing_metric gauge
disappearing_metric{env="prod"} 42
# TYPE persistent_metric gauge
persistent_metric{env="prod"} 1
`,
			AdditionalScrapeData: []string{
				`# TYPE persistent_metric gauge
persistent_metric{env="prod"} 2
`,
			},
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)
				for i := range res.Requests {
					require.NotNil(t, res.Requests[i].RW2)
				}
			},
			ValidateCases: []compliance.ValidateCase[ReceiverResult]{
				{
					Name:        "stale_nan_sent_on_disappearance",
					Description: "Disappearing series MUST have a sample encoded with StaleNaN (0x7ff0000000000002).",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						// Senders may either batch both scrapes into a single request or send them in separate requests.
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "disappearing_metric")
						ts := flattenTimeseriesResult(t, results).TimeSeries
						require.GreaterOrEqual(t, len(ts.Samples), 2, "Expected initial sample and staleness marker for disappearing_metric")
						require.Equal(t, 42.0, ts.Samples[0].Value)
						var foundStale bool
						for _, s := range ts.Samples[1:] {
							if value.IsStaleNaN(s.Value) {
								foundStale = true
								require.Greater(t, s.Timestamp, ts.Samples[0].Timestamp)
							}
						}
						require.True(t, foundStale, "Expected StaleNaN (0x7ff0000000000002) sample after series disappeared")
					},
				},
			},
		},
	}
}

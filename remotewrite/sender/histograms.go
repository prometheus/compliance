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
	"time"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance"
	dto "github.com/prometheus/prometheus/prompb/io/prometheus/client"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

func histogramsTests(mode remote.WriteMode) []Test {
	const (
		basicHistMetric      = "request_duration_seconds"
		negBucketsHistMetric = "temperature_delta_celsius"
		zeroBucketHistMetric = "latency_with_zeros_seconds"
		floatHistMetric      = "weighted_observations"
		emptyHistMetric      = "empty_histogram"
		largeCountHistMetric = "high_volume_requests"
	)

	now := time.Now().Truncate(time.Millisecond)
	nowMs := now.UnixMilli()
	startTime := now.Add(-time.Minute)
	startMs := startTime.UnixMilli()
	explicitTS := now.Add(9 * time.Minute)

	stLevel := compliance.ShouldLevel
	if mode == remote.WriteModeStatelessST {
		stLevel = compliance.MustLevel
	}

	nativeHistogramFamilies := []dto.MetricFamily{
		{
			Name: basicHistMetric,
			Help: "Request duration native histogram.",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{{
				Histogram: withCreatedTimestamp(&dto.Histogram{
					SampleCount:   21,
					SampleSum:     45.5,
					Schema:        0,
					ZeroThreshold: 0.001,
					ZeroCount:     2,
					PositiveSpan: []dto.BucketSpan{
						{Offset: 0, Length: 3},
					},
					PositiveDelta: []int64{5, 3, -2},
				}, startTime),
			}},
		},
		{
			Name: negBucketsHistMetric,
			Help: "Histogram with negative and positive buckets.",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{{
				Histogram: withCreatedTimestamp(&dto.Histogram{
					SampleCount:   18,
					SampleSum:     -3.2,
					Schema:        1,
					ZeroThreshold: 0.001,
					ZeroCount:     1,
					NegativeSpan: []dto.BucketSpan{
						{Offset: 0, Length: 2},
					},
					NegativeDelta: []int64{4, 2},
					PositiveSpan: []dto.BucketSpan{
						{Offset: 1, Length: 2},
					},
					PositiveDelta: []int64{3, 1},
				}, startTime),
			}},
		},
		{
			Name: zeroBucketHistMetric,
			Help: "Histogram with zero bucket observations.",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{{
				Histogram: withCreatedTimestamp(&dto.Histogram{
					SampleCount:   15,
					SampleSum:     12.3,
					Schema:        2,
					ZeroThreshold: 1e-6,
					ZeroCount:     5,
					PositiveSpan: []dto.BucketSpan{
						{Offset: 0, Length: 2},
					},
					PositiveDelta: []int64{6, -2},
				}, startTime),
			}},
		},
		{
			Name: floatHistMetric,
			Help: "Float native histogram.",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{{
				Histogram: withCreatedTimestamp(&dto.Histogram{
					SampleCountFloat: 12.5,
					SampleSum:        34.8,
					Schema:           0,
					ZeroThreshold:    0.001,
					ZeroCountFloat:   1.5,
					PositiveSpan: []dto.BucketSpan{
						{Offset: 0, Length: 2},
					},
					PositiveCount: []float64{5.5, 5.5},
				}, startTime),
			}},
		},
		{
			Name: emptyHistMetric,
			Help: "Empty native histogram.",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{{
				Histogram: withCreatedTimestamp(&dto.Histogram{
					SampleCount:   0,
					SampleSum:     0,
					Schema:        0,
					ZeroThreshold: 0,
					ZeroCount:     0,
					// NOTE: Per io.prometheus.client.Histogram proto specification, a native histogram
					// with 0 observations and zero_threshold 0 requires a no-op span (offset 0, length 0)
					// to distinguish it from a classic histogram.
					PositiveSpan: []dto.BucketSpan{
						{Offset: 0, Length: 0},
					},
				}, startTime),
			}},
		},
		{
			Name: largeCountHistMetric,
			Help: "Histogram with large bucket counts.",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{{
				Histogram: withCreatedTimestamp(&dto.Histogram{
					SampleCount:   3000000,
					SampleSum:     1500000.0,
					Schema:        0,
					ZeroThreshold: 0.001,
					ZeroCount:     0,
					PositiveSpan: []dto.BucketSpan{
						{Offset: 0, Length: 2},
					},
					PositiveDelta: []int64{1000000, 1000000},
				}, startTime),
			}},
		},
	}

	return []Test{
		{
			Name:              "native_histograms",
			Description:       "Tests native histogram encoding, bucket spans/deltas, schemas, zero buckets, float histograms, and timestamps.",
			RFCLevel:          compliance.MustLevel,
			Version:           remote.WriteV2MessageType,
			ScrapeContentType: compliance.ProtoContentType,
			ScrapeData:        metricFamiliesToProtobuf(nativeHistogramFamilies),
			Validate: func(t *testing.T, res ReceiverResult) {
				require.NotEmpty(t, res.Requests)
				var totalSeries int
				for _, req := range res.Requests {
					require.NotNil(t, req.RW2)
					totalSeries += len(req.RW2.Timeseries)
				}
				require.NotZero(t, totalSeries)
			},
			ValidateCases: []compliance.ValidateCase[ReceiverResult]{
				{
					Name:        "structure_count_and_sum",
					Description: "Tests that native histograms encode count, sum, and valid schema.",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						hist := findHistogramDataAcrossRequests(res.Requests, basicHistMetric)
						require.NotNil(t, hist, "Native histogram for %s must be present", basicHistMetric)
						require.Equal(t, 21.0, extractHistogramCount(hist))
						require.InDelta(t, 45.5, hist.Sum, 0.01)
						require.GreaterOrEqual(t, hist.Schema, int32(-4))
						require.LessOrEqual(t, hist.Schema, int32(8))
					},
				},
				{
					Name:        "positive_spans_and_deltas",
					Description: "Tests that positive bucket spans and delta-encoded counts are preserved.",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						hist := findHistogramDataAcrossRequests(res.Requests, basicHistMetric)
						require.NotNil(t, hist)
						require.NotEmpty(t, hist.PositiveSpans, "Positive spans must be present")
						require.NotEmpty(t, hist.PositiveDeltas, "Positive deltas must be present")

						totalLength := uint32(0)
						for _, span := range hist.PositiveSpans {
							totalLength += span.Length
						}
						require.Equal(t, int(totalLength), len(hist.PositiveDeltas), "Sum of positive span lengths must match positive deltas length")
					},
				},
				{
					Name:        "negative_spans_and_deltas",
					Description: "Tests that negative bucket spans and delta-encoded counts are preserved alongside positive buckets.",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						hist := findHistogramDataAcrossRequests(res.Requests, negBucketsHistMetric)
						require.NotNil(t, hist)
						require.Equal(t, int32(1), hist.Schema)
						require.NotEmpty(t, hist.NegativeSpans, "Negative spans must be present")
						require.NotEmpty(t, hist.NegativeDeltas, "Negative deltas must be present")
						require.NotEmpty(t, hist.PositiveSpans, "Positive spans must be present")
						require.NotEmpty(t, hist.PositiveDeltas, "Positive deltas must be present")

						negLen := uint32(0)
						for _, span := range hist.NegativeSpans {
							negLen += span.Length
						}
						require.Equal(t, int(negLen), len(hist.NegativeDeltas))
					},
				},
				{
					Name:        "zero_bucket",
					Description: "Tests that zero_threshold and zero_count are preserved.",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						hist := findHistogramDataAcrossRequests(res.Requests, zeroBucketHistMetric)
						require.NotNil(t, hist)
						require.Equal(t, int32(2), hist.Schema)
						require.InDelta(t, 1e-6, hist.ZeroThreshold, 1e-9)
						zc, ok := hist.ZeroCount.(*writev2.Histogram_ZeroCountInt)
						require.True(t, ok, "Expected integer zero count")
						require.Equal(t, uint64(5), zc.ZeroCountInt)
					},
				},
				{
					Name:        "float_histogram",
					Description: "Tests that float histograms encode float count, zero_count_float, and positive_counts.",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						hist := findHistogramDataAcrossRequests(res.Requests, floatHistMetric)
						require.NotNil(t, hist)
						require.InDelta(t, 12.5, extractHistogramCount(hist), 0.01)
						require.InDelta(t, 34.8, hist.Sum, 0.01)
						require.NotEmpty(t, hist.PositiveSpans)
						require.NotEmpty(t, hist.PositiveCounts, "Float histogram must use positive_counts")
					},
				},
				{
					Name:        "timestamp",
					Description: "Tests that native histograms include a valid millisecond timestamp.",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						hist := findHistogramDataAcrossRequests(res.Requests, basicHistMetric)
						require.NotNil(t, hist)
						require.GreaterOrEqual(t, hist.Timestamp, nowMs)
					},
				},
				{
					Name:        "start_timestamp_before_timestamp",
					Description: "Tests that native histogram start_timestamp is before or equal to timestamp (and non-zero in stateless-st mode).",
					RFCLevel:    stLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						hist := findHistogramDataAcrossRequests(res.Requests, basicHistMetric)
						require.NotNil(t, hist)
						if mode == remote.WriteModeStatelessST || hist.StartTimestamp != 0 {
							require.Equal(t, startMs, hist.StartTimestamp)
							require.LessOrEqual(t, hist.StartTimestamp, hist.Timestamp, "Histogram start_timestamp should be before or equal to timestamp")
						}
					},
				},
				{
					Name:        "empty_histogram",
					Description: "Tests that empty native histograms with 0 observations are encoded with count 0 and sum 0.",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						hist := findHistogramDataAcrossRequests(res.Requests, emptyHistMetric)
						require.NotNil(t, hist)
						require.Equal(t, 0.0, extractHistogramCount(hist))
						require.Equal(t, 0.0, hist.Sum)
					},
				},
				{
					Name:        "large_counts",
					Description: "Tests that native histograms preserve large bucket counts without overflow.",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						hist := findHistogramDataAcrossRequests(res.Requests, largeCountHistMetric)
						require.NotNil(t, hist)
						require.Equal(t, 3000000.0, extractHistogramCount(hist))
						require.InDelta(t, 1500000.0, hist.Sum, 0.01)
					},
				},
				{
					Name:        "no_mixed_samples_and_histograms",
					Description: "Tests that a TimeSeries containing native histograms does not mix float samples for the same metric.",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							for _, ts := range req.RW2.Timeseries {
								if len(ts.Histograms) > 0 {
									require.Empty(t, ts.Samples, "TimeSeries with native histograms must not also contain float samples")
								}
							}
						}
					},
				},
			},
		},
		{
			Name:              "native_histograms_ordering",
			Description:       "Tests that multiple native histograms for the same series are sent in increasing timestamp order.",
			RFCLevel:          compliance.MustLevel,
			Version:           remote.WriteV2MessageType,
			ScrapeContentType: compliance.ProtoContentType,
			ScrapeData: metricFamiliesToProtobuf([]dto.MetricFamily{
				{
					Name: basicHistMetric,
					Help: "Request duration native histogram.",
					Type: dto.MetricType_HISTOGRAM,
					Metric: []dto.Metric{
						{
							TimestampMs: explicitTS.UnixMilli(),
							Histogram: withCreatedTimestamp(&dto.Histogram{
								SampleCount:   10,
								SampleSum:     20.0,
								Schema:        0,
								ZeroThreshold: 0.001,
								ZeroCount:     1,
								PositiveSpan: []dto.BucketSpan{
									{Offset: 0, Length: 2},
								},
								PositiveDelta: []int64{5, -1},
							}, startTime),
						},
						{
							TimestampMs: explicitTS.Add(15 * time.Second).UnixMilli(),
							Histogram: withCreatedTimestamp(&dto.Histogram{
								SampleCount:   12,
								SampleSum:     25.0,
								Schema:        0,
								ZeroThreshold: 0.001,
								ZeroCount:     1,
								PositiveSpan: []dto.BucketSpan{
									{Offset: 0, Length: 2},
								},
								PositiveDelta: []int64{6, -1},
							}, startTime),
						},
						{
							TimestampMs: explicitTS.Add(30 * time.Second).UnixMilli(),
							Histogram: withCreatedTimestamp(&dto.Histogram{
								SampleCount:   15,
								SampleSum:     32.0,
								Schema:        0,
								ZeroThreshold: 0.001,
								ZeroCount:     1,
								PositiveSpan: []dto.BucketSpan{
									{Offset: 0, Length: 2},
								},
								PositiveDelta: []int64{8, -2},
							}, startTime),
						},
					},
				},
			}),
			Validate: func(t *testing.T, res ReceiverResult) {
				require.NotEmpty(t, res.Requests)
				for _, req := range res.Requests {
					require.NotNil(t, req.RW2)
				}
			},
			ValidateCases: []compliance.ValidateCase[ReceiverResult]{
				{
					Name:        "histograms_sorted",
					Description: "Tests that histograms within a TimeSeries are ordered by timestamp from oldest to newest.",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, r := range res.Requests {
							for _, ts := range r.RW2.Timeseries {
								for _, h := range ts.Histograms {
									require.Positive(t, h.Timestamp, "Histogram must have positive timestamp")
								}
								for i := 1; i < len(ts.Histograms); i++ {
									require.GreaterOrEqual(t, ts.Histograms[i].Timestamp, ts.Histograms[i-1].Timestamp,
										"Histograms within a TimeSeries must be ordered by timestamp")
								}
							}
						}
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, basicHistMetric)
						ts := flattenTimeseriesResult(t, results)
						require.GreaterOrEqual(t, len(ts.TimeSeries.Histograms), 3)
						for i := 1; i < len(ts.TimeSeries.Histograms); i++ {
							require.GreaterOrEqual(t, ts.TimeSeries.Histograms[i].Timestamp, ts.TimeSeries.Histograms[i-1].Timestamp,
								"Histograms for %s must be ordered by timestamp", basicHistMetric)
						}
					},
				},
			},
		},
		{
			Name:        "classic_histograms",
			Description: "Tests classic histogram encoding (_bucket, _sum, _count series or NHCB conversion).",
			RFCLevel:    compliance.MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `# HELP http_request_duration_seconds Classic HTTP request duration histogram.
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.1"} 10
http_request_duration_seconds_bucket{le="0.5"} 25
http_request_duration_seconds_bucket{le="1.0"} 30
http_request_duration_seconds_bucket{le="+Inf"} 32
http_request_duration_seconds_sum 14.5
http_request_duration_seconds_count 32
http_request_duration_seconds_created 1500.0
`,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.NotEmpty(t, res.Requests)
				var totalSeries int
				for _, req := range res.Requests {
					require.NotNil(t, req.RW2)
					totalSeries += len(req.RW2.Timeseries)
				}
				require.NotZero(t, totalSeries)
			},
			ValidateCases: []compliance.ValidateCase[ReceiverResult]{
				{
					Name:        "classic_histogram_series",
					Description: "Tests that classic histograms are sent as _bucket, _sum, and _count series (or as a Histogram message in stateless modes).",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						if mode != remote.WriteModeNone {
							hist := findHistogramDataAcrossRequests(res.Requests, "http_request_duration_seconds")
							require.NotNil(t, hist, "Stateless mode sender MUST encode classic histogram as a Histogram message")
							require.Equal(t, int32(-53), hist.Schema, "NHCB must use schema -53")
							require.Equal(t, 32.0, extractHistogramCount(hist))
							require.InDelta(t, 14.5, hist.Sum, 0.01)
							require.Equal(t, []float64{0.1, 0.5, 1.0}, hist.CustomValues)
							require.Empty(t, findTimeseriesByMetricNameAcrossRequests(res.Requests, "http_request_duration_seconds_bucket"),
								"Stateless mode MUST NOT send classic _bucket float series")
							require.Empty(t, findTimeseriesByMetricNameAcrossRequests(res.Requests, "http_request_duration_seconds_sum"),
								"Stateless mode MUST NOT send classic _sum float series")
							require.Empty(t, findTimeseriesByMetricNameAcrossRequests(res.Requests, "http_request_duration_seconds_count"),
								"Stateless mode MUST NOT send classic _count float series")
							if mode == remote.WriteModeStatelessST {
								require.Equal(t, int64(1500000), hist.StartTimestamp,
									"Stateless-ST mode MUST set start_timestamp on histogram")
							}
							return
						}

						buckets := findTimeseriesByMetricNameAcrossRequests(res.Requests, "http_request_duration_seconds_bucket")
						if len(buckets) > 0 {
							leValues := make(map[string]float64, 4)
							for _, b := range buckets {
								le := b.Labels["le"]
								require.NotEmpty(t, le, "_bucket series must have le label")
								require.NotEmpty(t, b.TimeSeries.Samples)
								leValues[le] = b.TimeSeries.Samples[0].Value
							}
							require.Len(t, leValues, 4, "Expected 4 distinct le bucket series")
							require.Equal(t, 10.0, leValues["0.1"])
							require.Equal(t, 25.0, leValues["0.5"])
							require.Equal(t, 30.0, leValues["1.0"])
							require.Equal(t, 32.0, leValues["+Inf"])

							sumSeries := flattenTimeseriesResult(t, requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "http_request_duration_seconds_sum"))
							require.NotEmpty(t, sumSeries.TimeSeries.Samples)
							require.InDelta(t, 14.5, sumSeries.TimeSeries.Samples[0].Value, 0.01)

							countSeries := flattenTimeseriesResult(t, requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "http_request_duration_seconds_count"))
							require.NotEmpty(t, countSeries.TimeSeries.Samples)
							require.Equal(t, 32.0, countSeries.TimeSeries.Samples[0].Value)
						} else {
							// Sender may also convert classic histograms to NHCB.
							hist := findHistogramDataAcrossRequests(res.Requests, "http_request_duration_seconds")
							require.NotNil(t, hist, "Must send either _bucket/_sum/_count series or NHCB native histogram")
							require.Equal(t, 32.0, extractHistogramCount(hist))
							require.InDelta(t, 14.5, hist.Sum, 0.01)
						}
					},
				},
			},
		},
	}
}

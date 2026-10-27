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
	dto "github.com/prometheus/prometheus/prompb/io/prometheus/client"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

func histogramsTests(transactional bool) []Test {
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
				Histogram: &dto.Histogram{
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
				},
			}},
		},
		{
			Name: zeroBucketHistMetric,
			Help: "Histogram with zero bucket observations.",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{{
				Histogram: &dto.Histogram{
					SampleCount:   15,
					SampleSum:     12.3,
					Schema:        2,
					ZeroThreshold: 1e-6,
					ZeroCount:     5,
					PositiveSpan: []dto.BucketSpan{
						{Offset: 0, Length: 2},
					},
					PositiveDelta: []int64{6, -2},
				},
			}},
		},
		{
			Name: floatHistMetric,
			Help: "Float native histogram.",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{{
				Histogram: &dto.Histogram{
					SampleCountFloat: 12.5,
					SampleSum:        34.8,
					Schema:           0,
					ZeroThreshold:    0.001,
					ZeroCountFloat:   1.5,
					PositiveSpan: []dto.BucketSpan{
						{Offset: 0, Length: 2},
					},
					PositiveCount: []float64{5.5, 5.5},
				},
			}},
		},
		{
			Name: emptyHistMetric,
			Help: "Empty native histogram.",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{{
				Histogram: &dto.Histogram{
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
				},
			}},
		},
		{
			Name: largeCountHistMetric,
			Help: "Histogram with large bucket counts.",
			Type: dto.MetricType_HISTOGRAM,
			Metric: []dto.Metric{{
				Histogram: &dto.Histogram{
					SampleCount:   3000000,
					SampleSum:     1500000.0,
					Schema:        0,
					ZeroThreshold: 0.001,
					ZeroCount:     0,
					PositiveSpan: []dto.BucketSpan{
						{Offset: 0, Length: 2},
					},
					PositiveDelta: []int64{1000000, 1000000},
				},
			}},
		},
	}

	return []Test{
		{
			Name:              "native_histograms",
			Description:       "Tests native histogram encoding, bucket spans/deltas, schemas, zero buckets, float histograms, and timestamps.",
			RFCLevel:          MustLevel,
			Version:           remote.WriteV2MessageType,
			ScrapeContentType: ProtoContentType,
			ScrapeData:        metricFamiliesToProtobuf(nativeHistogramFamilies),
			Validate: func(t *testing.T, res ReceiverResult) {
				require.Len(t, res.Requests, 1)
				require.NotNil(t, res.Requests[0].RW2)
				require.NotEmpty(t, res.Requests[0].RW2.Timeseries)
			},
			ValidateCases: []ValidateCase{
				{
					Name:        "structure_count_and_sum",
					Description: "Tests that native histograms encode count, sum, and valid schema.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						hist := findHistogramData(req, basicHistMetric)
						require.NotNil(t, hist, "Native histogram for %s must be present", basicHistMetric)
						require.Equal(t, 21.0, extractHistogramCount(hist))
						require.InDelta(t, 45.5, extractHistogramSum(hist), 0.01)
						require.GreaterOrEqual(t, hist.Schema, int32(-4))
						require.LessOrEqual(t, hist.Schema, int32(8))
					},
				},
				{
					Name:        "positive_spans_and_deltas",
					Description: "Tests that positive bucket spans and delta-encoded counts are preserved.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						hist := findHistogramData(req, basicHistMetric)
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
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						hist := findHistogramData(req, negBucketsHistMetric)
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
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						hist := findHistogramData(req, zeroBucketHistMetric)
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
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						hist := findHistogramData(req, floatHistMetric)
						require.NotNil(t, hist)
						require.InDelta(t, 12.5, extractHistogramCount(hist), 0.01)
						require.InDelta(t, 34.8, extractHistogramSum(hist), 0.01)
						require.NotEmpty(t, hist.PositiveSpans)
						require.NotEmpty(t, hist.PositiveCounts, "Float histogram must use positive_counts")
					},
				},
				{
					Name:        "timestamp",
					Description: "Tests that native histograms include a valid millisecond timestamp.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						hist := findHistogramData(req, basicHistMetric)
						require.NotNil(t, hist)
						require.GreaterOrEqual(t, hist.Timestamp, nowMs)
					},
				},
				{
					Name:        "start_timestamp_before_timestamp",
					Description: "Tests that native histogram start_timestamp is 0 or before or equal to timestamp.",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						hist := findHistogramData(req, basicHistMetric)
						require.NotNil(t, hist)
						if hist.StartTimestamp != 0 {
							require.Equal(t, startMs, hist.StartTimestamp)
							require.LessOrEqual(t, hist.StartTimestamp, hist.Timestamp, "Histogram start_timestamp should be before or equal to timestamp")
						}
					},
				},
				{
					Name:        "empty_histogram",
					Description: "Tests that empty native histograms with 0 observations are encoded with count 0 and sum 0.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						hist := findHistogramData(req, emptyHistMetric)
						require.NotNil(t, hist)
						require.Equal(t, 0.0, extractHistogramCount(hist))
						require.Equal(t, 0.0, extractHistogramSum(hist))
					},
				},
				{
					Name:        "large_counts",
					Description: "Tests that native histograms preserve large bucket counts without overflow.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						hist := findHistogramData(req, largeCountHistMetric)
						require.NotNil(t, hist)
						require.Equal(t, 3000000.0, extractHistogramCount(hist))
						require.InDelta(t, 1500000.0, extractHistogramSum(hist), 0.01)
					},
				},
				{
					Name:        "no_mixed_samples_and_histograms",
					Description: "Tests that a TimeSeries containing native histograms does not mix float samples for the same metric.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						for _, ts := range req.Timeseries {
							if len(ts.Histograms) > 0 {
								require.Empty(t, ts.Samples, "TimeSeries with native histograms must not also contain float samples")
							}
						}
					},
				},
			},
		},
		{
			Name:              "native_histograms_ordering",
			Description:       "Tests that multiple native histograms for the same series are sent in increasing timestamp order.",
			RFCLevel:          MustLevel,
			Version:           remote.WriteV2MessageType,
			ScrapeContentType: ProtoContentType,
			ScrapeData: metricFamiliesToProtobuf([]dto.MetricFamily{
				{
					Name: basicHistMetric,
					Help: "Request duration native histogram.",
					Type: dto.MetricType_HISTOGRAM,
					Metric: []dto.Metric{{
						Histogram: &dto.Histogram{
							SampleCount:   10,
							SampleSum:     20.0,
							Schema:        0,
							ZeroThreshold: 0.001,
							ZeroCount:     1,
							PositiveSpan: []dto.BucketSpan{
								{Offset: 0, Length: 2},
							},
							PositiveDelta: []int64{5, -1},
						},
					}},
				},
			}),
			TestResponses: []ReceiverResponse{
				{StatusCode: 500},
				{},
			},
			Validate: func(t *testing.T, res ReceiverResult) {
				require.Len(t, res.Requests, 2)
				require.NotNil(t, res.Requests[1].RW2)
			},
			ValidateCases: []ValidateCase{
				{
					Name:        "histograms_sorted",
					Description: "Tests that histograms within a TimeSeries are ordered by timestamp from oldest to newest.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						var allHistograms []writev2.Histogram
						for _, r := range res.Requests {
							if r.RW2 == nil {
								continue
							}
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
						lastReq := res.Requests[len(res.Requests)-1].RW2
						results := requireTimeseriesByMetricName(t, lastReq, basicHistMetric)
						for _, r := range results {
							allHistograms = append(allHistograms, r.TimeSeries.Histograms...)
						}
						require.NotEmpty(t, allHistograms)
					},
				},
			},
		},
		{
			Name:        "classic_histograms",
			Description: "Tests classic histogram encoding (_bucket, _sum, _count series or NHCB conversion).",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `# HELP http_request_duration_seconds Classic HTTP request duration histogram.
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.1"} 10
http_request_duration_seconds_bucket{le="0.5"} 25
http_request_duration_seconds_bucket{le="1.0"} 30
http_request_duration_seconds_bucket{le="+Inf"} 32
http_request_duration_seconds_sum 14.5
http_request_duration_seconds_count 32
`,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.Len(t, res.Requests, 1)
				require.NotNil(t, res.Requests[0].RW2)
				require.NotEmpty(t, res.Requests[0].RW2.Timeseries)
			},
			ValidateCases: []ValidateCase{
				{
					Name:        "classic_histogram_series",
					Description: "Tests that classic histograms are sent as _bucket, _sum, and _count series (or native histogram with custom buckets in transactional mode).",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						if transactional {
							hist := findHistogramData(req, "http_request_duration_seconds")
							require.NotNil(t, hist, "Transactional sender must convert classic histogram to NHCB")
							require.Equal(t, int32(-53), hist.Schema, "NHCB must use schema -53")
							require.Equal(t, 32.0, extractHistogramCount(hist))
							require.InDelta(t, 14.5, extractHistogramSum(hist), 0.01)
							require.Equal(t, []float64{0.1, 0.5, 1.0}, hist.CustomValues)
							return
						}

						buckets := findTimeseriesByMetricName(req, "http_request_duration_seconds_bucket")
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

							sumSeries := flattenTimeseriesResult(t, requireTimeseriesByMetricName(t, req, "http_request_duration_seconds_sum"))
							require.NotEmpty(t, sumSeries.TimeSeries.Samples)
							require.InDelta(t, 14.5, sumSeries.TimeSeries.Samples[0].Value, 0.01)

							countSeries := flattenTimeseriesResult(t, requireTimeseriesByMetricName(t, req, "http_request_duration_seconds_count"))
							require.NotEmpty(t, countSeries.TimeSeries.Samples)
							require.Equal(t, 32.0, countSeries.TimeSeries.Samples[0].Value)
						} else {
							// Sender may also convert classic histograms to NHCB.
							hist := findHistogramData(req, "http_request_duration_seconds")
							require.NotNil(t, hist, "Must send either _bucket/_sum/_count series or NHCB native histogram")
							require.Equal(t, 32.0, extractHistogramCount(hist))
							require.InDelta(t, 14.5, extractHistogramSum(hist), 0.01)
						}
					},
				},
			},
		},
	}
}

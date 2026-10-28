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

package openmetrics

import (
	"fmt"
	"math"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func histogramsTests(v Version) []Test {
	isOM2 := v == Version2_0_0 || v == Version2_0_0RC

	tests := []Test{
		{
			Name:        "histograms",
			Description: fmt.Sprintf("Exporters MUST emit valid classic/composite histograms in OpenMetrics %s", v),
			RFCLevel:    MustLevel,
			Version:     v,
			ValidateCases: []ValidateCase{
				{
					Name:        "cumulative_buckets_monotonic_and_inf_matches_count",
					Description: "Histogram classic buckets MUST have strictly increasing upper bounds, monotonically non-decreasing cumulative counts, and +Inf matching sample_count",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr)
						mf := requireFamily(t, res, "http_request_duration_seconds")
						require.Equal(t, dto.MetricType_HISTOGRAM, mf.GetType())
						require.NotEmpty(t, mf.Metric)

						for _, m := range mf.Metric {
							h := m.GetHistogram()
							require.NotNil(t, h)
							require.NotEmpty(t, h.Bucket, "Expected classic histogram buckets on http_request_duration_seconds")

							var prevBound float64
							var prevCount uint64
							for i, b := range h.Bucket {
								ub := b.GetUpperBound()
								if i > 0 {
									require.Greater(t, ub, prevBound, "Bucket upper bounds MUST be strictly increasing")
									require.GreaterOrEqual(t, b.GetCumulativeCount(), prevCount, "Cumulative bucket counts MUST be monotonically non-decreasing")
								}
								prevBound = ub
								prevCount = b.GetCumulativeCount()
							}

							lastBucket := h.Bucket[len(h.Bucket)-1]
							require.True(t, math.IsInf(lastBucket.GetUpperBound(), +1), "Last classic bucket MUST be +Inf")
							require.Equal(t, h.GetSampleCount(), lastBucket.GetCumulativeCount(), "+Inf bucket count MUST equal sample_count")
						}
					},
				},
				{
					Name:        "histogram_start_timestamp",
					Description: "Histogram series SHOULD include a valid start/created timestamp (_created line in OM 1.0, inline st@ in OM 2.0)",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						mf := requireFamily(t, res, "http_request_duration_seconds")
						for _, m := range mf.Metric {
							ct := m.GetHistogram().GetCreatedTimestamp()
							require.NotNil(t, ct, "Histogram series SHOULD include a CreatedTimestamp")
							require.NoError(t, ct.CheckValid())
							require.Positive(t, ct.AsTime().UnixNano())
						}
					},
				},
				{
					Name:        "histogram_wire_representation",
					Description: "Histograms MUST use multi-line _bucket/_sum/_count in OM 1.0 and single-line composite values in OM 2.0",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						lines := sampleLinesForFamily(res.Body, "http_request_duration_seconds")
						require.NotEmpty(t, lines)
						if isOM2 {
							require.Len(t, lines, 1, "OpenMetrics 2.0 histogram with 1 series MUST be a single composite sample line")
							require.Contains(t, lines[0], "{count:", "OpenMetrics 2.0 histogram MUST use composite {count:...,sum:...} format")
							require.Contains(t, lines[0], ",sum:")
							require.Contains(t, lines[0], ",bucket:[")
							require.Contains(t, lines[0], " st@", "OpenMetrics 2.0 histogram SHOULD include inline st@ start timestamp")
						} else {
							require.Greater(t, len(lines), 3, "OpenMetrics 1.0 histogram MUST emit multiple _bucket, _sum, and _count lines")
							require.Contains(t, string(res.Body), "http_request_duration_seconds_bucket{")
							require.Contains(t, string(res.Body), "http_request_duration_seconds_sum")
							require.Contains(t, string(res.Body), "http_request_duration_seconds_count")
						}
					},
				},
			},
		},
	}

	if isOM2 {
		tests = append(tests, Test{
			Name:        "native_histograms",
			Description: "OpenMetrics 2.0 exporters SHOULD support native histogram spans and bucket deltas in composite histogram values",
			RFCLevel:    ShouldLevel,
			Version:     v,
			ValidateCases: []ValidateCase{
				{
					Name:        "native_schema_and_spans_valid",
					Description: "Native histograms MUST have schema in [-4, 8], non-negative zero_threshold, and span bucket totals matching sample_count",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr)
						mf := requireFamily(t, res, "http_request_duration_seconds")
						require.NotEmpty(t, mf.Metric)

						m := mf.Metric[0]
						h := m.GetHistogram()
						require.NotNil(t, h.Schema, "Expected native histogram Schema on http_request_duration_seconds in OpenMetrics 2.0")
						require.GreaterOrEqual(t, h.GetSchema(), int32(-4), "Native histogram schema MUST be >= -4")
						require.LessOrEqual(t, h.GetSchema(), int32(8), "Native histogram schema MUST be <= 8")
						require.GreaterOrEqual(t, h.GetZeroThreshold(), 0.0, "Native histogram zero_threshold MUST be >= 0")
						require.NotEmpty(t, h.PositiveSpan, "Expected positive native histogram spans")
						require.NotEmpty(t, h.PositiveDelta, "Expected positive native histogram bucket deltas")

						var totalBuckets uint64
						var cur int64
						for _, d := range h.PositiveDelta {
							cur += d
							require.GreaterOrEqual(t, cur, int64(0), "Native histogram bucket count MUST be non-negative")
							totalBuckets += uint64(cur)
						}
						cur = 0
						for _, d := range h.NegativeDelta {
							cur += d
							require.GreaterOrEqual(t, cur, int64(0), "Native histogram bucket count MUST be non-negative")
							totalBuckets += uint64(cur)
						}
						require.Equal(t, h.GetSampleCount(), h.GetZeroCount()+totalBuckets,
							"Native histogram zero_count plus bucket counts MUST equal sample_count")
					},
				},
			},
		})
	}

	return tests
}

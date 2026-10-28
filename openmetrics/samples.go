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
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

func samplesTests(v Version) []Test {
	isOM2 := v == Version2_0_0 || v == Version2_0_0RC

	return []Test{
		{
			Name:        "counters",
			Description: fmt.Sprintf("Exporters MUST emit valid non-negative counter samples and start timestamps in OpenMetrics %s", v),
			RFCLevel:    MustLevel,
			Version:     v,
			ValidateCases: []ValidateCase{
				{
					Name:        "counter_values_non_negative",
					Description: "Counter values MUST be non-NaN and non-negative (>= 0)",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr)
						counters := familiesByType(res, dto.MetricType_COUNTER)
						require.NotEmpty(t, counters, "Exposition MUST contain at least one counter metric family")
						for _, mf := range counters {
							for _, m := range mf.Metric {
								require.NotNilf(t, m.Counter, "Metric in counter family %q must have Counter field", mf.GetName())
								val := m.GetCounter().GetValue()
								require.Falsef(t, math.IsNaN(val), "Counter %q value MUST NOT be NaN", mf.GetName())
								require.GreaterOrEqualf(t, val, 0.0, "Counter %q value MUST be non-negative", mf.GetName())
							}
						}

						reqCounter := requireFamily(t, res, "http_requests_total", "http_requests")
						mGet := findMetricWithLabels(reqCounter, map[string]string{"method": "GET", "status": "200"})
						require.NotNil(t, mGet, "Expected counter series with method=GET, status=200")
						require.GreaterOrEqual(t, mGet.GetCounter().GetValue(), 5.0)
					},
				},
				{
					Name:        "counter_start_timestamp",
					Description: "Counter series SHOULD include a valid start/created timestamp (_created line in OM 1.0, inline st@ in OM 2.0)",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						reqCounter := requireFamily(t, res, "http_requests_total", "http_requests")
						for _, m := range reqCounter.Metric {
							ct := m.GetCounter().GetCreatedTimestamp()
							require.NotNilf(t, ct, "Counter %q series SHOULD include a CreatedTimestamp", reqCounter.GetName())
							require.NoError(t, ct.CheckValid())
							require.Positive(t, ct.AsTime().UnixNano())
						}

						if isOM2 {
							lines := sampleLinesForFamily(res.Body, reqCounter.GetName())
							require.NotEmpty(t, lines)
							for _, l := range lines {
								require.Contains(t, l, " st@", "OpenMetrics 2.0 counter sample line SHOULD contain inline st@ start timestamp")
								require.NotContains(t, l, "_created", "OpenMetrics 2.0 MUST NOT emit separate _created lines")
							}
						} else {
							base := strings.TrimSuffix(reqCounter.GetName(), "_total")
							require.Contains(t, string(res.Body), base+"_created", "OpenMetrics 1.0 counter SHOULD emit _created lines")
						}
					},
				},
			},
		},
		{
			Name:        "gauges",
			Description: fmt.Sprintf("Exporters MUST emit valid gauge samples in OpenMetrics %s", v),
			RFCLevel:    MustLevel,
			Version:     v,
			ValidateCases: []ValidateCase{
				{
					Name:        "gauge_values_valid",
					Description: "Gauge metric families MUST contain valid float sample values and expected label sets",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr)
						gauges := familiesByType(res, dto.MetricType_GAUGE)
						require.NotEmpty(t, gauges, "Exposition MUST contain at least one gauge metric family")
						for _, mf := range gauges {
							require.NotEmptyf(t, mf.Metric, "Gauge family %q must contain at least one metric", mf.GetName())
							for _, m := range mf.Metric {
								require.NotNilf(t, m.Gauge, "Metric in gauge family %q must have Gauge field", mf.GetName())
								require.NotNilf(t, m.GetGauge().Value, "Gauge %q must have an explicit sample value", mf.GetName())
							}
						}

						inFlight := requireFamily(t, res, "http.server.active_requests", "http_server_active_requests")
						require.InDelta(t, 3.0, inFlight.Metric[0].GetGauge().GetValue(), 1e-6)
					},
				},
			},
		},
		{
			Name:        "summaries",
			Description: fmt.Sprintf("Exporters MUST emit valid summary metrics in OpenMetrics %s", v),
			RFCLevel:    MustLevel,
			Version:     v,
			ValidateCases: []ValidateCase{
				{
					Name:        "summary_count_and_sum_non_negative",
					Description: "Summary sample_count and sample_sum MUST be non-negative and not NaN",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr)
						mf := requireFamily(t, res, "rpc_latency_seconds")
						require.Equal(t, dto.MetricType_SUMMARY, mf.GetType())
						require.NotEmpty(t, mf.Metric)
						for _, m := range mf.Metric {
							s := m.GetSummary()
							require.NotNil(t, s)
							require.Positive(t, s.GetSampleCount(), "Summary count should be positive")
							require.False(t, math.IsNaN(s.GetSampleSum()), "Summary sum MUST NOT be NaN")
							require.GreaterOrEqual(t, s.GetSampleSum(), 0.0, "Summary sum MUST be non-negative")
						}
					},
				},
				{
					Name:        "summary_quantiles_valid_and_sorted",
					Description: "Summary quantiles MUST be in [0.0, 1.0], sorted in strictly increasing order, with non-negative values",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						mf := requireFamily(t, res, "rpc_latency_seconds")
						for _, m := range mf.Metric {
							s := m.GetSummary()
							require.NotEmpty(t, s.Quantile, "Summary should expose configured quantiles")
							prevQ := -1.0
							for _, q := range s.Quantile {
								qv := q.GetQuantile()
								require.GreaterOrEqual(t, qv, 0.0, "Quantile MUST be >= 0.0")
								require.LessOrEqual(t, qv, 1.0, "Quantile MUST be <= 1.0")
								require.Greater(t, qv, prevQ, "Quantiles MUST be strictly increasing")
								prevQ = qv

								val := q.GetValue()
								if !math.IsNaN(val) {
									require.GreaterOrEqual(t, val, 0.0, "Summary quantile value MUST NOT be negative")
								}
							}
						}
					},
				},
				{
					Name:        "summary_wire_representation",
					Description: "Summaries MUST use multi-line representation in OM 1.0 and single-line composite values in OM 2.0",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						lines := sampleLinesForFamily(res.Body, "rpc_latency_seconds")
						require.NotEmpty(t, lines)
						if isOM2 {
							require.Len(t, lines, 1, "OpenMetrics 2.0 summary with 1 series MUST be a single composite sample line")
							require.Contains(t, lines[0], "{count:", "OpenMetrics 2.0 summary MUST use composite {count:...,sum:...,quantile:[...]} format")
							require.Contains(t, lines[0], ",sum:")
							require.Contains(t, lines[0], ",quantile:[")
						} else {
							require.Greater(t, len(lines), 1, "OpenMetrics 1.0 summary MUST use multiple sample lines (_sum, _count, quantile)")
							require.Contains(t, string(res.Body), "rpc_latency_seconds_sum")
							require.Contains(t, string(res.Body), "rpc_latency_seconds_count")
						}
					},
				},
			},
		},
	}
}

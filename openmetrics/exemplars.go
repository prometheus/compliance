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
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

const om1ExemplarMaxRunes = 128

func exemplarsTests(v Version) []Test {
	isOM2 := v == Version2_0_0 || v == Version2_0_0RC

	validateCases := []ValidateCase{
		{
			Name:        "counter_exemplar_present",
			Description: "Counters observed with exemplars SHOULD include a valid exemplar with trace_id label, value, and timestamp",
			RFCLevel:    ShouldLevel,
			Validate: func(t *testing.T, res ScrapeResult) {
				require.NoError(t, res.ParseErr)
				mf := requireFamily(t, res, "http_requests_total", "http_requests")
				m := findMetricWithLabels(mf, map[string]string{"method": "GET", "status": "200"})
				require.NotNil(t, m)

				ex := m.GetCounter().GetExemplar()
				require.NotNil(t, ex, "Expected exemplar on counter series http_requests{method=\"GET\",status=\"200\"}")
				lbls := labelMap(ex.Label)
				require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", lbls["trace_id"])
				require.Equal(t, "00f067aa0ba902b7", lbls["span_id"])
				require.Positive(t, ex.GetValue())
				require.NotNil(t, ex.GetTimestamp(), "Exemplar SHOULD include a timestamp")
			},
		},
		{
			Name:        "histogram_exemplar_present",
			Description: "Histograms observed with exemplars SHOULD include a valid exemplar on a bucket (OM 1.0) or composite sample (OM 2.0)",
			RFCLevel:    ShouldLevel,
			Validate: func(t *testing.T, res ScrapeResult) {
				require.NoError(t, res.ParseErr)
				mf := requireFamily(t, res, "http_request_duration_seconds")
				require.NotEmpty(t, mf.Metric)
				h := mf.Metric[0].GetHistogram()
				require.NotNil(t, h)

				var found *dto.Exemplar
				if isOM2 {
					if len(h.Exemplars) > 0 {
						found = h.Exemplars[0]
					}
				} else {
					for _, b := range h.Bucket {
						if b.GetExemplar() != nil {
							found = b.GetExemplar()
							break
						}
					}
				}
				require.NotNil(t, found, "Expected exemplar on histogram http_request_duration_seconds")
				lbls := labelMap(found.Label)
				require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", lbls["trace_id"])
				require.Positive(t, found.GetValue())
			},
		},
	}

	if isOM2 {
		validateCases = append(validateCases, ValidateCase{
			Name:        "exemplar_extended_labels_over_128_runes",
			Description: "OpenMetrics 2.0 exporters SHOULD preserve exemplar label sets exceeding 128 runes",
			RFCLevel:    ShouldLevel,
			Validate: func(t *testing.T, res ScrapeResult) {
				require.NoError(t, res.ParseErr)
				sizeCounter := requireFamily(t, res, "http_request_size_bytes_total", "http_request_size_bytes")
				require.NotEmpty(t, sizeCounter.Metric)
				ex := sizeCounter.Metric[0].GetCounter().GetExemplar()
				require.NotNil(t, ex)
				require.Greater(t, exemplarLabelRunes(ex), om1ExemplarMaxRunes,
					"Expected OpenMetrics 2.0 exemplar to preserve >128 runes")
				lbls := labelMap(ex.Label)
				require.NotEmpty(t, lbls["attr_big"], "Expected extended exemplar attribute attr_big to be preserved in OpenMetrics 2.0")
			},
		})
	} else {
		validateCases = append(validateCases,
			ValidateCase{
				Name:        "exemplar_label_runes_within_spec_limit",
				Description: "Exemplar label sets MUST NOT exceed 128 UTF-8 runes in OpenMetrics 1.0",
				RFCLevel:    MustLevel,
				Validate: func(t *testing.T, res ScrapeResult) {
					require.NoError(t, res.ParseErr)
					for _, mf := range res.OrderedFamilies {
						for _, m := range mf.Metric {
							if ex := m.GetCounter().GetExemplar(); ex != nil {
								require.LessOrEqualf(t, exemplarLabelRunes(ex), om1ExemplarMaxRunes,
									"Counter %q exemplar label runes MUST be <= %d", mf.GetName(), om1ExemplarMaxRunes)
							}
							for _, b := range m.GetHistogram().GetBucket() {
								if ex := b.GetExemplar(); ex != nil {
									require.LessOrEqualf(t, exemplarLabelRunes(ex), om1ExemplarMaxRunes,
										"Histogram %q bucket exemplar label runes MUST be <= %d", mf.GetName(), om1ExemplarMaxRunes)
								}
							}
						}
					}
				},
			},
			ValidateCase{
				Name:        "exemplar_truncated_retains_trace_id",
				Description: "OpenMetrics 1.0 exporters SHOULD truncate oversized non-trace exemplar labels to stay within 128 runes while retaining trace_id",
				RFCLevel:    ShouldLevel,
				Validate: func(t *testing.T, res ScrapeResult) {
					require.NoError(t, res.ParseErr)
					sizeCounter := requireFamily(t, res, "http_request_size_bytes_total", "http_request_size_bytes")
					require.NotEmpty(t, sizeCounter.Metric)
					ex := sizeCounter.Metric[0].GetCounter().GetExemplar()
					require.NotNil(t, ex, "Expected exemplar on http_request_size_bytes counter")
					lbls := labelMap(ex.Label)
					require.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", lbls["trace_id"], "trace_id SHOULD be retained on exemplar")
					require.LessOrEqual(t, exemplarLabelRunes(ex), om1ExemplarMaxRunes)
					require.NotContains(t, lbls, "attr_big", "OpenMetrics 1.0 exemplar SHOULD truncate oversized label pairs to stay <= 128 runes")
				},
			},
		)
	}

	return []Test{
		{
			Name:          "exemplars",
			Description:   fmt.Sprintf("Exporters MUST emit valid exemplars conforming to OpenMetrics %s rune limits and timestamp rules", v),
			RFCLevel:      MustLevel,
			Version:       v,
			ValidateCases: validateCases,
		},
	}
}

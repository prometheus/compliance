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
	"regexp"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/compliance"
	"github.com/stretchr/testify/require"
)

var (
	legacyMetricNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	legacyLabelNameRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

func escapingTests(v Version) []Test {
	utf8Level := compliance.ShouldLevel
	if v == Version2_0_0 || v == Version2_0_0RC {
		utf8Level = compliance.MustLevel
	}

	tests := []Test{
		{
			Name:        "escaping_allow_utf8",
			Description: "Exporters MUST/SHOULD support escaping=allow-utf-8 and preserve UTF-8 metric and label names via quoting",
			RFCLevel:    utf8Level,
			Version:     v,
			Request: ScrapeRequest{
				Escaping: model.AllowUTF8,
			},
			ValidateCases: []compliance.ValidateCase[ScrapeResult]{
				{
					Name:        "content_type_escaping_param",
					Description: "Content-Type SHOULD echo escaping=allow-utf-8 when negotiated",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.MediaParseErr)
						require.Equal(t, model.AllowUTF8, res.MediaParams[model.EscapingKey])
					},
				},
				{
					Name:        "utf8_metric_and_label_names_preserved",
					Description: "UTF-8 dotted metric and label names (http.server.active_requests, service.name) MUST be preserved unescaped after parsing",
					RFCLevel:    utf8Level,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr)
						mf := requireFamily(t, res, "http.server.active_requests")
						m := findMetricWithLabels(mf, map[string]string{"service.name": "demo-api"})
						require.NotNil(t, m, "Expected series http.server.active_requests{service.name=\"demo-api\"}")
						require.NotEmpty(t, sampleLinesForFamily(res.Body, "http.server.active_requests"),
							"Expected quoted UTF-8 metric name on the wire")
					},
				},
			},
		},
		{
			Name:        "escaping_underscores",
			Description: "Exporters SHOULD support escaping=underscores and replace non-legacy characters with underscores",
			RFCLevel:    compliance.ShouldLevel,
			Version:     v,
			Request: ScrapeRequest{
				Escaping: model.EscapeUnderscores,
			},
			ValidateCases: []compliance.ValidateCase[ScrapeResult]{
				{
					Name:        "content_type_escaping_param",
					Description: "Content-Type SHOULD include escaping=underscores when negotiated",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.MediaParseErr)
						require.Equal(t, model.EscapeUnderscores, res.MediaParams[model.EscapingKey])
					},
				},
				{
					Name:        "all_names_conform_to_legacy_charset",
					Description: "Under escaping=underscores, all metric and label names MUST conform to the legacy ASCII character set",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr)
						for _, mf := range res.OrderedFamilies {
							require.Truef(t, legacyMetricNameRE.MatchString(mf.GetName()),
								"Metric name %q MUST match legacy regex under escaping=underscores", mf.GetName())
							for _, m := range mf.Metric {
								for _, lp := range m.Label {
									require.Truef(t, legacyLabelNameRE.MatchString(lp.GetName()),
										"Label name %q on %q MUST match legacy regex under escaping=underscores", lp.GetName(), mf.GetName())
								}
							}
						}
					},
				},
				{
					Name:        "dotted_names_translated_to_underscores",
					Description: "Dotted metric and label names SHOULD be translated to underscores (http_server_active_requests, service_name)",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						mf := requireFamily(t, res, "http_server_active_requests")
						m := findMetricWithLabels(mf, map[string]string{"service_name": "demo-api"})
						require.NotNil(t, m, "Expected series http_server_active_requests{service_name=\"demo-api\"}")
					},
				},
			},
		},
		{
			Name:        "escaping_dots",
			Description: "Exporters SHOULD support escaping=dots and encode dots as _dot_",
			RFCLevel:    compliance.ShouldLevel,
			Version:     v,
			Request: ScrapeRequest{
				Escaping: model.EscapeDots,
			},
			ValidateCases: []compliance.ValidateCase[ScrapeResult]{
				{
					Name:        "content_type_escaping_param",
					Description: "Content-Type SHOULD include escaping=dots when negotiated",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.MediaParseErr)
						require.Equal(t, model.EscapeDots, res.MediaParams[model.EscapingKey])
					},
				},
				{
					Name:        "dotted_names_escaped_as_dot",
					Description: "Dotted metric and label names SHOULD be escaped as _dot_ with underscores doubled (http_dot_server_dot_active__requests, service_dot_name)",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr)
						mf := requireFamily(t, res, "http_dot_server_dot_active__requests")
						m := findMetricWithLabels(mf, map[string]string{"service_dot_name": "demo-api"})
						require.NotNil(t, m, "Expected series http_dot_server_dot_active__requests{service_dot_name=\"demo-api\"}")
					},
				},
			},
		},
		{
			Name:        "escaping_values",
			Description: "Exporters SHOULD support escaping=values and hex-encode non-legacy characters with U__ prefix",
			RFCLevel:    compliance.ShouldLevel,
			Version:     v,
			Request: ScrapeRequest{
				Escaping: model.EscapeValues,
			},
			ValidateCases: []compliance.ValidateCase[ScrapeResult]{
				{
					Name:        "content_type_escaping_param",
					Description: "Content-Type SHOULD include escaping=values when negotiated",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.MediaParseErr)
						require.Equal(t, model.EscapeValues, res.MediaParams[model.EscapingKey])
					},
				},
				{
					Name:        "dotted_names_hex_encoded",
					Description: "Dotted metric and label names SHOULD be hex-encoded with U__ prefix and underscores doubled (U__http_2e_server_2e_active__requests, U__service_2e_name)",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr)
						mf := requireFamily(t, res, "U__http_2e_server_2e_active__requests")
						m := findMetricWithLabels(mf, map[string]string{"U__service_2e_name": "demo-api"})
						require.NotNil(t, m, "Expected series U__http_2e_server_2e_active__requests{U__service_2e_name=\"demo-api\"}")
					},
				},
			},
		},
	}

	if v == Version2_0_0 || v == Version2_0_0RC {
		tests = append(tests, Test{
			Name:        "escaping_default_allow_utf8",
			Description: fmt.Sprintf("OpenMetrics %s MUST default to allow-utf-8 when no escaping parameter is specified in Accept", v),
			RFCLevel:    compliance.MustLevel,
			Version:     v,
			Request: ScrapeRequest{
				Accept: fmt.Sprintf("application/openmetrics-text; version=%s", v),
			},
			ValidateCases: []compliance.ValidateCase[ScrapeResult]{
				{
					Name:        "preserves_utf8_names_by_default",
					Description: "Without an explicit escaping parameter in Accept, OpenMetrics 2.0 MUST preserve UTF-8 metric and label names",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr)
						mf := requireFamily(t, res, "http.server.active_requests")
						m := findMetricWithLabels(mf, map[string]string{"service.name": "demo-api"})
						require.NotNil(t, m, "Expected UTF-8 series http.server.active_requests{service.name=\"demo-api\"} by default in OpenMetrics 2.0")
					},
				},
			},
		})
	}

	return tests
}

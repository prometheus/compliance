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
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/compliance"
	"github.com/stretchr/testify/require"
)

// gzipMagicHeader is the 2-byte magic number at the start of a gzip stream (RFC 1952).
var gzipMagicHeader = []byte{0x1f, 0x8b}

func protocolTests(v Version) []Test {
	tests := []Test{
		{
			Name:        "content_negotiation",
			Description: fmt.Sprintf("Exporters MUST negotiate OpenMetrics %s when requested via the Accept header", v),
			RFCLevel:    compliance.MustLevel,
			Version:     v,
			ValidateCases: []compliance.ValidateCase[ScrapeResult]{
				{
					Name:        "http_status_ok",
					Description: "Exporters MUST respond with HTTP 200 OK to a valid scrape request",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.Equal(t, http.StatusOK, res.StatusCode, "HTTP status code MUST be 200 OK")
					},
				},
				{
					Name:        "content_type_media_type",
					Description: "Exporters MUST respond with Content-Type media type application/openmetrics-text",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NotEmpty(t, res.ContentType, "Content-Type header MUST be present")
						require.NoError(t, res.MediaParseErr, "Content-Type header MUST be a valid MIME media type")
						require.Equal(t, expfmt.OpenMetricsType, res.MediaType, "Media type MUST be application/openmetrics-text")
					},
				},
				{
					Name:        "content_type_version_param",
					Description: fmt.Sprintf("Exporters MUST include version=%s in the Content-Type header", v),
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.MediaParseErr)
						require.Equal(t, string(v), res.MediaParams["version"], "Content-Type version parameter MUST be %q", v)
					},
				},
				{
					Name:        "content_type_charset_utf8",
					Description: "Exporters MUST include charset=utf-8 in the Content-Type header",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.MediaParseErr)
						require.Equal(t, "utf-8", strings.ToLower(res.MediaParams["charset"]), "Content-Type charset parameter MUST be utf-8")
					},
				},
			},
		},
		{
			Name:        "negotiation_quality_priority",
			Description: "Exporters MUST honor Accept quality values (q=) when selecting the exposition format",
			RFCLevel:    compliance.MustLevel,
			Version:     v,
			Request: ScrapeRequest{
				Accept: fmt.Sprintf("text/plain;version=0.0.4;q=0.4, application/openmetrics-text;version=%s;q=0.9, */*;q=0.1", v),
			},
			ValidateCases: []compliance.ValidateCase[ScrapeResult]{
				{
					Name:        "selects_highest_q_openmetrics_version",
					Description: fmt.Sprintf("Exporters MUST select OpenMetrics %s when it has a higher q= priority than text/plain", v),
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.Equal(t, http.StatusOK, res.StatusCode)
						require.NoError(t, res.MediaParseErr)
						require.Equal(t, expfmt.OpenMetricsType, res.MediaType)
						require.Equal(t, string(v), res.MediaParams["version"])
					},
				},
				{
					Name:        "payload_valid_abnf",
					Description: "Negotiated response MUST decode without ABNF syntax or semantic errors",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr)
						require.NotEmpty(t, res.OrderedFamilies)
					},
				},
			},
		},
		{
			Name:        "compression_gzip",
			Description: "Exporters SHOULD support gzip compression when Accept-Encoding: gzip is sent",
			RFCLevel:    compliance.ShouldLevel,
			Version:     v,
			Request: ScrapeRequest{
				AcceptEncoding: "gzip",
			},
			ValidateCases: []compliance.ValidateCase[ScrapeResult]{
				{
					Name:        "content_encoding_gzip",
					Description: "Exporters SHOULD return Content-Encoding: gzip and a valid gzip stream when requested",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.Equal(t, http.StatusOK, res.StatusCode)
						require.Equal(t, "gzip", res.Headers.Get("Content-Encoding"), "Content-Encoding header SHOULD be gzip")
						require.True(t, bytes.HasPrefix(res.RawBody, gzipMagicHeader), "Raw response body MUST start with gzip magic header 0x1f8b")
						require.NoError(t, res.DecompressErr, "Response body MUST be a valid gzip stream")
					},
				},
				{
					Name:        "decompressed_payload_valid",
					Description: "Decompressed gzip payload MUST be valid OpenMetrics exposition",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.DecompressErr)
						require.NoError(t, res.ParseErr)
						require.NotEmpty(t, res.OrderedFamilies)
					},
				},
			},
		},
	}

	if v == Version2_0_0 {
		tests = append(tests, Test{
			Name:        "negotiation_rc_version",
			Description: "OpenMetrics 2.0 exporters SHOULD also accept version=2.0.0-rc during the release candidate transition",
			RFCLevel:    compliance.ShouldLevel,
			Version:     v,
			Request: ScrapeRequest{
				Accept: "application/openmetrics-text; version=2.0.0-rc; escaping=allow-utf-8",
			},
			ValidateCases: []compliance.ValidateCase[ScrapeResult]{
				{
					Name:        "negotiates_2_0_0_rc",
					Description: "Exporters SHOULD negotiate version=2.0.0-rc when explicitly requested",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.Equal(t, http.StatusOK, res.StatusCode)
						require.NoError(t, res.MediaParseErr)
						require.Equal(t, expfmt.OpenMetricsType, res.MediaType)
						require.Equal(t, expfmt.OpenMetricsVersion_2_0_0_rc, res.MediaParams["version"])
						require.NoError(t, res.ParseErr)
					},
				},
			},
		})
	}

	return tests
}

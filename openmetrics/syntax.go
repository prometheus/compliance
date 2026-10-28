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
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

func syntaxTests(v Version) []Test {
	return []Test{
		{
			Name:        "syntax",
			Description: fmt.Sprintf("Exporters MUST emit valid UTF-8, LF-delimited OpenMetrics %s text conforming to the ABNF grammar", v),
			RFCLevel:    MustLevel,
			Version:     v,
			ValidateCases: []ValidateCase{
				{
					Name:        "non_empty_exposition",
					Description: "Exposition payload MUST NOT be empty and MUST at least contain # EOF",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NotEmpty(t, res.Body, "Exposition payload MUST NOT be empty")
					},
				},
				{
					Name:        "valid_utf8",
					Description: "Exposition payload MUST be valid UTF-8",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.True(t, utf8.Valid(res.Body), "Exposition payload MUST be valid UTF-8")
					},
				},
				{
					Name:        "line_endings_lf_only",
					Description: "Lines MUST use LF (\\n) line endings and MUST NOT use CRLF (\\r\\n)",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.False(t, bytes.Contains(res.Body, []byte("\r\n")), "Exposition MUST NOT contain CRLF (\\r\\n) line endings")
					},
				},
				{
					Name:        "no_blank_lines",
					Description: "Exposition MUST NOT contain empty lines",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						trimmed := strings.TrimSuffix(string(res.Body), "\n")
						for i, line := range strings.Split(trimmed, "\n") {
							require.NotEmptyf(t, line, "Line %d MUST NOT be empty", i+1)
						}
					},
				},
				{
					Name:        "ends_with_eof",
					Description: "Exposition MUST end with # EOF and MUST NOT have trailing data after # EOF",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						s := string(res.Body)
						require.True(t, strings.HasSuffix(s, "# EOF\n") || strings.HasSuffix(s, "# EOF"),
							"Exposition MUST end with '# EOF\\n' or '# EOF'")
						idx := strings.Index(s, "# EOF")
						require.NotEqual(t, -1, idx, "Exposition MUST contain '# EOF'")
						trailing := s[idx+len("# EOF"):]
						require.True(t, trailing == "" || trailing == "\n",
							"Exposition MUST NOT have trailing data after '# EOF', got %q", trailing)
					},
				},
				{
					Name:        "abnf_grammar_and_semantics",
					Description: fmt.Sprintf("Exposition MUST conform to the OpenMetrics %s ABNF grammar and semantic rules", v),
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr, "Exposition MUST parse without ABNF or semantic errors")
						require.NotEmpty(t, res.OrderedFamilies, "Exposition SHOULD contain at least one decoded MetricFamily")
					},
				},
			},
		},
	}
}

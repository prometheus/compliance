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

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance"
	"github.com/stretchr/testify/require"
)

func symbolsTests() []Test {
	return []Test{
		{
			Name:        "symbols",
			Description: "Senders MUST provide a valid, deduplicated symbol table starting with an empty string",
			RFCLevel:    compliance.MustLevel,
			ScrapeData: `# HELP http_requests Total HTTP requests
# TYPE http_requests counter
http_requests_total{method="GET",status="200",handler="/api/v1"} 100 # {trace_id="abc123"} 1 1234567890.0
http_requests_created{method="GET",status="200",handler="/api/v1"} 1000.0
http_requests_total{method="POST",status="200",handler="/api/v1"} 50
http_requests_created{method="POST",status="200",handler="/api/v1"} 1000.0
http_requests_total{method="GET",status="404",handler="/api/v1"} 10
http_requests_created{method="GET",status="404",handler="/api/v1"} 1000.0
# HELP http_errors Total HTTP errors
# TYPE http_errors counter
http_errors_total{method="GET",status="500",handler="/api/v1"} 5
http_errors_created{method="GET",status="500",handler="/api/v1"} 1000.0
`,
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.NotEmpty(t, res.Requests, "Should receive at least 1 request")
			},
			ValidateCases: []ValidateCase{
				{
					Name:        "empty_string_at_index_zero",
					Description: "The first element of the symbols table MUST be an empty string",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							require.NotEmpty(t, req.RW2.Symbols, "Symbols table MUST NOT be empty")
							require.Equal(t, "", req.RW2.Symbols[0], "First element of symbols table MUST be an empty string")
						}
					},
				},
				{
					Name:        "deduplicated_strings",
					Description: "The symbols table MUST contain deduplicated strings",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							seen := make(map[string]int, len(req.RW2.Symbols))
							for idx, sym := range req.RW2.Symbols {
								if prevIdx, exists := seen[sym]; exists {
									t.Errorf("duplicate symbol %q found at indices %d and %d", sym, prevIdx, idx)
								}
								seen[sym] = idx
							}
						}
					},
				},
				{
					Name:        "valid_references",
					Description: "All symbol references MUST point to an existing index in the symbols array",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							symLen := uint32(len(req.RW2.Symbols))
							require.NotEmpty(t, req.RW2.Timeseries, "Request must contain timeseries")
							for tsIdx, ts := range req.RW2.Timeseries {
								for refIdx, ref := range ts.LabelsRefs {
									require.Less(t, ref, symLen, "Timeseries[%d].LabelsRefs[%d] out of bounds", tsIdx, refIdx)
								}
								require.Less(t, ts.Metadata.HelpRef, symLen, "Timeseries[%d].Metadata.HelpRef out of bounds", tsIdx)
								require.Less(t, ts.Metadata.UnitRef, symLen, "Timeseries[%d].Metadata.UnitRef out of bounds", tsIdx)
								for exIdx, ex := range ts.Exemplars {
									for refIdx, ref := range ex.LabelsRefs {
										require.Less(t, ref, symLen, "Timeseries[%d].Exemplars[%d].LabelsRefs[%d] out of bounds", tsIdx, exIdx, refIdx)
									}
								}
							}
						}
					},
				},
			},
		},
	}
}

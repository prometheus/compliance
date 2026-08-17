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
	"github.com/stretchr/testify/require"
)

func exemplarsTests() []Test {
	return []Test{
		{
			Name:        "exemplar_fields_valid",
			Description: "Sender MUST send valid exemplars if supported",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `# TYPE request_count counter
request_count 1000 # {trace_id="abc123"} 999 1234567890.123
`,
			Validate: func(t *testing.T, res ReceiverResult) {
				var foundExemplar bool
				for _, req := range res.Requests {
					if req.RW2 != nil {
						for _, ts := range req.RW2.Timeseries {
							for _, ex := range ts.Exemplars {
								foundExemplar = true
								require.NotZero(t, ex.Timestamp, "Exemplar timestamp must be set")
								require.Equal(t, 0, len(ex.LabelsRefs)%2, "Exemplar labels must be even length")
								// Ensure references are valid
								for _, ref := range ex.LabelsRefs {
									require.Less(t, int(ref), len(req.RW2.Symbols), "Exemplar label ref out of bounds")
								}
							}
						}
					}
				}
				// It's a MAY for senders to support exemplars, but if they send them, they must be valid.
				// We won't require foundExemplar to be true if they don't support it, but since they are
				// scraping the data, we hope to see it. Wait, the test is MUST if sent.
				// For the sake of validation, let's just log if not found.
				if !foundExemplar {
					t.Log("Sender did not send exemplars (optional)")
				}
			},
		},
	}
}

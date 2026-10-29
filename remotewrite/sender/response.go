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
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance"
	"github.com/stretchr/testify/require"
)

func responseTests() []Test {
	return []Test{
		{
			Name:        "response_handling",
			Description: "Senders MUST accept 2xx responses, ignore response bodies, and not assume 0 written when written headers are omitted.",
			RFCLevel:    compliance.MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `# TYPE response_test_metric gauge
response_test_metric 10
`,
			TestResponses: []ReceiverResponse{
				{
					StatusCode:         http.StatusOK,
					Body:               "accepted with diagnostic message that sender must ignore",
					OmitWrittenHeaders: true,
				},
				{
					StatusCode: http.StatusNoContent,
				},
			},
			Validate: func(t *testing.T, res ReceiverResult) {
				require.Len(t, res.Requests, 2)
				require.NotNil(t, res.Requests[0].RW2)
				require.NotNil(t, res.Requests[1].RW2)
			},
			ValidateCases: []compliance.ValidateCase[ReceiverResult]{
				{
					Name:        "accept_200_and_ignore_body_and_missing_headers",
					Description: "Sender MUST treat 200 OK with body and omitted X-Prometheus-Remote-Write-*-Written headers as success and advance to the next batch.",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						res0 := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "response_test_metric")
						ts0 := flattenTimeseriesResult(t, res0).TimeSeries
						require.NotEmpty(t, ts0.Samples)
						maxTS0 := ts0.Samples[len(ts0.Samples)-1].Timestamp

						res1 := requireTimeseriesByMetricName(t, res.Requests[1].RW2, "response_test_metric")
						ts1 := flattenTimeseriesResult(t, res1).TimeSeries
						require.NotEmpty(t, ts1.Samples)
						minTS1 := ts1.Samples[0].Timestamp

						require.Greater(t, minTS1, maxTS0, "Second request must contain newer samples rather than retrying the accepted first batch")
					},
				},
			},
		},
	}
}

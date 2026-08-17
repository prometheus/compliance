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

func histogramsTests() []Test {
	return []Test{
		{
			Name:        "histogram_fields_valid",
			Description: "Sender MUST send valid native histograms if supported",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `# HELP request_duration_seconds Request duration in seconds
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="0.1"} 100
request_duration_seconds_bucket{le="0.5"} 250
request_duration_seconds_bucket{le="1.0"} 500
request_duration_seconds_bucket{le="+Inf"} 1000
request_duration_seconds_sum 450.5
request_duration_seconds_count 1000
`,
			Validate: func(t *testing.T, res ReceiverResult) {
				var foundHistogram bool
				for _, req := range res.Requests {
					if req.RW2 != nil {
						for _, ts := range req.RW2.Timeseries {
							for _, hist := range ts.Histograms {
								foundHistogram = true
								require.NotZero(t, hist.Count, "Histogram must have count")
							}
						}
					}
				}
				if !foundHistogram {
					t.Log("Sender did not send native histograms (or sent classic)")
				}
			},
		},
	}
}

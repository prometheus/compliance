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

func batchingTests() []Test {
	return []Test{
		{
			Name:        "multiple_series_per_request",
			Description: "Senders SHOULD use Remote-Write to send samples for multiple series in a single request.",
			RFCLevel:    ShouldLevel,
			ScrapeData: `# Multiple metrics to batch
http_requests_total{method="GET",status="200"} 1000
http_requests_total{method="POST",status="200"} 500
http_requests_total{method="GET",status="404"} 50
cpu_usage_percent 45.2
memory_usage_bytes 1048576
disk_io_bytes_total 1000000
`,
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 1 request")

				// At least one request should batch multiple series
				batched := false
				for _, req := range res.Requests {
					if req.RW2 != nil && len(req.RW2.Timeseries) >= 3 {
						batched = true
						break
					}
				}
				require.True(t, batched, "Sender should batch multiple series")
			},
		},
	}
}

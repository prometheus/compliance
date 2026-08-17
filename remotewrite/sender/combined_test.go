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

func combinedTests() []Test {
	return []Test{
		{
			Name:        "real_world_scenario",
			Description: "Sender MUST handle realistic mixed metric payload",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `# Realistic scrape output with multiple metric types
# TYPE up gauge
up 1

# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 45.67

# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 2097152

# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.05"} 100
http_request_duration_seconds_bucket{le="0.1"} 200
http_request_duration_seconds_bucket{le="0.5"} 450
http_request_duration_seconds_bucket{le="1.0"} 480
http_request_duration_seconds_bucket{le="+Inf"} 500
http_request_duration_seconds_sum 125.5
http_request_duration_seconds_count 500

# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 5000
http_requests_total{method="POST",code="201"} 1000
`,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Expected at least 1 request")

				metricNames := make(map[string]bool)

				for _, req := range res.Requests {
					if req.RW2 != nil {
						for _, ts := range req.RW2.Timeseries {
							// Find the __name__ label
							var metricName string
							for i := 0; i < len(ts.LabelsRefs); i += 2 {
								keyIdx := ts.LabelsRefs[i]
								valIdx := ts.LabelsRefs[i+1]
								if req.RW2.Symbols[keyIdx] == "__name__" {
									metricName = req.RW2.Symbols[valIdx]
									break
								}
							}
							metricNames[metricName] = true

							require.NotEmpty(t, metricName, "Each timeseries must have __name__")

							// Validate no mixed samples and histograms.
							if len(ts.Samples) > 0 && len(ts.Histograms) > 0 {
								require.Fail(t, "Timeseries must not mix samples and histograms")
							}
						}
					}
				}

				require.NotEmpty(t, metricNames, "Request must contain metrics")
				require.GreaterOrEqual(t, len(metricNames), 3, "Real-world scenario should have multiple distinct metrics")
			},
		},
	}
}

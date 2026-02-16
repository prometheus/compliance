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

package main

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance/remotewrite/sender/sendertest"
	"github.com/stretchr/testify/require"
)

// TestExemplars validates exemplar encoding.
// TODO(bwplotka): Add tests for NHCB/NH
func TestExemplars(t *testing.T) {
	sendertest.Run(t,
		targetsToTest,
		sendertest.Case{
			// TODO(bwplotka): Prometheus will fail this until https://github.com/prometheus/prometheus/issues/17857 is fixed.
			Name:        "exemplar_per_sample",
			Description: "Sender MAY attach exemplars",
			RFCLevel:    sendertest.MayLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.1"} 50 # {trace_id="abc123xyz"} 0.05 1234567890.123
http_request_duration_seconds_bucket{le="+Inf"} 100
http_request_duration_seconds_sum 10.5
http_request_duration_seconds_count 100
# TYPE http_requests_total counter
http_requests_total 1000 # {trace_id="abc123",span_id="def456"} 999`,
			Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)

				ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "http_request_duration_seconds_bucket")
				require.Len(t, ts.Exemplars, 1, "Exemplar is not present on http_request_duration_seconds_bucket")

				ts, _ = requireTimeseriesByMetricName(t, res.Requests[0].RW2, "http_requests_total")
				require.Len(t, ts.Exemplars, 1, "Exemplar is not present on http_requests_total")
			},
			// https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/#exemplars.
			ValidateCases: []sendertest.ValidateCase{
				{
					Name:        "value",
					Description: "Exemplars MUST contain value",
					RFCLevel:    sendertest.MustLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "http_request_duration_seconds_bucket")
						require.Equal(t, 0.05, ts.Exemplars[0].Value)
						ts, _ = requireTimeseriesByMetricName(t, res.Requests[0].RW2, "http_requests_total")
						require.Equal(t, 999, ts.Exemplars[0].Value)
					},
				},
				{
					Name:        "labels",
					Description: "Exemplars MAY contain labels",
					RFCLevel:    sendertest.MayLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "http_request_duration_seconds_bucket")
						require.Equal(t, map[string]string{
							"trace_id": "abc123xyz",
						}, extractExemplarLabels(&ts.Exemplars[0], res.Requests[0].RW2.Symbols))
						ts, _ = requireTimeseriesByMetricName(t, res.Requests[0].RW2, "http_requests_total")
						require.Equal(t, map[string]string{
							"trace_id": "abc123",
							"span_id":  "def456",
						}, extractExemplarLabels(&ts.Exemplars[0], res.Requests[0].RW2.Symbols))
					},
				},
				{
					Name:        "timestamp",
					Description: "Exemplars MUST contain timestamp",
					RFCLevel:    sendertest.MustLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "http_request_duration_seconds_bucket")
						require.Equal(t, 1234567890.123, ts.Exemplars[0].Timestamp)
						ts, _ = requireTimeseriesByMetricName(t, res.Requests[0].RW2, "http_requests_total")
						require.Greater(t, time.Now().Add(-10*time.Minute), ts.Exemplars[0].Timestamp) // More or less scrape time.
					},
				},
			},
		},
		// Exemplars are not part of the official 1.0 spec, so use the recommended level.
		sendertest.Case{
			Name:        "exemplar_per_series",
			Description: "Sender attach exemplars",
			RFCLevel:    sendertest.RecommendedLevel,
			Version:     remote.WriteV1MessageType,
			ScrapeData: `# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.1"} 50 # {trace_id="abc123xyz"} 0.05 1234567890.123
http_request_duration_seconds_bucket{le="+Inf"} 100
http_request_duration_seconds_sum 10.5
http_request_duration_seconds_count 100
# TYPE http_requests_total counter
http_requests_total 1000 # {trace_id="abc123",span_id="def456"} 999`,
			Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)

				t.Log("DEBUG:", res.Requests[0].RW1)

				ts := requireTimeseriesRW1ByMetricName(t, res.Requests[0].RW1, "http_request_duration_seconds_bucket")
				require.Len(t, ts.Exemplars, 1, "Exemplar is not present on http_request_duration_seconds_bucket")

				ts = requireTimeseriesRW1ByMetricName(t, res.Requests[0].RW1, "http_requests_total")
				require.Len(t, ts.Exemplars, 1, "Exemplar is not present on http_requests_total")
			},
			ValidateCases: []sendertest.ValidateCase{
				{
					Name:        "value",
					Description: "Exemplars MUST contain value",
					RFCLevel:    sendertest.MustLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts := requireTimeseriesRW1ByMetricName(t, res.Requests[0].RW1, "http_request_duration_seconds_bucket")
						require.Equal(t, 0.05, ts.Exemplars[0].Value)
						ts = requireTimeseriesRW1ByMetricName(t, res.Requests[0].RW1, "http_requests_total")
						require.Equal(t, 999, ts.Exemplars[0].Value)
					},
				},
				{
					Name:        "labels",
					Description: "Exemplars MAY contain labels",
					RFCLevel:    sendertest.MayLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts := requireTimeseriesRW1ByMetricName(t, res.Requests[0].RW1, "http_request_duration_seconds_bucket")
						require.Equal(t, map[string]string{
							"trace_id": "abc123xyz",
						}, ts.Exemplars[0].Labels)
						ts = requireTimeseriesRW1ByMetricName(t, res.Requests[0].RW1, "http_requests_total")
						require.Equal(t, map[string]string{
							"trace_id": "abc123",
							"span_id":  "def456",
						}, ts.Exemplars[0].Labels)
					},
				},
				{
					Name:        "timestamp",
					Description: "Exemplars MUST contain timestamp",
					RFCLevel:    sendertest.MustLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts := requireTimeseriesRW1ByMetricName(t, res.Requests[0].RW1, "http_request_duration_seconds_bucket")
						require.Equal(t, 1234567890.123, ts.Exemplars[0].Timestamp)
						ts = requireTimeseriesRW1ByMetricName(t, res.Requests[0].RW1, "http_requests_total")
						require.Greater(t, time.Now().Add(-10*time.Minute), ts.Exemplars[0].Timestamp) // More or less scrape time.
					},
				},
			},
		},
	)
}

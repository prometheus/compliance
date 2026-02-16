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

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance/remotewrite/sender/sendertest"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

// TestMetadata validates metric metadata encoding.
func TestMetadata(t *testing.T) {
	sendertest.Run(t,
		targetsToTest,
		sendertest.Case{
			RFCLevel: sendertest.MustLevel,
			// TODO(bwplotka): Add Stateset/gaugehistogram/info tests.
			ScrapeData: `# HELP http_requests_total Total HTTP requests
# TYPE http_requests_total counter
# UNIT http_requests_total seconds
http_requests_total 100
# HELP memory_usage_bytes Current memory usage
# TYPE memory_usage_bytes gauge
# UNIT memory_usage_bytes bytes
memory_usage_bytes 1048576
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="+Inf"} 100
request_duration_seconds_sum 50.0
request_duration_seconds_count 100
# TYPE rpc_duration_seconds summary
rpc_duration_seconds{quantile="0.5"} 0.05
rpc_duration_seconds{quantile="0.9"} 0.1
rpc_duration_seconds_sum 100.0
rpc_duration_seconds_count 1000
`,
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)
				require.Greater(t, len(res.Requests[0].RW2.Timeseries), 9, "Request must contain at least 9 timeseries")
			},
			// https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/#metadata
			ValidateCases: []sendertest.ValidateCase{
				{
					Name:        "type_counter",
					Description: "Sender SHOULD include TYPE metadata for counter metrics",
					RFCLevel:    sendertest.ShouldLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "http_requests_total")
						require.Equal(t, writev2.Metadata_METRIC_TYPE_COUNTER, ts.Metadata.Type,
							"metric should have counter type in metadata")
					},
				},
				{
					Name:        "type_gauge",
					Description: "Sender SHOULD include TYPE metadata for gauge metrics",
					RFCLevel:    sendertest.ShouldLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "memory_usage_bytes")
						require.Equal(t, writev2.Metadata_METRIC_TYPE_GAUGE, ts.Metadata.Type,
							"metric should have gauge type in metadata")
					},
				},
				{
					Name:        "type_histogram",
					Description: "Sender SHOULD include TYPE metadata for histogram metrics",
					RFCLevel:    sendertest.ShouldLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "request_duration_seconds_sum")
						require.Equal(t, writev2.Metadata_METRIC_TYPE_HISTOGRAM, ts.Metadata.Type,
							"metric should have histogram type in metadata")
					},
				},
				{
					Name:        "type_summary",
					Description: "Sender SHOULD include TYPE metadata for summary metrics",
					RFCLevel:    sendertest.ShouldLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "rpc_duration_seconds")
						require.Equal(t, writev2.Metadata_METRIC_TYPE_SUMMARY, ts.Metadata.Type,
							"metric should have summary type in metadata")
					},
				},
				{
					Name:        "help",
					Description: "Sender SHOULD preserve HELP text",
					RFCLevel:    sendertest.ShouldLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "http_requests_total")
						require.Equal(t, "Total HTTP requests", res.Requests[0].RW2.Symbols[ts.Metadata.HelpRef],
							"http_requests_total metric should have help in metadata")
						ts, _ = requireTimeseriesByMetricName(t, res.Requests[0].RW2, "memory_usage_bytes")
						require.Equal(t, "Current memory usage", res.Requests[0].RW2.Symbols[ts.Metadata.HelpRef],
							"memory_usage_bytes metric should have help in metadata")

						ts, _ = requireTimeseriesByMetricName(t, res.Requests[0].RW2, "request_duration_seconds_count")
						require.Empty(t, res.Requests[0].RW2.Symbols[ts.Metadata.HelpRef],
							"request_duration_seconds_count metric should not have help in metadata")
						ts, _ = requireTimeseriesByMetricName(t, res.Requests[0].RW2, "rpc_duration_seconds")
						require.Empty(t, res.Requests[0].RW2.Symbols[ts.Metadata.HelpRef],
							"rpc_duration_seconds metric should not have help in metadata")
					},
				},
				{
					Name:        "unit",
					Description: "Sender MAY include UNIT in metadata",
					RFCLevel:    sendertest.MayLevel,
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "http_requests_total")
						require.Equal(t, "seconds", res.Requests[0].RW2.Symbols[ts.Metadata.UnitRef],
							"http_requests_total metric should have unit in metadata")
						ts, _ = requireTimeseriesByMetricName(t, res.Requests[0].RW2, "memory_usage_bytes")
						require.Equal(t, "bytes", res.Requests[0].RW2.Symbols[ts.Metadata.UnitRef],
							"memory_usage_bytes metric should have unit in metadata")

						ts, _ = requireTimeseriesByMetricName(t, res.Requests[0].RW2, "request_duration_seconds_count")
						require.Empty(t, res.Requests[0].RW2.Symbols[ts.Metadata.UnitRef],
							"request_duration_seconds_count metric should not have unit in metadata")
						ts, _ = requireTimeseriesByMetricName(t, res.Requests[0].RW2, "rpc_duration_seconds")
						require.Empty(t, res.Requests[0].RW2.Symbols[ts.Metadata.UnitRef],
							"rpc_duration_seconds metric should not have unit in metadata")
					},
				},
			},
		},
		// TODO: Metadata are not part of the official 1.0 spec, but we could test it using the recommended level.
		// This requires some longer wait time until we receive metadata periodic batch.
	)
}

func TestMetadataEdgeCases(t *testing.T) {
	sendertest.Run(t, targetsToTest,
		sendertest.Case{
			Name:        "help_with_special_chars",
			Description: "Sender preserves special characters in HELP text",
			RFCLevel:    sendertest.RecommendedLevel, // This depends on UTF-8 feature on scrape, thus recommended level.
			ScrapeData: `# HELP test_metric 🚀 测试 yolo/d
test_metric 42`,
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)

				ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric")
				require.Equal(t, "🚀 测试 yolo/d", res.Requests[0].RW2.Symbols[ts.Metadata.HelpRef],
					"test_metric metric should have help in metadata")
			},
		},
	)
}

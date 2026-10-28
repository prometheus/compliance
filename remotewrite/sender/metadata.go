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
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

func metadataTests(mode Mode) []Test {
	typeLevel := ShouldLevel
	if mode != ModeNone {
		typeLevel = MustLevel
	}

	validateCases := []ValidateCase{
		{
			Name:        "metadata_valid_references",
			Description: "Metadata HelpRef and UnitRef MUST point to valid symbol table indices.",
			RFCLevel:    MustLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				req := res.Requests[0].RW2
				for _, ts := range req.Timeseries {
					require.Less(t, int(ts.Metadata.HelpRef), len(req.Symbols), "HelpRef must be within symbol table bounds")
					require.Less(t, int(ts.Metadata.UnitRef), len(req.Symbols), "UnitRef must be within symbol table bounds")
				}
			},
		},
	}

	if mode != ModeNone {
		validateCases = append(validateCases, ValidateCase{
			Name:        "no_unspecified_metadata_type",
			Description: "Metadata.Type MUST be specified and MUST NOT be METRIC_TYPE_UNSPECIFIED (0) in stateless modes.",
			RFCLevel:    MustLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				req := res.Requests[0].RW2
				for _, ts := range req.Timeseries {
					require.NotEqual(t, writev2.Metadata_METRIC_TYPE_UNSPECIFIED, ts.Metadata.Type,
						"Metadata.Type MUST NOT be METRIC_TYPE_UNSPECIFIED in %s mode", mode)
				}
			},
		})
	}

	validateCases = append(validateCases,
		ValidateCase{
			Name:        "type_counter",
			Description: "Counter series SHOULD include METRIC_TYPE_COUNTER and help text reference (type is MUST in stateless modes).",
			RFCLevel:    typeLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				req := res.Requests[0].RW2
				results := requireTimeseriesByMetricName(t, req, "http_requests_total")
				for _, r := range results {
					if len(r.TimeSeries.Samples) == 0 {
						continue
					}
					require.Equal(t, writev2.Metadata_METRIC_TYPE_COUNTER, r.TimeSeries.Metadata.Type)
					require.Less(t, int(r.TimeSeries.Metadata.HelpRef), len(req.Symbols))
					require.Equal(t, "Total HTTP requests handled (UTF-8: café).", req.Symbols[r.TimeSeries.Metadata.HelpRef])
					require.Zero(t, r.TimeSeries.Metadata.UnitRef)
				}
			},
		},
		ValidateCase{
			Name:        "type_gauge_and_unit",
			Description: "Gauge series with unit SHOULD include METRIC_TYPE_GAUGE, help text reference, and unit reference (type is MUST in stateless modes).",
			RFCLevel:    typeLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				req := res.Requests[0].RW2
				results := requireTimeseriesByMetricName(t, req, "memory_usage_bytes")
				for _, r := range results {
					if len(r.TimeSeries.Samples) == 0 {
						continue
					}
					require.Equal(t, writev2.Metadata_METRIC_TYPE_GAUGE, r.TimeSeries.Metadata.Type)
					require.Less(t, int(r.TimeSeries.Metadata.HelpRef), len(req.Symbols))
					require.Equal(t, "Current memory usage in bytes.", req.Symbols[r.TimeSeries.Metadata.HelpRef])
					require.Less(t, int(r.TimeSeries.Metadata.UnitRef), len(req.Symbols))
					require.Equal(t, "bytes", req.Symbols[r.TimeSeries.Metadata.UnitRef])
				}
			},
		},
		ValidateCase{
			Name:        "type_histogram",
			Description: "Histogram series SHOULD include METRIC_TYPE_HISTOGRAM, help text reference, and unit reference (type is MUST in stateless modes).",
			RFCLevel:    typeLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				req := res.Requests[0].RW2
				results := findTimeseriesByMetricName(req, "req_latency_seconds_count")
				if len(results) == 0 {
					results = requireTimeseriesByMetricName(t, req, "req_latency_seconds")
				}
				for _, r := range results {
					if len(r.TimeSeries.Samples) == 0 && len(r.TimeSeries.Histograms) == 0 {
						continue
					}
					require.Equal(t, writev2.Metadata_METRIC_TYPE_HISTOGRAM, r.TimeSeries.Metadata.Type)
					require.Less(t, int(r.TimeSeries.Metadata.HelpRef), len(req.Symbols))
					require.Equal(t, "Request latency histogram.", req.Symbols[r.TimeSeries.Metadata.HelpRef])
					require.Less(t, int(r.TimeSeries.Metadata.UnitRef), len(req.Symbols))
					require.Equal(t, "seconds", req.Symbols[r.TimeSeries.Metadata.UnitRef])
				}
			},
		},
		ValidateCase{
			Name:        "type_summary",
			Description: "Summary series SHOULD include METRIC_TYPE_SUMMARY and help text reference (type is MUST in stateless modes).",
			RFCLevel:    typeLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				req := res.Requests[0].RW2
				results := requireTimeseriesByMetricName(t, req, "rpc_duration_seconds_count")
				for _, r := range results {
					if len(r.TimeSeries.Samples) == 0 {
						continue
					}
					require.Equal(t, writev2.Metadata_METRIC_TYPE_SUMMARY, r.TimeSeries.Metadata.Type)
					require.Less(t, int(r.TimeSeries.Metadata.HelpRef), len(req.Symbols))
					require.Equal(t, "RPC latency summary.", req.Symbols[r.TimeSeries.Metadata.HelpRef])
				}
			},
		},
		ValidateCase{
			Name:        "type_info_and_stateset",
			Description: "Info and StateSet series SHOULD include METRIC_TYPE_INFO and METRIC_TYPE_STATESET (MUST in stateless modes).",
			RFCLevel:    typeLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				req := res.Requests[0].RW2
				infoResults := requireTimeseriesByMetricName(t, req, "target_build_info")
				for _, r := range infoResults {
					if len(r.TimeSeries.Samples) == 0 {
						continue
					}
					require.Equal(t, writev2.Metadata_METRIC_TYPE_INFO, r.TimeSeries.Metadata.Type)
				}

				stateResults := requireTimeseriesByMetricName(t, req, "feature_state")
				for _, r := range stateResults {
					if len(r.TimeSeries.Samples) == 0 {
						continue
					}
					require.Equal(t, writev2.Metadata_METRIC_TYPE_STATESET, r.TimeSeries.Metadata.Type)
				}
			},
		},
	)

	if mode == ModeNone {
		validateCases = append(validateCases, ValidateCase{
			Name:        "untyped_metadata",
			Description: "Series without TYPE/HELP/UNIT metadata MUST use METRIC_TYPE_UNSPECIFIED and zero references.",
			RFCLevel:    MustLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				req := res.Requests[0].RW2
				results := requireTimeseriesByMetricName(t, req, "untyped_metric_no_meta")
				for _, r := range results {
					require.Equal(t, writev2.Metadata_METRIC_TYPE_UNSPECIFIED, r.TimeSeries.Metadata.Type)
					require.Zero(t, r.TimeSeries.Metadata.HelpRef)
					require.Zero(t, r.TimeSeries.Metadata.UnitRef)
				}
			},
		})
	} else {
		validateCases = append(validateCases, ValidateCase{
			Name:        "untyped_metadata_inferred",
			Description: "In stateless modes, series with unknown metric type SHOULD assume METRIC_TYPE_GAUGE (or METRIC_TYPE_COUNTER for _total) and set help to '(type inferred from untyped)'.",
			RFCLevel:    ShouldLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				req := res.Requests[0].RW2
				results := requireTimeseriesByMetricName(t, req, "untyped_metric_no_meta")
				for _, r := range results {
					require.Equal(t, writev2.Metadata_METRIC_TYPE_GAUGE, r.TimeSeries.Metadata.Type)
					require.Less(t, int(r.TimeSeries.Metadata.HelpRef), len(req.Symbols))
					require.Equal(t, "(type inferred from untyped)", req.Symbols[r.TimeSeries.Metadata.HelpRef])
				}
			},
		})
	}

	validateCases = append(validateCases, ValidateCase{
		Name:        "consistent_family_metadata",
		Description: "Multiple series of the same metric family SHOULD carry identical metadata.",
		RFCLevel:    ShouldLevel,
		Validate: func(t *testing.T, res ReceiverResult) {
			req := res.Requests[0].RW2
			results := requireTimeseriesByMetricName(t, req, "http_requests_total")
			var metas []writev2.Metadata
			for _, r := range results {
				if len(r.TimeSeries.Samples) > 0 {
					metas = append(metas, r.TimeSeries.Metadata)
				}
			}
			require.GreaterOrEqual(t, len(metas), 2, "Expected at least 2 series for http_requests_total")
			for i := 1; i < len(metas); i++ {
				require.Equal(t, metas[0], metas[i], "All series of the same metric family should have identical metadata")
			}
		},
	})

	return []Test{
		{
			Name:        "metadata",
			Description: "Senders SHOULD include metric type, help, and unit metadata for each TimeSeries (type is MUST in stateless modes).",
			RFCLevel:    typeLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `# HELP http_requests Total HTTP requests handled (UTF-8: café).
# TYPE http_requests counter
http_requests_total{method="GET"} 100
http_requests_created{method="GET"} 1000.0
http_requests_total{method="POST"} 25
http_requests_created{method="POST"} 1000.0
# HELP memory_usage_bytes Current memory usage in bytes.
# TYPE memory_usage_bytes gauge
# UNIT memory_usage_bytes bytes
memory_usage_bytes 1048576
# HELP req_latency_seconds Request latency histogram.
# TYPE req_latency_seconds histogram
# UNIT req_latency_seconds seconds
req_latency_seconds_bucket{le="0.5"} 8
req_latency_seconds_bucket{le="+Inf"} 10
req_latency_seconds_sum 2.5
req_latency_seconds_count 10
req_latency_seconds_created 1500.0
# HELP rpc_duration_seconds RPC latency summary.
# TYPE rpc_duration_seconds summary
rpc_duration_seconds{quantile="0.5"} 0.1
rpc_duration_seconds_sum 1.2
rpc_duration_seconds_count 10
rpc_duration_seconds_created 1500.0
# HELP target_build Build metadata info.
# TYPE target_build info
target_build_info{version="2.0.0"} 1
# HELP feature_state Feature state set.
# TYPE feature_state stateset
feature_state{feature_state="enabled"} 1
feature_state{feature_state="disabled"} 0
untyped_metric_no_meta 42
`,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.Len(t, res.Requests, 1)
				require.NotNil(t, res.Requests[0].RW2)
				require.NotEmpty(t, res.Requests[0].RW2.Timeseries)
			},
			ValidateCases: validateCases,
		},
	}
}

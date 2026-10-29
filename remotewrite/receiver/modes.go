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

package receiver

import (
	"math"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/prometheus/model/value"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

func modesTests() []Test {
	var tests []Test

	// Valid Content-Type mode parameters and RFC 9110 parameter syntax variations.
	contentTypeCases := []struct {
		name        string
		metricName  string
		contentType string
		allow415    bool
		description string
	}{
		{
			name:        "default_mode",
			metricName:  "mode_default_requests_total",
			contentType: remote.ContentTypeHeader(remote.WriteV2MessageType, remote.WriteModeNone),
			allow415:    true,
			description: "Default Content-Type without explicit mode parameter",
		},
		{
			name:        "explicit_none_mode",
			metricName:  "mode_none_requests_total",
			contentType: "application/x-protobuf;proto=io.prometheus.write.v2.Request;mode=none",
			allow415:    true,
			description: "Explicit mode=none in Content-Type",
		},
		{
			name:        "stateless_mode",
			metricName:  "mode_stateless_requests_total",
			contentType: remote.ContentTypeHeader(remote.WriteV2MessageType, remote.WriteModeStateless),
			allow415:    true,
			description: "Explicit mode=stateless in Content-Type",
		},
		{
			name:        "stateless_st_mode",
			metricName:  "mode_stateless_st_requests_total",
			contentType: remote.ContentTypeHeader(remote.WriteV2MessageType, remote.WriteModeStatelessST),
			description: "Explicit mode=stateless-st in Content-Type",
		},
		{
			name:        "rfc9110_whitespace",
			metricName:  "mode_whitespace_requests_total",
			contentType: "application/x-protobuf; proto=io.prometheus.write.v2.Request; mode=stateless-st",
			description: "Content-Type with optional RFC 9110 whitespace around parameters",
		},
		{
			name:        "rfc9110_uppercase_params",
			metricName:  "mode_uppercase_requests_total",
			contentType: "application/x-protobuf;PROTO=io.prometheus.write.v2.Request;MODE=stateless-st",
			description: "Content-Type with case-insensitive RFC 9110 parameter names",
		},
		{
			name:        "rfc9110_quoted_values",
			metricName:  "mode_quoted_requests_total",
			contentType: `application/x-protobuf;proto="io.prometheus.write.v2.Request";mode="stateless-st"`,
			description: "Content-Type with quoted RFC 9110 parameter values",
		},
		{
			name:        "rfc9110_reordered_params",
			metricName:  "mode_reordered_requests_total",
			contentType: "application/x-protobuf;mode=stateless-st;proto=io.prometheus.write.v2.Request",
			description: "Content-Type with mode parameter preceding proto parameter",
		},
	}

	for _, tc := range contentTypeCases {
		metricName := tc.metricName
		contentType := tc.contentType
		// A receiver that requires stateless or stateless-st mode may reject none or stateless mode with 415.
		tests = append(tests, complianceTest(
			"modes/content_type/"+tc.name,
			tc.description,
			func() RequestOpts {
				startTime := time.Now().Add(-30 * time.Minute)
				return RequestOpts{
					Samples: []SampleWithLabels{
						{
							Labels:         map[string]string{"__name__": metricName, "job": "api"},
							Value:          42.0,
							StartTimestamp: &startTime,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric(metricName),
							Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
							Help:   "Total requests for mode negotiation test",
							Unit:   "requests",
						},
					},
					ContentType: contentType,
				}
			},
			requestParams{samples: 1, allowAny4xx: tc.allow415},
			true,
		))
	}

	// Stateless mode valid payloads.
	gaugeHist := histogram(5.0, true, false, true, false, false)
	gaugeHist.ResetHint = writev2.Histogram_RESET_HINT_GAUGE

	statelessCases := []struct {
		name        string
		description string
		opts        RequestOpts
		params      requestParams
	}{
		{
			name:        "counter_with_metadata_and_exemplar",
			description: "Stateless mode counter with explicit metadata and attached exemplar",
			opts: RequestOpts{
				Mode: remote.WriteModeStateless,
				Samples: []SampleWithLabels{
					{
						Labels: map[string]string{"__name__": "stateless_http_requests_total", "job": "api", "method": "GET"},
						Value:  120.0,
					},
				},
				Metadata: []MetadataWithLabels{
					{
						Labels: basicMetric("stateless_http_requests_total"),
						Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
						Help:   "Total number of HTTP requests in stateless mode",
						Unit:   "requests",
					},
				},
				Exemplars: []ExemplarWithLabels{
					{
						Labels:         map[string]string{"__name__": "stateless_http_requests_total", "job": "api", "method": "GET"},
						ExemplarLabels: traceExemplar("stateless_counter_trace_1"),
						Value:          1.0,
					},
				},
			},
			params: requestParams{samples: 1, exemplars: 1},
		},
		{
			name:        "gauge_with_metadata",
			description: "Stateless mode gauge with explicit metadata",
			opts: RequestOpts{
				Mode: remote.WriteModeStateless,
				Samples: []SampleWithLabels{
					{
						Labels: map[string]string{"__name__": "stateless_memory_usage_bytes", "instance": "node-1"},
						Value:  1048576.0,
					},
				},
				Metadata: []MetadataWithLabels{
					{
						Labels: basicMetric("stateless_memory_usage_bytes"),
						Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
						Help:   "Current memory usage in bytes",
						Unit:   "bytes",
					},
				},
			},
			params: requestParams{samples: 1},
		},
		{
			name:        "summary_series_with_metadata",
			description: "Stateless mode summary quantile, sum, and count series with explicit SUMMARY metadata",
			opts: RequestOpts{
				Mode: remote.WriteModeStateless,
				Samples: []SampleWithLabels{
					{Labels: map[string]string{"__name__": "stateless_rpc_duration_seconds", "quantile": "0.5"}, Value: 0.05},
					{Labels: map[string]string{"__name__": "stateless_rpc_duration_seconds", "quantile": "0.99"}, Value: 0.25},
					{Labels: basicMetric("stateless_rpc_duration_seconds_sum"), Value: 12.5},
					{Labels: basicMetric("stateless_rpc_duration_seconds_count"), Value: 100.0},
				},
				Metadata: []MetadataWithLabels{
					{
						Labels: basicMetric("stateless_rpc_duration_seconds"),
						Type:   writev2.Metadata_METRIC_TYPE_SUMMARY,
						Help:   "RPC latency summary",
						Unit:   "seconds",
					},
					{
						Labels: basicMetric("stateless_rpc_duration_seconds_sum"),
						Type:   writev2.Metadata_METRIC_TYPE_SUMMARY,
						Help:   "RPC latency summary",
						Unit:   "seconds",
					},
					{
						Labels: basicMetric("stateless_rpc_duration_seconds_count"),
						Type:   writev2.Metadata_METRIC_TYPE_SUMMARY,
						Help:   "RPC latency summary",
						Unit:   "seconds",
					},
				},
			},
			params: requestParams{samples: 4},
		},
		{
			name:        "info_and_stateset_with_metadata",
			description: "Stateless mode info and stateset series with explicit metadata",
			opts: RequestOpts{
				Mode: remote.WriteModeStateless,
				Samples: []SampleWithLabels{
					{Labels: map[string]string{"__name__": "stateless_build_info", "version": "3.0.0"}, Value: 1.0},
					{Labels: map[string]string{"__name__": "stateless_service_state", "state": "ready"}, Value: 1.0},
				},
				Metadata: []MetadataWithLabels{
					{
						Labels: basicMetric("stateless_build_info"),
						Type:   writev2.Metadata_METRIC_TYPE_INFO,
						Help:   "Build information",
					},
					{
						Labels: basicMetric("stateless_service_state"),
						Type:   writev2.Metadata_METRIC_TYPE_STATESET,
						Help:   "Service readiness state",
					},
				},
			},
			params: requestParams{samples: 2},
		},
		{
			name:        "native_histogram_with_metadata_and_exemplar",
			description: "Stateless mode native histogram with explicit metadata and attached exemplar",
			opts: RequestOpts{
				Mode: remote.WriteModeStateless,
				Histograms: []HistogramWithLabels{
					{
						Labels:    map[string]string{"__name__": "stateless_request_duration_seconds", "service": "api"},
						Histogram: histogram(2.5, true, true, true, false, false),
					},
				},
				Metadata: []MetadataWithLabels{
					{
						Labels: basicMetric("stateless_request_duration_seconds"),
						Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
						Help:   "Request duration distribution",
						Unit:   "seconds",
					},
				},
				Exemplars: []ExemplarWithLabels{
					{
						Labels:         map[string]string{"__name__": "stateless_request_duration_seconds", "service": "api"},
						ExemplarLabels: traceExemplar("stateless_hist_trace_1"),
						Value:          0.42,
					},
				},
			},
			params: requestParams{histograms: 1, exemplars: 1},
		},
		{
			name:        "nhcb_custom_buckets_histogram_with_metadata",
			description: "Stateless mode NHCB custom buckets histogram with explicit metadata and exemplar",
			opts: RequestOpts{
				Mode: remote.WriteModeStateless,
				Histograms: []HistogramWithLabels{
					{
						Labels:    map[string]string{"__name__": "stateless_classic_latency_seconds", "service": "frontend"},
						Histogram: histogram(15.2, false, false, false, true, false),
					},
				},
				Metadata: []MetadataWithLabels{
					{
						Labels: basicMetric("stateless_classic_latency_seconds"),
						Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
						Help:   "Classic histogram encoded as NHCB in stateless mode",
						Unit:   "seconds",
					},
				},
				Exemplars: []ExemplarWithLabels{
					{
						Labels:         map[string]string{"__name__": "stateless_classic_latency_seconds", "service": "frontend"},
						ExemplarLabels: traceExemplar("stateless_nhcb_trace_1"),
						Value:          0.25,
					},
				},
			},
			params: requestParams{histograms: 1, exemplars: 1},
		},
		{
			name:        "gauge_histogram_with_metadata",
			description: "Stateless mode gauge histogram with explicit GAUGEHISTOGRAM metadata",
			opts: RequestOpts{
				Mode: remote.WriteModeStateless,
				Histograms: []HistogramWithLabels{
					{
						Labels:    map[string]string{"__name__": "stateless_queue_depth_histogram", "queue": "ingest"},
						Histogram: gaugeHist,
					},
				},
				Metadata: []MetadataWithLabels{
					{
						Labels: basicMetric("stateless_queue_depth_histogram"),
						Type:   writev2.Metadata_METRIC_TYPE_GAUGEHISTOGRAM,
						Help:   "Queue depth gauge histogram",
					},
				},
			},
			params: requestParams{histograms: 1},
		},
	}

	for _, sc := range statelessCases {
		tests = append(tests, staticComplianceTest("modes/stateless/"+sc.name, sc.description, sc.opts, sc.params, true))
	}

	// Stateless-ST mode valid payloads.
	equalToSampleTime := time.Time{}
	statelessSTCases := []struct {
		name        string
		description string
		buildOpts   func() RequestOpts
		params      requestParams
	}{
		{
			name:        "counter_with_start_timestamp",
			description: "Stateless-ST mode counter with non-zero start_timestamp, metadata, and exemplar",
			buildOpts: func() RequestOpts {
				startTime := time.Now().Add(-1 * time.Hour)
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Samples: []SampleWithLabels{
						{
							Labels:         map[string]string{"__name__": "stateless_st_http_requests_total", "job": "api"},
							Value:          500.0,
							StartTimestamp: &startTime,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_http_requests_total"),
							Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
							Help:   "Total HTTP requests in stateless-st mode",
							Unit:   "requests",
						},
					},
					Exemplars: []ExemplarWithLabels{
						{
							Labels:         map[string]string{"__name__": "stateless_st_http_requests_total", "job": "api"},
							ExemplarLabels: traceExemplar("stateless_st_counter_trace"),
							Value:          1.0,
						},
					},
				}
			},
			params: requestParams{samples: 1, exemplars: 1},
		},
		{
			name:        "counter_with_equal_start_and_sample_timestamp",
			description: "Stateless-ST mode counter where start_timestamp equals timestamp",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Samples: []SampleWithLabels{
						{
							Labels:         map[string]string{"__name__": "stateless_st_equal_st_total", "job": "api"},
							Value:          1.0,
							StartTimestamp: &equalToSampleTime,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_equal_st_total"),
							Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
							Help:   "Counter where start_timestamp equals timestamp",
							Unit:   "requests",
						},
					},
				}
			},
			params: requestParams{samples: 1},
		},
		{
			name:        "counter_staleness_marker_without_start_timestamp",
			description: "Stateless-ST mode counter staleness marker without start_timestamp",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Samples: []SampleWithLabels{
						{
							Labels: map[string]string{"__name__": "stateless_st_stale_counter_total", "job": "api"},
							Value:  math.Float64frombits(value.StaleNaN),
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_stale_counter_total"),
							Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
							Help:   "Stale counter marker in stateless-st mode",
						},
					},
				}
			},
			params: requestParams{samples: 1},
		},
		{
			name:        "gauge_without_start_timestamp",
			description: "Stateless-ST mode gauge without start_timestamp",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Samples: []SampleWithLabels{
						{
							Labels: map[string]string{"__name__": "stateless_st_cpu_temperature_celsius", "core": "0"},
							Value:  64.5,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_cpu_temperature_celsius"),
							Type:   writev2.Metadata_METRIC_TYPE_GAUGE,
							Help:   "CPU core temperature in Celsius",
							Unit:   "celsius",
						},
					},
				}
			},
			params: requestParams{samples: 1},
		},
		{
			name:        "summary_with_start_timestamp_on_sum_and_count",
			description: "Stateless-ST mode summary with start_timestamp on _sum and _count and no start_timestamp on quantile",
			buildOpts: func() RequestOpts {
				startTime := time.Now().Add(-1 * time.Hour)
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Samples: []SampleWithLabels{
						{
							Labels: map[string]string{"__name__": "stateless_st_rpc_duration_seconds", "quantile": "0.9"},
							Value:  0.18,
						},
						{
							Labels:         basicMetric("stateless_st_rpc_duration_seconds_sum"),
							Value:          45.2,
							StartTimestamp: &startTime,
						},
						{
							Labels:         basicMetric("stateless_st_rpc_duration_seconds_count"),
							Value:          300.0,
							StartTimestamp: &startTime,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_rpc_duration_seconds"),
							Type:   writev2.Metadata_METRIC_TYPE_SUMMARY,
							Help:   "RPC latency summary in stateless-st mode",
							Unit:   "seconds",
						},
						{
							Labels: basicMetric("stateless_st_rpc_duration_seconds_sum"),
							Type:   writev2.Metadata_METRIC_TYPE_SUMMARY,
							Help:   "RPC latency summary in stateless-st mode",
							Unit:   "seconds",
						},
						{
							Labels: basicMetric("stateless_st_rpc_duration_seconds_count"),
							Type:   writev2.Metadata_METRIC_TYPE_SUMMARY,
							Help:   "RPC latency summary in stateless-st mode",
							Unit:   "seconds",
						},
					},
				}
			},
			params: requestParams{samples: 3},
		},
		{
			name:        "native_histogram_with_start_timestamp",
			description: "Stateless-ST mode native histogram with start_timestamp, metadata, and exemplar",
			buildOpts: func() RequestOpts {
				startTime := time.Now().Add(-1 * time.Hour)
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Histograms: []HistogramWithLabels{
						{
							Labels:         map[string]string{"__name__": "stateless_st_request_duration_seconds", "service": "checkout"},
							Histogram:      histogram(4.2, true, true, true, false, false),
							StartTimestamp: &startTime,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_request_duration_seconds"),
							Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
							Help:   "Request duration native histogram in stateless-st mode",
							Unit:   "seconds",
						},
					},
					Exemplars: []ExemplarWithLabels{
						{
							Labels:         map[string]string{"__name__": "stateless_st_request_duration_seconds", "service": "checkout"},
							ExemplarLabels: traceExemplar("stateless_st_hist_trace"),
							Value:          0.35,
						},
					},
				}
			},
			params: requestParams{histograms: 1, exemplars: 1},
		},
		{
			name:        "nhcb_histogram_with_start_timestamp",
			description: "Stateless-ST mode NHCB histogram with start_timestamp and metadata",
			buildOpts: func() RequestOpts {
				startTime := time.Now().Add(-1 * time.Hour)
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Histograms: []HistogramWithLabels{
						{
							Labels:         map[string]string{"__name__": "stateless_st_classic_latency_seconds", "service": "checkout"},
							Histogram:      histogram(21.4, false, false, false, true, false),
							StartTimestamp: &startTime,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_classic_latency_seconds"),
							Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
							Help:   "Classic histogram encoded as NHCB with start_timestamp in stateless-st mode",
							Unit:   "seconds",
						},
					},
				}
			},
			params: requestParams{histograms: 1},
		},
		{
			name:        "float_histogram_with_start_timestamp",
			description: "Stateless-ST mode float histogram with start_timestamp and metadata",
			buildOpts: func() RequestOpts {
				startTime := time.Now().Add(-1 * time.Hour)
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Histograms: []HistogramWithLabels{
						{
							Labels:         map[string]string{"__name__": "stateless_st_float_duration_seconds", "service": "checkout"},
							Histogram:      floatHistogram(8.5, true, true, true, false, false),
							StartTimestamp: &startTime,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_float_duration_seconds"),
							Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
							Help:   "Float native histogram with start_timestamp in stateless-st mode",
							Unit:   "seconds",
						},
					},
				}
			},
			params: requestParams{histograms: 1},
		},
		{
			name:        "native_histogram_with_equal_start_and_sample_timestamp",
			description: "Stateless-ST mode native histogram where start_timestamp equals timestamp",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Histograms: []HistogramWithLabels{
						{
							Labels:         map[string]string{"__name__": "stateless_st_equal_st_duration_seconds", "service": "checkout"},
							Histogram:      histogram(1.5, true, false, true, false, false),
							StartTimestamp: &equalToSampleTime,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_equal_st_duration_seconds"),
							Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
							Help:   "Histogram where start_timestamp equals timestamp",
							Unit:   "seconds",
						},
					},
				}
			},
			params: requestParams{histograms: 1},
		},
	}

	for _, sc := range statelessSTCases {
		tests = append(tests, complianceTest("modes/stateless_st/"+sc.name, sc.description, sc.buildOpts, sc.params, true))
	}

	// Mode violation cases (SHOULD reject with 400).
	validationCases := []struct {
		name        string
		description string
		buildOpts   func() RequestOpts
	}{
		{
			name:        "stateless_metadata_only_timeseries",
			description: "Metadata-only TimeSeries in stateless mode should be rejected with 400",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					Mode: remote.WriteModeStateless,
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_orphan_metadata_metric"),
							Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
							Help:   "Orphaned metadata in stateless mode",
						},
					},
					UnsafeRequest: true,
				}
			},
		},
		{
			name:        "stateless_exemplar_only_timeseries",
			description: "Exemplar-only TimeSeries without samples or histograms in stateless mode should be rejected with 400",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					Mode: remote.WriteModeStateless,
					Exemplars: []ExemplarWithLabels{
						{
							Labels:         basicMetric("stateless_orphan_exemplar_metric"),
							ExemplarLabels: traceExemplar("orphan_trace_stateless"),
							Value:          1.0,
						},
					},
					UnsafeRequest: true,
				}
			},
		},
		{
			name:        "stateless_classic_histogram_samples",
			description: "Classic histogram float samples with METRIC_TYPE_HISTOGRAM in stateless mode should be rejected with 400",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					Mode: remote.WriteModeStateless,
					Samples: []SampleWithLabels{
						{
							Labels: map[string]string{"__name__": "stateless_classic_hist_bucket", "le": "1.0"},
							Value:  5.0,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_classic_hist_bucket"),
							Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
							Help:   "Classic histogram float bucket sample in stateless mode",
						},
					},
				}
			},
		},
		{
			name:        "stateless_untyped_metadata",
			description: "TimeSeries with unspecified Metadata.type in stateless mode should be rejected with 400",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					Mode: remote.WriteModeStateless,
					Samples: []SampleWithLabels{
						{
							Labels: basicMetric("stateless_untyped_metric"),
							Value:  1.0,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_untyped_metric"),
							Type:   writev2.Metadata_METRIC_TYPE_UNSPECIFIED,
							Help:   "Untyped metric in stateless mode",
						},
					},
				}
			},
		},
		{
			name:        "stateless_st_metadata_only_timeseries",
			description: "Metadata-only TimeSeries in stateless-st mode should be rejected with 400",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_orphan_metadata_metric"),
							Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
							Help:   "Orphaned metadata in stateless-st mode",
						},
					},
					UnsafeRequest: true,
				}
			},
		},
		{
			name:        "stateless_st_exemplar_only_timeseries",
			description: "Exemplar-only TimeSeries without samples or histograms in stateless-st mode should be rejected with 400",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Exemplars: []ExemplarWithLabels{
						{
							Labels:         basicMetric("stateless_st_orphan_exemplar_metric"),
							ExemplarLabels: traceExemplar("orphan_trace_stateless_st"),
							Value:          1.0,
						},
					},
					UnsafeRequest: true,
				}
			},
		},
		{
			name:        "stateless_st_classic_histogram_samples",
			description: "Classic histogram float samples with METRIC_TYPE_HISTOGRAM in stateless-st mode should be rejected with 400",
			buildOpts: func() RequestOpts {
				startTime := time.Now().Add(-30 * time.Minute)
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Samples: []SampleWithLabels{
						{
							Labels:         map[string]string{"__name__": "stateless_st_classic_hist_bucket", "le": "1.0"},
							Value:          5.0,
							StartTimestamp: &startTime,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_classic_hist_bucket"),
							Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
							Help:   "Classic histogram float bucket sample in stateless-st mode",
						},
					},
				}
			},
		},
		{
			name:        "stateless_st_untyped_metadata",
			description: "TimeSeries with unspecified Metadata.type in stateless-st mode should be rejected with 400",
			buildOpts: func() RequestOpts {
				startTime := time.Now().Add(-30 * time.Minute)
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Samples: []SampleWithLabels{
						{
							Labels:         basicMetric("stateless_st_untyped_metric"),
							Value:          1.0,
							StartTimestamp: &startTime,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_untyped_metric"),
							Type:   writev2.Metadata_METRIC_TYPE_UNSPECIFIED,
							Help:   "Untyped metric in stateless-st mode",
						},
					},
				}
			},
		},
		{
			name:        "stateless_st_counter_without_start_timestamp",
			description: "Counter sample without start_timestamp in stateless-st mode should be rejected with 400",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Samples: []SampleWithLabels{
						{
							Labels: basicMetric("stateless_st_missing_st_counter_total"),
							Value:  10.0,
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_missing_st_counter_total"),
							Type:   writev2.Metadata_METRIC_TYPE_COUNTER,
							Help:   "Counter sample missing start_timestamp in stateless-st mode",
						},
					},
				}
			},
		},
		{
			name:        "stateless_st_histogram_without_start_timestamp",
			description: "Native histogram without start_timestamp in stateless-st mode should be rejected with 400",
			buildOpts: func() RequestOpts {
				return RequestOpts{
					Mode: remote.WriteModeStatelessST,
					Histograms: []HistogramWithLabels{
						{
							Labels:    basicMetric("stateless_st_missing_st_histogram_seconds"),
							Histogram: histogram(15.0, true, false, false, false, false),
						},
					},
					Metadata: []MetadataWithLabels{
						{
							Labels: basicMetric("stateless_st_missing_st_histogram_seconds"),
							Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
							Help:   "Native histogram missing start_timestamp in stateless-st mode",
						},
					},
				}
			},
		},
	}

	for _, vc := range validationCases {
		buildOpts := vc.buildOpts
		sampleOpts := buildOpts()
		tests = append(tests, singleRequestTest(
			"modes/validation/"+vc.name,
			vc.description,
			ShouldLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				return generateRequest(buildOpts(), externalLabels)
			},
			requestParams{
				samples:    len(sampleOpts.Samples),
				histograms: len(sampleOpts.Histograms),
				exemplars:  len(sampleOpts.Exemplars),
				success:    false,
			},
		))
	}

	return tests
}

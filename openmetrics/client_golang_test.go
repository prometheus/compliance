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

package openmetrics_test

import (
	"context"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/compliance/openmetrics"
)

type clientGolang struct{}

func (clientGolang) Name() string { return "client_golang" }

// Run starts an in-process HTTP server instrumented with prometheus/client_golang
// exposing the canonical OpenMetrics 1.0 and 2.0 compliance metric families.
func (clientGolang) Run(ctx context.Context, opts openmetrics.Options) error {
	reg := prometheus.NewRegistry()

	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "app_build_info",
		Help: "Application build information.",
	}, []string{"version", "sdk"})
	requests := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total number of HTTP requests handled.",
	}, []string{"method", "status"})
	requestBytes := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "http_request_size_bytes_total",
		Help: "Total HTTP request bytes received.",
		Unit: "bytes",
	})
	// Dotted OpenTelemetry-style metric and label names that require UTF-8 quoting
	// under allow-utf-8 or translation under underscores/dots/values escaping.
	inFlight := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "http.server.active_requests",
		Help: "Current number of in-flight HTTP requests (OTel-style dotted name).",
	}, []string{"service.name"})
	// Histogram configured with both classic and native buckets.
	requestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:                        "http_request_duration_seconds",
		Help:                        "HTTP request duration in seconds.",
		Unit:                        "seconds",
		Buckets:                     []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5},
		NativeHistogramBucketFactor: 1.1,
	}, []string{"method"})
	rpcLatency := prometheus.NewSummary(prometheus.SummaryOpts{
		Name:       "rpc_latency_seconds",
		Help:       "RPC latency summary in seconds.",
		Unit:       "seconds",
		Objectives: map[float64]float64{0.5: 0.01, 0.9: 0.01, 0.99: 0.001},
	})

	reg.MustRegister(buildInfo, requests, requestBytes, inFlight, requestDuration, rpcLatency)

	exemplar := prometheus.Labels{
		"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
		"span_id":  "00f067aa0ba902b7",
	}
	// Combined exemplar label runes > 128: truncated in OpenMetrics 1.0, preserved in OpenMetrics 2.0.
	extendedExemplar := prometheus.Labels{
		"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
		"span_id":  "00f067aa0ba902b7",
		"attr_big": strings.Repeat("a", 80),
	}

	buildInfo.WithLabelValues("1.0.0", "client_golang").Set(1)
	requests.WithLabelValues("GET", "200").(prometheus.ExemplarAdder).AddWithExemplar(5, exemplar)
	requests.WithLabelValues("POST", "201").Add(2)
	requestBytes.(prometheus.ExemplarAdder).AddWithExemplar(4096, extendedExemplar)
	inFlight.WithLabelValues("demo-api").Set(3)
	requestDuration.WithLabelValues("GET").(prometheus.ExemplarObserver).ObserveWithExemplar(0.042, exemplar)
	requestDuration.WithLabelValues("GET").Observe(0.18)
	requestDuration.WithLabelValues("GET").Observe(0.73)
	rpcLatency.Observe(0.015)
	rpcLatency.Observe(0.085)

	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		EnableOpenMetrics:                   true,
		EnableOpenMetricsTextCreatedSamples: true,
		AcceptedFormats: []expfmt.Format{
			expfmt.FmtOpenMetrics_2_0_0,
			expfmt.FmtOpenMetrics_2_0_0_rc,
			expfmt.FmtOpenMetrics_1_0_0,
			expfmt.FmtOpenMetrics_0_0_1,
			expfmt.FmtText,
		},
	})

	return openmetrics.NewHandlerTarget("client_golang", handler).Run(ctx, opts)
}

var _ openmetrics.Target = clientGolang{}

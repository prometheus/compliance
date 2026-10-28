# OpenMetrics Compliance Suite

This package provides a programmatic Go compliance test suite for metric producers (client SDKs, instrumentation libraries, and `/metrics` HTTP exporters) implementing the [OpenMetrics 1.0](https://prometheus.io/docs/specs/om/open_metrics_spec/) and [OpenMetrics 2.0](https://prometheus.io/docs/specs/om/open_metrics_2_0_spec/) specifications.

Each compliance check is tagged with its RFC 2119 requirement level (`MUST`, `SHOULD`, `MAY`, `RECOMMENDED`) and scrapes an HTTP `/metrics` endpoint with controlled `Accept` and `Accept-Encoding` headers, verifying HTTP negotiation, framing, ABNF syntax, metric metadata, label escaping, sample types, classic and native histograms, and exemplars.

## Running Tests

To run the compliance suite against the in-process `prometheus/client_golang` reference target:

```bash
make test
```

To run the tests and open the interactive HTML compliance matrix in your browser:

```bash
make test-web
```

## Programmatic Usage in Go

You can import `github.com/prometheus/compliance/openmetrics` directly in your own Go test suite to test any `http.Handler` or external `/metrics` process.

### Testing an `http.Handler` in-process

```go
package myexporter_test

import (
	"testing"

	"github.com/prometheus/compliance/openmetrics"
)

func TestOpenMetricsCompliance(t *testing.T) {
	handler := newMetricsHandler()
	target := openmetrics.NewHandlerTarget("my-exporter", handler)
	openmetrics.RunTests(t, target, openmetrics.ComplianceTests())
}
```

You can also run only a specific specification version:

- `openmetrics.ComplianceTests()` — runs both OpenMetrics 1.0 and OpenMetrics 2.0 tests.
- `openmetrics.V1ComplianceTests()` — runs only OpenMetrics 1.0 (`1.0.0`) tests.
- `openmetrics.V2ComplianceTests()` — runs only OpenMetrics 2.0 (`2.0.0` / `2.0.0-rc`) tests.

### Testing an external binary or container

Implement `openmetrics.Target` (optionally using `openmetrics.RunCommand` to manage the child process lifecycle until `ctx` is canceled):

```go
type binaryTarget struct{}

func (binaryTarget) Name() string { return "my-binary" }

func (binaryTarget) Run(ctx context.Context, opts openmetrics.Options) error {
	// Start your binary or container listening on opts.MetricsHostPort and serving opts.MetricsPath:
	return openmetrics.RunCommand(ctx, ".", nil, "./my-exporter", "--web.listen-address="+opts.MetricsHostPort, "--web.telemetry-path="+opts.MetricsPath)
}
```

## Expected Canonical Metric Set

While the `protocol` and `syntax` test categories validate any non-empty OpenMetrics exposition, the `metadata`, `escaping`, `samples`, `histograms`, and `exemplars` categories inspect specific canonical metric families when verifying SDK instrumentation features. Reference targets should expose:

- `app_build_info` (`gauge` with value `1` and labels such as `version` and `sdk`)
- `http_requests_total` (`counter` with no unit, labels `method="GET"` and `status="200"` with value `>= 5`, start timestamp, and an exemplar carrying `trace_id="4bf92f3577b34da6a3ce929d0e0e4736"` and `span_id="00f067aa0ba902b7"`)
- `http_request_size_bytes_total` (`counter` with unit `bytes` and an exemplar carrying `trace_id="4bf92f3577b34da6a3ce929d0e0e4736"`, `span_id="00f067aa0ba902b7"`, and an extended attribute `attr_big` exceeding 128 total UTF-8 runes to exercise OpenMetrics 1.0 128-rune truncation and OpenMetrics 2.0 extended exemplar length)
- `http.server.active_requests` (`gauge` with value `3`, dots in the metric name, and UTF-8 label `service.name="demo-api"` to exercise `escaping=allow-utf-8`, `underscores`, `dots`, and `values`)
- `http_request_duration_seconds` (`histogram` with unit `seconds`, label `method="GET"`, classic buckets `0.01`..`2.5`, native histogram buckets, start timestamp, and an exemplar carrying `trace_id="4bf92f3577b34da6a3ce929d0e0e4736"`)
- `rpc_latency_seconds` (`summary` with unit `seconds` and quantiles `0.5`, `0.9`, `0.99`)

See [`client_golang_test.go`](client_golang_test.go) for the reference implementation.

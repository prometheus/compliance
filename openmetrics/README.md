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

Implement `openmetrics.Target` (optionally using `openmetrics.RunCommand` to manage the child process lifecycle and pass the bound `opts.ScrapeTarget` URL to `scrapeFn`):

```go
type binaryTarget struct{}

func (binaryTarget) Run(t *testing.T, opts openmetrics.Options, scrapeFn func(opts openmetrics.Options)) {
	// Start your binary or container, wait until ready, and pass its /metrics URL:
	opts.ScrapeTarget = "http://127.0.0.1:9100/metrics"
	scrapeFn(opts)
}
```

## Expected Canonical Metric Set

While the `protocol` and `syntax` test categories validate any non-empty OpenMetrics exposition, the `metadata`, `escaping`, `samples`, `histograms`, and `exemplars` categories inspect specific canonical metric families when verifying SDK instrumentation features. Reference targets should expose:

- `app_build_info` (`info` or `gauge` with value `1` and labels such as `version` and `goversion`)
- `http_requests_total` (`counter` with unit `requests`, label `code`, start timestamp, and an exemplar carrying a long attribute value >= 130 UTF-8 runes to exercise OpenMetrics 1.0 128-rune truncation and OpenMetrics 2.0 extended exemplar length)
- `http_request_size_bytes_total` (`counter` with unit `bytes` and label `method`)
- `http.server.active_requests` (`gauge` with dots in the metric name and a UTF-8 label name `http.request.method` to exercise `escaping=allow-utf-8`, `underscores`, `dots`, and `values`)
- `http_request_duration_seconds` (`histogram` with unit `seconds`, classic buckets `0.005`..`10`, native histogram buckets, start timestamp, and bucket exemplars)
- `rpc_latency_seconds` (`summary` with unit `seconds`, quantiles `0.5`, `0.9`, `0.99`, and start timestamp)

See [`client_golang_test.go`](client_golang_test.go) for the reference implementation.

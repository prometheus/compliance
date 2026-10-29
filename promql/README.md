# PromQL Compliance Test Suite

This directory contains the compliance test suite for PromQL implementations. It validates query results from a tested target against a reference Prometheus server over a deterministic 70-minute synthetic dataset replayed via fast timestamped scrapes.

The test suite shares [`../index.html`](../index.html) to view compliance test results in the browser after a test run completes.

## How It Works

1. An in-process scrape replayer serves 174 deterministic synthetic snapshots covering `[end-70m, end]` (`2m` step over `[end-70m, end-12m)` and `5s` step over `[end-12m, end]`) with explicit timestamps.
2. Both the reference Prometheus server and the target under test scrape the replayer at a fast `15ms` interval (`honor_timestamps: true`, `honor_labels: true`), ingesting the full 70-minute dataset in ~2.5 seconds.
3. All expanded PromQL test cases from [`promql-test-queries.yml`](./promql-test-queries.yml) are executed in parallel `go test` subtests and compared against the reference Prometheus query results.

Tests cover:
- Scalar literals, vector selectors, offsets, and staleness gaps
- Aggregation operators (`sum`, `avg`, `max`, `min`, `count`, `stddev`, `stdvar`, `topk`, `bottomk`, `quantile`, `count_values`, `group`)
- Arithmetic, comparison, and vector-matching binary operators (`on`, `ignoring`, `group_left`, `group_right`, `bool`)
- Unary expressions, `NaN`/`Inf` handling, and operator precedence
- Range and instant rate functions (`rate`, `increase`, `delta`, `irate`, `idelta`, `resets`, `changes`, `deriv`, `predict_linear`)
- Over-time aggregations (`*_over_time`) and subqueries
- Math, clamping, date/time, label manipulation (`label_replace`, `label_join`), and `absent` functions
- Classic and Native Histogram functions (`histogram_quantile`, `histogram_fraction`, `histogram_count`, `histogram_sum`, `histogram_avg`, `histogram_stddev`, `histogram_stdvar`)

## Prerequisites

- Go 1.25 or later

By default, the test suite automatically downloads and runs the pinned Prometheus 3.x release binary into `./bin/prometheus`.

## Running Tests

```bash
make test
```

To run the tests and open the HTML report viewer on `http://localhost:8080`:

```bash
make test-web
```

### Environment Variables

- `PROMETHEUS_COMPLIANCE_PROMQL_BINARY=<path>`: Override the Prometheus binary path used by `TestCompliance` instead of downloading the default release binary.
- `DEBUG="1"`: Stream target process stdout/stderr and disable panic recovery wrapping during test execution.

## Programmatic Usage in External Repositories

Projects written in Go can import `github.com/prometheus/compliance/promql` and implement the `promql.Target` interface to run the compliance suite directly from their own `go test` suites:

```go
package myproject_test

import (
	"testing"

	"github.com/prometheus/compliance/promql"
)

func TestPromQLCompliance(t *testing.T) {
	promql.RunTests(t, refTarget{}, myTarget{}, promql.ComplianceTests())
}
```

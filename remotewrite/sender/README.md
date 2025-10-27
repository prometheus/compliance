# Prometheus Remote-Write 2.0 Sender Compliance Test Suite

This repository contains a comprehensive compliance test suite for Prometheus Remote-Write Protocol **senders**. It validates that sender implementations properly comply with the [Remote-Write 2.0 specification](https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/).

## Overview

The test suite validates sender compliance across **4 comprehensive phases**:

1. **Foundation & Protocol** - HTTP protocol, headers, compression, protobuf encoding, symbol tables
2. **Data Correctness & Encoding** - Samples, histograms, exemplars, metadata, labels, timestamps
3. **Behavior & Reliability** - Retry logic, backoff, batching, error handling, response processing
4. **Backward Compatibility & Edge Cases** - RW 1.0 compatibility, version fallback, edge case handling

**Test Coverage:**
- ✅ **161+ test cases** across 17 test files
- ✅ **98% spec coverage** of official Remote Write 2.0 requirements
- ✅ **All RFC compliance levels**: MUST (95%), SHOULD (100%), MAY (100%)

## Quick Start

The test suite **automatically downloads** and configures sender binaries for you:

```bash
# Run tests against ALL senders (downloads binaries automatically)
make test

# Test specific senders only
make test-prometheus      # Test Prometheus only
make test-grafana         # Test Grafana Agent only
make test-otel            # Test OpenTelemetry Collector only
make test-vmagent         # Test VictoriaMetrics Agent only
```

**Supported Senders:**
- `prometheus` - Prometheus v3.7.1
- `grafana_agent` - Grafana Agent v0.19.0 (coming soon)
- `otelcollector` - OpenTelemetry Collector (coming soon)
- `vmagent` - VictoriaMetrics Agent (coming soon)
- `telegraf` - Telegraf (coming soon)
- `vector` - Vector (coming soon)

Binaries are automatically downloaded to `bin/` and cached for subsequent runs.

## Running Tests

### Basic Usage

```bash
# Run all tests with auto-download (recommended)
make test

# Run tests against specific sender
make test-prometheus
make test-grafana

# Generate HTML results report
make results

# Run specific test by name
make test-run TEST=TestProtocolCompliance
```

### Advanced Usage

```bash
# Test only specific senders (comma-separated)
PROMETHEUS_RW2_COMPLIANCE_SENDERS="prometheus,grafana_agent" go test -v

# Filter tests by pattern
go test -v -run TestHistograms

# Increase timeout for slow environments
PROMETHEUS_RW2_COMPLIANCE_TEST_TIMEOUT=60s make test

# Run with coverage report
make coverage
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PROMETHEUS_RW2_COMPLIANCE_SENDERS` | Filter which senders to test (comma-separated) | All registered senders |
| `PROMETHEUS_RW2_COMPLIANCE_TEST_TIMEOUT` | Timeout for each test | `2m` |

## HTML Test Results

The test suite includes an interactive HTML results viewer:

```bash
# Generate test results in JSON format
make results

# Or manually:
go test -json | tee results.json
```

Then open `index.html` in your browser to view:
- ✅ Test matrix with pass/fail status per sender
- 📊 Summary statistics and progress bars
- 🔍 Detailed test output and assertions
- 🏷️ RFC compliance levels (MUST/SHOULD/MAY)
- 🔗 Cross-references between related tests

## Test Coverage

Our test suite provides **comprehensive coverage** of the Remote Write 2.0 specification:

### Phase 1: Foundation & Protocol
- `protocol_test.go` - HTTP method, headers, compression, protobuf encoding (9 tests)
- `symbols_test.go` - Symbol table structure and deduplication (5 tests)

### Phase 2: Data Correctness & Encoding
- `samples_test.go` - Float sample encoding, special values (16 tests)
- `histograms_test.go` - Native and classic histogram encoding (12 tests)
- `exemplars_test.go` - Exemplar attachment with trace IDs (9 tests)
- `metadata_test.go` - TYPE, HELP, UNIT metadata (11 tests)
- `labels_test.go` - Label format, ordering, validation (13 tests)
- `timestamps_test.go` - Timestamp format, created_timestamp (8 tests)
- `combined_test.go` - Integration of multiple features (8 tests)

### Phase 3: Behavior & Reliability
- `retry_test.go` - Retry behavior on 4xx/5xx errors (10 tests)
- `backoff_test.go` - Exponential backoff validation (5 tests)
- `batching_test.go` - Batching and flushing strategies (7 tests)
- `error_handling_test.go` - Network errors, timeouts (10 tests)
- `response_test.go` - Response header processing (9 tests)

### Phase 4: Compatibility & Edge Cases
- `rw1_compat_test.go` - Remote Write 1.0 backward compatibility (9 tests)
- `fallback_test.go` - Version fallback on 415 responses (6 tests)
- `edge_cases_test.go` - Edge cases and stress testing (14 tests)

**Total:** 161+ test cases

See `COVERAGE_CHECKLIST.md` for detailed mapping to specification requirements.

## Architecture

### Test Infrastructure

```
┌─────────────────────────────────────────────────┐
│  Test Runner (main_test.go)                     │
│  - Registered targets (prometheus, etc)         │
│  - Iterates through test cases                  │
│  - Manages test execution                       │
└─────────────┬───────────────────────────────────┘
              │
              ▼
      ┌─────────────────┐
      │ Auto Targets    │
      │ (targets/*.go)  │
      │ - Download bins │
      │ - Generate cfg  │
      └────────┬────────┘
               │
               ▼
      ┌─────────────────┐
      │ Mock Receiver   │ ◄──────┐
      │ (Captures       │        │
      │  requests)      │        │
      └─────────────────┘        │
                                 │
      ┌─────────────────┐        │
      │ Mock Scrape     │        │
      │ Target          │        │
      │ (Serves metrics)│        │
      └────────┬────────┘        │
               │                 │
               │                 │
               ▼                 │
      ┌─────────────────┐        │
      │ Sender Instance │────────┘
      │ (Auto-started)  │ Sends
      └─────────────────┘ Remote Write
```

### Key Components

- **`helpers_test.go`** - Mock receiver, mock scrape target, assertion helpers
- **`main_test.go`** - Test framework, sender lifecycle management
- **`targets/*.go`** - Automatic binary downloading and configuration
- **Test files** - Individual test suites for each specification area

## Development

### Prerequisites

- Go 1.25.0 or later
- Make (optional, for convenience commands)

### Building

```bash
# Install dependencies
make deps

# Build test binary
make build

# Run linter
make lint
```

### Adding Tests

1. Create a new test file (e.g., `my_feature_test.go`)
2. Use `forEachSender` pattern to test all registered senders
3. Write test scenario with validator function
4. Set RFC compliance level: `t.Attr("rfcLevel", "MUST")`

Example:
```go
package main

import "testing"

func TestMyFeature(t *testing.T) {
    t.Attr("rfcLevel", "MUST")
    t.Attr("description", "Sender MUST support my feature")

    forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
        runSenderTest(t, targetName, target, SenderTestScenario{
            ScrapeData: "test_metric 42\n",
            Validator: func(t *testing.T, req *CapturedRequest) {
                must(t).NotNil(req.Request)
                // Add your assertions here
            },
        })
    })
}
```

## Troubleshooting

### Tests are skipped

```
=== SKIP TestProtocolCompliance/prometheus
    No auto targets found matching "xyz"
```

**Solution:** Check the sender name is correct. Available senders: prometheus, grafana_agent, otelcollector, vmagent, telegraf, vector.

### Timeout errors

```
Test timed out after 2m
```

**Solution:** Increase timeout:
```bash
PROMETHEUS_RW2_COMPLIANCE_TEST_TIMEOUT=10m make test
```

### Download failures

```
Error downloading: 404
```

**Solution:** Check your internet connection. Some targets may require specific OS/architecture combinations.

### No requests captured

```
Expected at least 1 request(s), got 0
```

**Solution:**
- Increase wait time in test scenario
- Check sender logs for startup errors
- Verify sender configuration template is correct

## Contributing

Contributions are welcome! Please:

1. Run tests before submitting: `make test-all`
2. Add tests for new features
3. Update documentation
4. Follow Go coding conventions

## License

Apache License 2.0 - See LICENSE file for details

## Resources

- [Remote Write 2.0 Specification](https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/)
- [Prometheus Documentation](https://prometheus.io/docs/)
- [CNCF Prometheus Conformance Program](https://github.com/cncf/prometheus-conformance)

---

**Questions?** Open an issue in the [prometheus/compliance](https://github.com/prometheus/compliance) repository.

# Remote Write 2.0 Sender Compliance Tests

> **Status:** Proposal & Design Phase
> **Last Updated:** 2025-10-26

This directory contains compliance tests for validating Prometheus Remote Write 2.0 **sender** implementations against the [official specification](https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/).

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Test Structure](#test-structure)
- [Implementation Phases](#implementation-phases)
  - [Phase 1: Foundation & Protocol](#phase-1-foundation--protocol)
  - [Phase 2: Data Correctness & Encoding](#phase-2-data-correctness--encoding)
  - [Phase 3: Behavior & Reliability](#phase-3-behavior--reliability)
  - [Phase 4: Backward Compatibility & Edge Cases](#phase-4-backward-compatibility--edge-cases)
- [Usage](#usage)
- [Configuration](#configuration)
- [Test Coverage Summary](#test-coverage-summary)
- [Design Decisions](#design-decisions)

---

## Overview

### Purpose

This test suite validates that Remote Write sender implementations (Prometheus, Grafana Agent, OpenTelemetry Collector, etc.) correctly implement the Remote Write 2.0 specification for **sending** metrics to remote endpoints.

### Key Differences from Receiver Tests

The sender tests **invert** the architecture of receiver tests:

| Aspect | Receiver Tests | Sender Tests |
|--------|---------------|--------------|
| **Role** | HTTP Client | HTTP Server (Mock Receiver) |
| **Request Flow** | Generate → Send → Validate Response | Capture → Parse → Validate Request |
| **Instance** | External receivers | Fork/launch sender processes |
| **Focus** | Response correctness | Request correctness |
| **Validation** | Status codes, headers | Protobuf structure, protocol compliance |

### Specification Coverage

Tests cover all sender requirements from the Remote Write 2.0 specification:

- ✅ **MUST requirements** - Mandatory compliance
- ✅ **SHOULD requirements** - Recommended behavior
- ✅ **MAY requirements** - Optional features
- ✅ **Edge cases** - Robustness validation
- ✅ **Backward compatibility** - RW 1.0 support

---

## Architecture

### Core Components

```
┌─────────────────┐
│  Test Framework │
│  (Go Test)      │
└────────┬────────┘
         │
         ├──► Mock HTTP Receiver (captures requests)
         │    └─► Request Parser & Validator
         │
         ├──► Mock Scrape Target (provides metrics)
         │    └─► OpenMetrics/Prometheus format
         │
         └──► Sender Instance Launcher
              └─► Manages sender process lifecycle
```

### Test Flow

```
1. Start Mock HTTP Receiver (test server)
2. Start Mock Scrape Target (provides metrics)
3. Launch Sender Instance (binary + config)
4. Wait for sender to scrape and send
5. Capture request(s) at mock receiver
6. Parse and validate protobuf structure
7. Validate protocol compliance
8. Stop sender instance
9. Assert test expectations
```

### Directory Structure (Planned)

```
/remotewrite/sender/
├── README.md                 # This file
├── go.mod                    # Go module definition
├── go.sum                    # Dependencies
│
├── main_test.go              # Test framework, sender launcher, config management
├── helpers_test.go           # Mock server, request capture, validation utilities
│
├── protocol_test.go          # HTTP protocol compliance (headers, encoding)
├── symbols_test.go           # Symbol table structure and optimization
├── samples_test.go           # Float sample encoding and ordering
├── histograms_test.go        # Native histogram encoding
├── exemplars_test.go         # Exemplar attachment and encoding
├── metadata_test.go          # Metadata structure and types
├── labels_test.go            # Label validation and special cases
├── timestamps_test.go        # Timestamp ordering and created_timestamp
├── combined_test.go          # Multi-feature integration tests
│
├── retry_test.go             # Retry behavior on errors
├── backoff_test.go           # Exponential backoff validation
├── batching_test.go          # Request batching behavior
├── error_handling_test.go    # 4xx/5xx response handling
├── response_test.go          # Response header processing
│
├── rw1_compat_test.go        # Backward compatibility with RW 1.x
├── fallback_test.go          # Content-type fallback on 415
├── edge_cases_test.go        # Edge cases and robustness
│
├── config_example.yml        # Example sender configurations
├── testdata/                 # Test fixtures
│   ├── scrapes/              # Scrape data samples
│   │   ├── simple_counter.txt
│   │   ├── simple_gauge.txt
│   │   ├── native_histogram.txt
│   │   ├── with_exemplars.txt
│   │   └── ...
│   └── configs/              # Sender config templates
│       ├── prometheus_template.yml
│       ├── grafana_agent_template.yml
│       └── otel_template.yml
└── bin/                      # Sender binaries (gitignored)
    ├── prometheus
    ├── agent
    └── ...
```

---

## Test Structure

### Test File Organization

Tests are organized by **feature area** rather than by sender:

- **Protocol tests** - HTTP headers, encoding, content-type
- **Data structure tests** - Protobuf encoding, symbol tables, labels
- **Metric type tests** - Samples, histograms, exemplars, metadata
- **Behavior tests** - Retry logic, backoff, batching, error handling
- **Compatibility tests** - RW 1.0 fallback, version negotiation

### Test Pattern

Each test follows this pattern:

```go
func TestFeature(t *testing.T) {
    tests := []struct {
        name        string
        description string
        rfcLevel    string  // "MUST", "SHOULD", "MAY"
        scrapeData  string
        validator   func(*testing.T, *CapturedRequest)
    }{
        {
            name: "test_case_name",
            description: "Human-readable description of requirement",
            rfcLevel: "MUST",
            scrapeData: "test_metric 42\n",
            validator: func(t *testing.T, req *CapturedRequest) {
                must(t).Equal(expectedValue, actualValue)
            },
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Attr("rfcLevel", tt.rfcLevel)
            t.Attr("description", tt.description)

            forEachSender(t, func(t *testing.T, sender SenderConfig) {
                runSenderTest(t, SenderTestScenario{
                    ScrapeData: tt.scrapeData,
                    Validator: tt.validator,
                })
            })
        })
    }
}
```

### RFC Compliance Markers

Tests use helper functions to mark compliance levels:

```go
must(t).Equal(...)    // RFC MUST level - test failure
should(t).Equal(...)  // RFC SHOULD level - warning
may(t).Equal(...)     // RFC MAY level - informational
```

---

## Implementation Phases

### Phase 1: Foundation & Protocol

**Goal:** Establish test framework, mock server infrastructure, and validate basic HTTP protocol requirements.

**Deliverables:**
- Test framework with sender config loading
- Mock HTTP receiver with request capture
- Sender process launcher and lifecycle management
- Mock scrape target for feeding data
- HTTP protocol compliance tests
- Symbol table structure tests

**Files:**
- `main_test.go` - Test framework and orchestration
- `helpers_test.go` - Mock server, request capture, validators
- `protocol_test.go` - HTTP protocol compliance (8 test cases)
- `symbols_test.go` - Symbol table validation (4 test cases)

**Test Cases:** 12+

#### Protocol Tests (`protocol_test.go`)

| Test | Requirement | RFC Level |
|------|-------------|-----------|
| `content_type_protobuf` | Content-Type: application/x-protobuf | MUST |
| `content_type_with_proto_param` | Include proto parameter for RW 2.0 | SHOULD |
| `content_encoding_snappy` | Content-Encoding: snappy | MUST |
| `version_header_present` | X-Prometheus-Remote-Write-Version present | MUST |
| `version_header_value` | Version 2.0.0 for RW 2.0 receivers | SHOULD |
| `user_agent_present` | User-Agent header (RFC 9110) | MUST |
| `snappy_block_format` | Snappy block format (not framed) | MUST |
| `protobuf_parseable` | Valid protobuf message | MUST |

#### Symbol Table Tests (`symbols_test.go`)

| Test | Requirement | RFC Level |
|------|-------------|-----------|
| `empty_string_at_index_zero` | Symbols[0] must be empty string | MUST |
| `string_deduplication` | Deduplicate repeated strings | MUST |
| `labels_refs_valid_indices` | All refs must be valid symbol indices | MUST |
| `labels_refs_even_length` | labels_refs length must be even | MUST |

---

### Phase 2: Data Correctness & Encoding

**Goal:** Validate that senders correctly encode samples, histograms, exemplars, metadata, labels, and timestamps.

**Deliverables:**
- Sample encoding tests (15+ test cases)
- Histogram encoding tests (12+ test cases)
- Exemplar tests (8+ test cases)
- Metadata tests (10+ test cases)
- Label validation tests (12+ test cases)
- Timestamp tests (6+ test cases)
- Integration tests (8+ test cases)

**Files:**
- `samples_test.go`
- `histograms_test.go`
- `exemplars_test.go`
- `metadata_test.go`
- `labels_test.go`
- `timestamps_test.go`
- `combined_test.go`

**Test Cases:** 71+

#### Sample Encoding Tests (`samples_test.go`)

| Test Category | Example Tests | Count |
|--------------|---------------|-------|
| Basic encoding | Float values, int values, zero | 3 |
| Special values | NaN, +Inf, -Inf, StaleNaN | 4 |
| Timestamps | Milliseconds format, ordering | 2 |
| Created timestamp | Counter created_timestamp | 2 |
| Label completeness | Full labelset, job/instance | 2 |
| Edge cases | Empty values, huge values | 2 |

#### Histogram Tests (`histograms_test.go`)

| Test Category | Example Tests | Count |
|--------------|---------------|-------|
| Basic encoding | Native histogram structure | 2 |
| Bucket configurations | Positive, negative, zero buckets | 3 |
| Schema variations | Different schemas | 2 |
| Ordering | Timestamp ordering | 1 |
| Separation | No mixed samples/histograms | 1 |
| Created timestamp | Histogram created_timestamp | 1 |
| Edge cases | Empty histograms, large counts | 2 |

#### Exemplar Tests (`exemplars_test.go`)

| Test Category | Example Tests | Count |
|--------------|---------------|-------|
| Basic attachment | Exemplars on samples/histograms | 2 |
| Trace labels | trace_id, span_id labels | 2 |
| Custom labels | Non-trace use cases | 2 |
| Edge cases | Empty labels, many labels | 2 |

#### Metadata Tests (`metadata_test.go`)

| Test Category | Example Tests | Count |
|--------------|---------------|-------|
| Basic metadata | Type, help, unit fields | 3 |
| All metric types | Counter, gauge, histogram, summary | 4 |
| Special characters | Newlines in help, unicode | 2 |
| Edge cases | Missing fields, empty values | 1 |

#### Label Validation Tests (`labels_test.go`)

| Test Category | Example Tests | Count |
|--------------|---------------|-------|
| Ordering | Lexicographic sorting | 1 |
| Required labels | __name__ label present | 1 |
| Name format | Metric name regex | 1 |
| Label format | Label name regex | 1 |
| Duplicates | No repeated label names | 1 |
| Empty values | No empty names/values | 2 |
| Reserved names | __ prefix handling | 1 |
| Special characters | Unicode, spaces, dots | 2 |
| Edge cases | Very long labels, many labels | 2 |

#### Timestamp Tests (`timestamps_test.go`)

| Test Category | Example Tests | Count |
|--------------|---------------|-------|
| Format | Int64 milliseconds | 1 |
| Ordering | Older samples first | 1 |
| Created timestamp | Counter/histogram created_timestamp | 2 |
| Zero handling | Created timestamp zero value | 1 |
| Edge cases | Very old/new timestamps | 1 |

#### Integration Tests (`combined_test.go`)

| Test Category | Example Tests | Count |
|--------------|---------------|-------|
| Multi-feature | Samples + metadata + exemplars | 2 |
| Multi-type | Counter, gauge, histogram, summary | 2 |
| Large requests | Many timeseries | 2 |
| Real-world | Complex production-like data | 2 |

---

### Phase 3: Behavior & Reliability

**Goal:** Validate sender retry logic, backoff behavior, batching, error handling, and response processing.

**Deliverables:**
- Retry behavior tests (10+ test cases)
- Backoff validation tests (5+ test cases)
- Batching behavior tests (6+ test cases)
- Error handling tests (8+ test cases)
- Response processing tests (6+ test cases)

**Files:**
- `retry_test.go`
- `backoff_test.go`
- `batching_test.go`
- `error_handling_test.go`
- `response_test.go`

**Test Cases:** 35+

#### Retry Behavior Tests (`retry_test.go`)

| Test | Requirement | RFC Level |
|------|-------------|-----------|
| `no_retry_on_4xx` | Don't retry on 4xx errors | MUST |
| `retry_on_500` | Retry on 500 Internal Server Error | MUST |
| `retry_on_503` | Retry on 503 Service Unavailable | MUST |
| `retry_on_all_5xx` | Retry on all 5xx errors | MUST |
| `may_retry_on_429` | May retry on 429 Too Many Requests | MAY |
| `no_retry_on_400` | Don't retry on 400 Bad Request | MUST |
| `no_retry_on_401` | Don't retry on 401 Unauthorized | MUST |
| `no_retry_on_404` | Don't retry on 404 Not Found | MUST |
| `retry_after_header` | Honor Retry-After header | MAY |
| `eventual_success` | Succeed after transient failures | SHOULD |

#### Backoff Tests (`backoff_test.go`)

| Test | Requirement | RFC Level |
|------|-------------|-----------|
| `exponential_backoff` | Use backoff algorithm | MUST |
| `increasing_delays` | Delays increase over time | SHOULD |
| `backoff_max_delay` | Reasonable maximum delay | SHOULD |
| `backoff_with_jitter` | May use jitter | MAY |
| `backoff_reset_on_success` | Reset after success | SHOULD |

#### Batching Tests (`batching_test.go`)

| Test | Requirement | RFC Level |
|------|-------------|-----------|
| `multiple_series_per_request` | Batch multiple series | SHOULD |
| `parallel_requests_supported` | Can send parallel requests | MAY |
| `queue_capacity` | Respect queue limits | SHOULD |
| `flush_on_interval` | Time-based flushing | SHOULD |
| `flush_on_size` | Size-based flushing | SHOULD |
| `queue_full_behavior` | Handle queue full gracefully | SHOULD |

#### Error Handling Tests (`error_handling_test.go`)

| Test | Scenario | Expected Behavior |
|------|----------|-------------------|
| `network_timeout` | Connection timeout | Retry with backoff |
| `connection_refused` | Connection refused | Retry with backoff |
| `connection_reset` | Connection reset | Retry with backoff |
| `partial_write` | Partial HTTP write | Handle gracefully |
| `malformed_response` | Invalid HTTP response | Handle gracefully |
| `dns_failure` | DNS resolution failure | Retry with backoff |
| `tls_error` | TLS handshake failure | Handle gracefully |
| `keep_running` | Sender stays alive on errors | Should not crash |

#### Response Processing Tests (`response_test.go`)

| Test | Requirement | RFC Level |
|------|-------------|-----------|
| `ignore_response_body_on_success` | Ignore 2xx response body | SHOULD |
| `use_written_count_headers` | May use X-Prometheus-Remote-Write-*-Written | MAY |
| `missing_headers_on_2xx` | Assume 415 if 2xx with no headers (RW 2.0) | SHOULD |
| `assume_zero_written` | Missing headers = 0 written (RW 2.0) | SHOULD |
| `log_error_messages` | Log errors as-is without interpretation | MUST |
| `handle_large_response_body` | Handle large error messages | SHOULD |

---

### Phase 4: Backward Compatibility & Edge Cases

**Goal:** Validate RW 1.0 backward compatibility, content-type fallback, and edge case handling.

**Deliverables:**
- RW 1.0 compatibility tests (8+ test cases)
- Content-type fallback tests (5+ test cases)
- Edge case tests (10+ test cases)
- Performance reference tests (5+ test cases, optional)

**Files:**
- `rw1_compat_test.go`
- `fallback_test.go`
- `edge_cases_test.go`
- `performance_test.go` (optional)

**Test Cases:** 28+

#### RW 1.0 Compatibility Tests (`rw1_compat_test.go`)

| Test | Requirement | RFC Level |
|------|-------------|-----------|
| `send_rw1_format` | Support RW 1.0 format when configured | MUST |
| `rw1_version_header` | Use version 0.1.0 for RW 1.0 | SHOULD |
| `rw1_content_type` | Use basic content-type for RW 1.0 | SHOULD |
| `rw1_protobuf_structure` | Correct prometheus.WriteRequest format | MUST |
| `rw1_samples_encoding` | Encode samples in RW 1.0 format | MUST |
| `rw1_labels_encoding` | Encode labels in RW 1.0 format | MUST |
| `rw1_no_histograms` | RW 1.0 doesn't support native histograms | N/A |
| `rw1_no_created_timestamp` | RW 1.0 doesn't support created_timestamp | N/A |

#### Fallback Tests (`fallback_test.go`)

| Test | Requirement | RFC Level |
|------|-------------|-----------|
| `fallback_on_415` | Fallback to RW 1.0 on 415 | SHOULD |
| `retry_with_different_version` | Retry with version 0.1.0 | SHOULD |
| `remember_fallback_choice` | Remember successful fallback | SHOULD |
| `fallback_header_changes` | Change headers on fallback | MUST |
| `fallback_format_changes` | Change protobuf format on fallback | MUST |

#### Edge Case Tests (`edge_cases_test.go`)

| Test | Scenario | Expected Behavior |
|------|----------|-------------------|
| `empty_scrape` | No metrics scraped | Handle gracefully |
| `huge_label_values` | Very large label values (10KB+) | Handle without crash |
| `unicode_in_labels` | Emoji, non-ASCII characters | Preserve correctly |
| `many_timeseries` | 10,000+ timeseries | Handle efficiently |
| `high_cardinality` | 1,000+ unique label values | Symbol table optimization |
| `very_long_metric_name` | 1KB+ metric name | Handle or reject gracefully |
| `special_float_combinations` | NaN with exemplars, etc. | Handle correctly |
| `zero_timestamp` | Timestamp exactly 0 | Handle correctly |
| `future_timestamp` | Timestamp in future | Handle correctly |
| `concurrent_scrapes` | Multiple scrapes simultaneously | Handle correctly |

#### Performance Tests (`performance_test.go`)

*Optional, informational only - not for compliance*

| Test | Measurement |
|------|-------------|
| `throughput_1000_series` | Samples/sec with 1000 series |
| `throughput_10000_series` | Samples/sec with 10,000 series |
| `memory_usage` | Peak memory under load |
| `cpu_usage` | CPU usage during sending |
| `compression_ratio` | Symbol table compression effectiveness |

---

## Usage

### Prerequisites

1. **Go 1.23+** installed
2. **Sender binary** to test (e.g., Prometheus, Grafana Agent)
3. Configuration file specifying sender(s) to test

### Running Tests

```bash
# Run all tests for all configured senders
go test -v ./...

# Run tests for specific sender
export PROMETHEUS_RW2_COMPLIANCE_SENDERS=prometheus
go test -v ./...

# Run specific test file
go test -v -run TestProtocolCompliance ./...

# Run tests with custom config
export PROMETHEUS_RW2_COMPLIANCE_SENDER_CONFIG_FILE=/path/to/config.yml
go test -v ./...

# Run with verbose output and timeout
go test -v -timeout 30m ./...

# Run only MUST-level tests (skip SHOULD/MAY)
# TODO: Implement filtering by RFC level
```

### Test Output

Tests produce detailed output with:
- RFC compliance level (MUST/SHOULD/MAY)
- Test description
- Sender name being tested
- Pass/fail status
- Detailed error messages on failure

Example output:
```
=== RUN   TestProtocolCompliance
=== RUN   TestProtocolCompliance/content_type_protobuf
=== RUN   TestProtocolCompliance/content_type_protobuf/prometheus
    protocol_test.go:45: [MUST] Sender MUST use Content-Type: application/x-protobuf
    protocol_test.go:45: ✓ PASS
=== RUN   TestProtocolCompliance/version_header_value
=== RUN   TestProtocolCompliance/version_header_value/prometheus
    protocol_test.go:78: [SHOULD] Sender SHOULD use version 2.0.0 for RW 2.0 receivers
    protocol_test.go:80: ⚠ WARNING: Version should be 2.0.x, got: 1.0.0
--- PASS: TestProtocolCompliance (5.23s)
```

---

## Configuration

### Sender Configuration File

Create a YAML configuration file (e.g., `config.yml`) specifying senders to test:

```yaml
senders:
  - name: prometheus
    binary: ./bin/prometheus
    config_template: |
      global:
        scrape_interval: 1s
        evaluation_interval: 1s
      scrape_configs:
        - job_name: 'test'
          static_configs:
            - targets: ['{{.ScrapeTarget}}']
      remote_write:
        - url: {{.RemoteWriteURL}}
          queue_config:
            max_shards: 1
            capacity: 500
            batch_send_deadline: 1s
    ready_check:
      type: http
      url: http://localhost:9090/-/ready
      timeout: 30s
    ports:
      http: 9090

  - name: grafana-agent
    binary: ./bin/agent
    config_template: |
      prometheus:
        configs:
          - name: test
            scrape_configs:
              - job_name: test
                scrape_interval: 1s
                static_configs:
                  - targets: ['{{.ScrapeTarget}}']
            remote_write:
              - url: {{.RemoteWriteURL}}
    ready_check:
      type: log
      pattern: "Remote write client started"
      timeout: 10s

  - name: otel-collector
    binary: ./bin/otelcol
    config_template: |
      receivers:
        prometheus:
          config:
            scrape_configs:
              - job_name: test
                scrape_interval: 1s
                static_configs:
                  - targets: ['{{.ScrapeTarget}}']
      exporters:
        prometheusremotewrite:
          endpoint: {{.RemoteWriteURL}}
      service:
        pipelines:
          metrics:
            receivers: [prometheus]
            exporters: [prometheusremotewrite]
    ready_check:
      type: http
      url: http://localhost:13133/
      timeout: 30s
```

### Configuration Fields

| Field | Description | Required |
|-------|-------------|----------|
| `name` | Unique sender identifier | Yes |
| `binary` | Path to sender binary | Yes |
| `config_template` | Go template for sender config | Yes |
| `ready_check.type` | How to check if sender is ready (`http` or `log`) | Yes |
| `ready_check.url` | HTTP endpoint for readiness (if type=http) | Conditional |
| `ready_check.pattern` | Log pattern to match (if type=log) | Conditional |
| `ready_check.timeout` | Max wait time for readiness | Yes |
| `ports.http` | HTTP port for sender (if needed) | No |

### Template Variables

The `config_template` supports these variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `{{.RemoteWriteURL}}` | Mock receiver URL | `http://localhost:54321/receive` |
| `{{.ScrapeTarget}}` | Mock scrape target URL | `http://localhost:54322/metrics` |

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PROMETHEUS_RW2_COMPLIANCE_SENDER_CONFIG_FILE` | Path to sender config | `config.yml` or `config_example.yml` |
| `PROMETHEUS_RW2_COMPLIANCE_SENDERS` | Comma-separated sender names to test | All senders |
| `PROMETHEUS_RW2_COMPLIANCE_TEST_TIMEOUT` | Default test timeout | `2m` |
| `PROMETHEUS_RW2_COMPLIANCE_SCRAPE_INTERVAL` | Scrape interval for tests | `1s` |
| `PROMETHEUS_RW2_COMPLIANCE_VERBOSE` | Enable verbose logging | `false` |

---

## Test Coverage Summary

### Total Statistics

| Phase | Files | Test Cases | Estimated LOC |
|-------|-------|-----------|---------------|
| Phase 1: Foundation & Protocol | 4 | 12+ | ~800 |
| Phase 2: Data Correctness | 7 | 71+ | ~1000 |
| Phase 3: Behavior & Reliability | 5 | 35+ | ~600 |
| Phase 4: Compatibility & Edge Cases | 4 | 28+ | ~300 |
| **Total** | **18** | **146+** | **~2700** |

### Specification Coverage

| Compliance Level | Coverage |
|-----------------|----------|
| **MUST** requirements | 100% |
| **SHOULD** requirements | 100% |
| **MAY** requirements | ~80% |
| Edge cases | Comprehensive |
| Error scenarios | Comprehensive |

### Test Distribution by Category

| Category | Test Count | Percentage |
|----------|-----------|------------|
| Data Encoding & Structure | 71 | 49% |
| Behavior & Reliability | 35 | 24% |
| Compatibility & Edge Cases | 28 | 19% |
| Protocol & Foundation | 12 | 8% |

---

## Design Decisions

### Architecture Choices

#### 1. Mock HTTP Server vs. Real Receiver

**Decision:** Use mock HTTP server
**Rationale:**
- Full control over response codes, delays, headers
- Can simulate error conditions (timeouts, partial responses)
- Faster test execution (no real receiver startup)
- Easier to validate exact request structure

#### 2. Process Forking vs. Docker Containers

**Decision:** Fork sender processes (Phase 1), Docker support later
**Rationale:**
- Simpler initial implementation
- Faster startup/teardown
- Easier debugging (logs, process inspection)
- Docker support can be added in later phases

#### 3. Request Validation vs. Response Validation

**Decision:** Validate incoming requests at mock server
**Rationale:**
- This is a sender test - validate what sender sends
- Mirrors receiver test pattern (but inverted)
- Direct inspection of protobuf structure

#### 4. Multi-Scrape Test Infrastructure

**Decision:** Support multiple scrape cycles in single test
**Rationale:**
- Some tests require state changes (stale markers, timestamp ordering)
- Allows testing retry/backoff behavior
- More realistic test scenarios

### Reused Components from Receiver Tests

**Directly Reused:**
- RFC compliance markers (`must()`, `should()`, `may()`)
- Protobuf structures (`writev2.Request`, `writev2.TimeSeries`)
- Symbol table parsing utilities
- Label extraction helpers
- Test attribute system (`t.Attr()`)
- Configuration loading patterns
- Table-driven test patterns

**Adapted/Inverted:**
- Request generation → Request validation
- HTTP client → HTTP server
- Response validation → Request validation
- External endpoint config → Sender binary config

**New Components:**
- Mock HTTP receiver with request capture
- Sender process launcher and lifecycle management
- Mock scrape target (OpenMetrics/Prometheus format)
- Timing validation for retry/backoff tests
- Multi-scrape test orchestration

### Test Organization Philosophy

**Feature-based, not sender-based:**
- Tests organized by specification requirement
- Each test runs against all configured senders
- Easier to identify specification gaps
- Clearer compliance reporting

**Table-driven patterns:**
- Reduces code duplication
- Easier to add new test cases
- Consistent test structure
- Clear test case documentation

**Explicit RFC levels:**
- Every test marked with MUST/SHOULD/MAY
- Different assertion behavior per level
- Clear compliance expectations

---

## Roadmap

### Phase 1: Foundation (Week 1-2)
- [ ] Test framework implementation
- [ ] Mock HTTP receiver
- [ ] Sender process launcher
- [ ] Mock scrape target
- [ ] Protocol compliance tests
- [ ] Symbol table tests

### Phase 2: Data Correctness (Week 3-4)
- [ ] Sample encoding tests
- [ ] Histogram encoding tests
- [ ] Exemplar tests
- [ ] Metadata tests
- [ ] Label validation tests
- [ ] Timestamp tests
- [ ] Integration tests

### Phase 3: Behavior (Week 5-6)
- [ ] Retry behavior tests
- [ ] Backoff validation tests
- [ ] Batching behavior tests
- [ ] Error handling tests
- [ ] Response processing tests

### Phase 4: Compatibility (Week 7)
- [ ] RW 1.0 compatibility tests
- [ ] Fallback tests
- [ ] Edge case tests
- [ ] Performance tests (optional)

### Post-Implementation
- [ ] Documentation
- [ ] CI/CD integration
- [ ] Docker support for senders
- [ ] Test result reporting
- [ ] Compliance badge generation

---

## Contributing

### Adding New Tests

1. Identify specification requirement
2. Determine appropriate test file
3. Add test case to table-driven test
4. Mark with RFC compliance level
5. Add description and validator
6. Update this README

### Test Case Template

```go
{
    name: "test_case_name",
    description: "Sender [MUST|SHOULD|MAY] do something specific",
    rfcLevel: "MUST", // or "SHOULD", "MAY"
    scrapeData: "test_metric 42\n",
    validator: func(t *testing.T, req *CapturedRequest) {
        // Validate request structure
        must(t).Equal(expected, actual, "failure message")
    },
}
```

### Adding New Senders

1. Add sender binary to `bin/` directory
2. Create config template in `config.yml`
3. Define ready check mechanism
4. Run tests: `go test -v ./...`

---

## FAQ

**Q: How long does the full test suite take?**
A: Approximately 10-15 minutes per sender, depending on retry/backoff tests.

**Q: Can I run tests in parallel?**
A: Yes, but senders are tested sequentially by default to avoid port conflicts.

**Q: What if my sender doesn't support RW 2.0 yet?**
A: Tests will show which features are missing. Use for development tracking.

**Q: Can I test against a real receiver instead of mock?**
A: Not currently supported, but could be added. Mock provides more control.

**Q: How do I test custom sender implementations?**
A: Add binary and config to `config.yml`, then run tests normally.

**Q: Do tests validate data correctness?**
A: No - tests validate **protocol compliance**, not data accuracy.

---

## References

- [Remote Write 2.0 Specification](https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/)
- [Prometheus Remote Write 1.0 Spec](https://prometheus.io/docs/concepts/remote_write_spec/)
- [OpenMetrics Specification](https://openmetrics.io/)
- [Prometheus Exposition Formats](https://prometheus.io/docs/instrumenting/exposition_formats/)

---

## License

Apache License 2.0 - See repository root for details.

# Prometheus Remote-Write v2 Sender Compliance Test Suite

This repository contains a compliance test suite for Prometheus Remote-Write Protocol senders. It validates that Remote-Write senders properly implement the Remote-Write v2 specification according to the [official protocol requirements](https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/).

## Overview

The test suite forks sender instances (e.g., Prometheus with remote-write enabled), examines the requests they generate, and validates them against the protocol specification. It tests proper implementation of:

- Float samples encoding
- Native Histograms encoding
- Exemplars encoding
- Protocol headers and content negotiation
- Error handling and retry logic
- Backoff and batching behavior
- Metadata and symbol table management
- Request formatting and compression

## Limitations

The test suite validates the format and structure of requests sent by the sender but does not verify end-to-end data flow or persistence. Tests examine:

- Request payload structure and encoding
- Protocol compliance of generated requests
- Proper header usage and content negotiation
- Correct retry and backoff behavior when receivers respond with errors

Because senders may have different configuration options and capabilities, passing all tests does not guarantee a sender supports every Remote-Write feature (such as Native Histograms). Some senders may not expose certain features or may require specific configuration.

You should review the detailed test output to judge compliance for your sender. A successful `go test` run demonstrates the sender correctly encodes and sends Remote-Write v2 requests when configured to do so.

## Prerequisites

- Go 1.23 or later
- The sender binary to test (e.g., Prometheus)

The test suite automatically downloads and runs Prometheus as the reference sender implementation. For testing custom senders, place the binary in the `bin/` directory.

## Configuration

The test suite uses environment variables for configuration:

### Sender Selection

You can specify which sender to test using the `PROMETHEUS_RW2_COMPLIANCE_SENDERS` environment variable:

```bash
export PROMETHEUS_RW2_COMPLIANCE_SENDERS="prometheus"
go test -v
```

Currently supported senders:
- `prometheus` - The reference Prometheus implementation (automatically downloaded)

### Test Timeout

You can override the default test timeout using:

```bash
export PROMETHEUS_RW2_COMPLIANCE_TEST_TIMEOUT="10m"
go test -v
```

## Running Tests

The test suite automatically sets up mock receiver endpoints and forks sender instances. You can run tests using standard Go test commands:

```bash
# Run all compliance tests
go test -v -timeout 5m

# Run tests for a specific area
go test -v -run TestHistograms
go test -v -run TestExemplars

# Run specific test case
go test -v -run "TestExemplarEncoding/exemplar_with_trace_id"

# Run tests with detailed output
go test -v -count=1

# Run tests against specific sender only
PROMETHEUS_RW2_COMPLIANCE_SENDERS="prometheus" go test -v
```

## HTML visualisation

This repository contains a `index.html` that enables viewing the results of the compliance tests.

It loads a `results.json` that can be generated with:

```bash
go test -json | tee results.json
```

## Protocol Compliance Levels

Tests are marked with compliance levels:
- **MUST**: Required by specification
- **SHOULD**: Recommended by specification

Use `t.Attr("rfcLevel", "MUST")` or `t.Attr("rfcLevel", "SHOULD")` to identify compliance levels.

## Request Validation

The test suite validates sender behavior by examining:
- Request payload structure and encoding (protobuf format)
- Required request headers (Content-Type, Content-Encoding, etc.)
- Proper compression (snappy)
- Correct retry behavior on receiver errors
- Backoff strategy implementation
- Batching and queueing behavior
- Metadata and symbol handling

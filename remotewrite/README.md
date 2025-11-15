# Prometheus Remote-Write v2 Compliance Test Suite

This repository contains compliance test suites for both **senders** and **receivers** of the Prometheus Remote-Write Protocol 2.0. It validates implementation of the Remote-Write v2 specification according to the [official protocol requirements](https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/) as well as some more strict Prometheus implementation aspects.

## Overview

The test suites validate protocol compliance for both sides of Remote-Write communication:

### Sender Compliance (`/sender`)
Tests that Remote-Write senders properly implement the protocol by forking sender instances (e.g., Prometheus), examining generated requests, and validating them against the specification.

Tests cover:
- Float samples encoding
- Native Histograms encoding
- Exemplars encoding
- Protocol headers and content negotiation
- Error handling and retry logic
- Backoff and batching behavior
- Metadata and symbol table management
- Request formatting and compression

### Receiver Compliance (`/receiver`)
Tests that Remote-Write endpoints properly handle incoming requests by sending various Remote-Write v2 requests and validating responses.

Tests cover:
- Float samples handling
- Native Histograms handling
- Exemplars handling
- Protocol headers and response codes
- Error conditions and edge cases
- Content-Type validation
- Response headers (`X-Prometheus-Remote-Write-*-Written`)

## Limitations

### Sender Tests
The test suite validates the format and structure of requests sent by the sender but does not verify end-to-end data flow or persistence. Because senders may have different configuration options and capabilities, passing all tests does not guarantee a sender supports every Remote-Write feature (such as Native Histograms). Some senders may not expose certain features or may require specific configuration.

### Receiver Tests
This test suite does not verify data ingestion by reading data back from the receiver. Some requests that are valid for one backend might be rejected by another. The suite tolerates both 200 and 400 series HTTP responses since actual data validation is up to the receiver. Therefore, passing all tests does not guarantee that a receiver supports every Remote-Write feature.

**Important**: You should review the detailed test output to judge compliance for your implementation. A successful `go test` run alone is not sufficient.

## Prerequisites

### For Sender Tests
- Go 1.23 or later
- The sender binary to test (e.g., Prometheus)

The test suite automatically downloads and runs Prometheus as the reference sender implementation. For testing custom senders, place the binary in the `bin/` directory.

### For Receiver Tests
A Prometheus server with Remote-Write Receiver enabled, as baseline:
```bash
prometheus --web.enable-remote-write-receiver --enable-feature=native-histograms
```

Or any other Remote-Write Receiver endpoint.

## Configuration

### Sender Configuration
The test suite uses environment variables:

**Sender Selection:**
```bash
export PROMETHEUS_RW2_COMPLIANCE_SENDERS="prometheus"
```

Currently supported senders:
- `prometheus` - The reference Prometheus implementation (automatically downloaded)

**Test Timeout:**
```bash
export PROMETHEUS_RW2_COMPLIANCE_TEST_TIMEOUT="10m"
```

### Receiver Configuration
The main configuration file `config.yml` in the `/remotewrite/receiver/` directory controls which receiver endpoints to test. It follows the Prometheus [`remote_write`](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write) structure:

```yaml
remote_write:
  - name: local-prometheus
    url: http://127.0.0.1:9090/api/v1/write
  - name: remote-endpoint
    url: https://your-remote-write-endpoint.com/api/v1/write
    basic_auth:
      username: user
      password: pass
```

If no `config.yml` exists, the test suite will fall back to `config_example.yml`.

Alternatively, use the `PROMETHEUS_RW2_COMPLIANCE_CONFIG_FILE` environment variable.

**Receiver Filtering:**
```bash
export PROMETHEUS_RW2_COMPLIANCE_RECEIVERS="local-prometheus,mimir"
```

## Running Tests

### Sender Tests
```bash
PROMETHEUS_RW2_COMPLIANCE_SENDERS="prometheus" go test -v
# Or other sender if you want
```

### Receiver Tests
```bash
PROMETHEUS_RW2_COMPLIANCE_RECEIVERS="local-prometheus" go test -v
# Or other receiver if you want
```

## HTML Visualization

Both sender and receiver test suites include an `index.html` file that enables viewing compliance test results.

To generate and view results:

**For Sender Tests:**
```bash
cd sender
go test -json | tee results.json
# Open index.html in your browser
```

**For Receiver Tests:**
```bash
cd receiver
go test -json | tee results.json
# Open index.html in your browser
```

## Protocol Compliance Levels

Tests are marked with compliance levels according to RFC specifications:
- **MUST**: Required by specification
- **SHOULD**: Recommended by specification
- **MAY**: Could have by specification
- **RECOMMENDED**: Not in specification but recommended for performance 

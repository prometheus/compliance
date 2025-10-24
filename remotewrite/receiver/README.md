# Prometheus Remote-Write v2 Receiver Compliance Test Suite

This repository contains a compliance test suite for Prometheus Remote-Write Protocol receivers. It validates that Remote-Write endpoints properly implement the Remote-Write v2 specification according to the [official protocol requirements](https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/).

## Overview

The test suite sends various Remote-Write v2 requests to configured endpoints and validates responses against the protocol specification. It tests proper handling of:

- Float samples
- Native Histograms
- Exemplars 
- Protocol headers and response codes
- Error conditions and edge cases
- Content-Type validation
- Response header (`X-Prometheus-Remote-Write-*-Written`)

## Limitations

Because different Remote-Write receivers have varied behaviors and APIs, this test suite does not verify data ingestion by reading data back from the receiver.

Some requests that are valid for one backend might be rejected by another. The suite tolerates both 200 and 400 series HTTP responses since actual data validation is up to the receiver.

Therefore, passing all tests does not guarantee that a receiver supports every Remote-Write feature (such as Native Histograms); some receivers may legitimately return a 400 error for unsupported features.

You should review the detailed test output to judge compliance for your receiver. A successful `go test` run alone is not sufficient.

## Prerequisites

A Prometheus server with Remote-Write Receiver enabled, as baseline:
  ```bash
  prometheus --web.enable-remote-write-receiver --enable-feature=native-histograms
  ```

Or

Any other Remote-Write Receiver.

## Configuration

The main test configuration file `config.yml` in the `<repo>/remotewrite/receiver/` directory controls
the receiver test suite, notably which receiver endpoints to test and any extra client configuration
(auth, extra headers or relabeling). It follows the Prometheus [`remote_write`](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#remote_write) structure.
Configuration options that are not relevant for Remote-Write are ignored.

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

Alternatively, a configuration file can be provided with the `PROMETHEUS_RW2_COMPLIANCE_CONFIG_FILE` environment variable.

### Receiver Filtering

You can filter which receivers from the configuration file to test using the `PROMETHEUS_RW2_COMPLIANCE_RECEIVERS` environment variable:

```bash
export PROMETHEUS_RW2_COMPLIANCE_RECEIVERS="local-prometheus,mimir"
go test -v
```

## Running Tests

After ensuring your receivers are running and available (e.g. Prometheus) and `config.yml` is configured, you can use `go test` to run all or some test cases:

```bash
# Run all compliance tests
go test -v -timeout 5m

# Run tests for a specific area
go test -v -run TestHistograms
go test -v -run TestMetrics

# Run tests with detailed output
go test -v -count=1

# Run tests against specific receivers only
PROMETHEUS_RW2_COMPLIANCE_RECEIVERS="local-prometheus" go test -v
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

## Response Validation

The test suite validates:
- HTTP status codes (2xx for success, 4xx for client errors, 5xx for server errors)
- Required response headers (`X-Prometheus-Remote-Write-*-Written`)

# Prometheus Remote-Write v2 Compliance Test Suite

This repository contains compliance test suites for both **senders** and **receivers** of the Prometheus Remote-Write Protocol 2.0. It validates implementation of the Remote-Write v2 specification according to the [official protocol requirements](https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/) as well as some more strict Prometheus implementation aspects.

Both [sender](#sender-compliance-sender) and [receiver](#receiver-compliance-receiver) test suites uses a shared [`./index.html`](./index.html) file that enables viewing compliance test results after they complete.

To generate and view results, add `-json` flag to `go test` command and `tee` the results to a file called `results.json`.

Tests are marked with compliance levels according to RFC specifications:
- **MUST**: Required by specification
- **SHOULD**: Recommended by specification
- **MAY**: Could have by specification
- **RECOMMENDED**: Not in specification but recommended for performance

## Sender Compliance (`/sender`)

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

### Limitations

The test suite validates the format and structure of requests sent by the sender but does not verify end-to-end data flow or persistence. Because senders may have different configuration options and capabilities, passing all tests does not guarantee a sender supports every Remote-Write feature (such as Native Histograms). Some senders may not expose certain features or may require specific configuration.

### Prerequisites

- Go 1.23 or later
- The sender binary to test (e.g., Prometheus)

The test suite automatically downloads and runs Prometheus as the reference sender implementation. For testing custom senders, place the binary in the `bin/` directory.

### Configuration

The test suite uses environment variables:

**Sender Selection:**

```bash
PROMETHEUS_RW2_COMPLIANCE_SENDERS="prometheus"
```

**Debug output:**

Debug variable controls if the tested process suppresses output (empty DEBUG) or not. 

```bash
DEBUG="1"
```

Currently supported senders:
- `prometheus` - The reference Prometheus implementation (automatically downloaded)

### Running Tests

```bash
pushd ./sender
# Run compliance tests e.g. for the default prometheus binary ...
PROMETHEUS_RW2_COMPLIANCE_SENDERS="prometheus" go test -v
popd
```

To use visualization HTML page, export data to JSON and generate `results.json` e.g.

```bash
pushd ./sender
PROMETHEUS_RW2_COMPLIANCE_SENDERS="prometheus" go test -json | tee results.json
popd

# Open ./index.html in your browser and see results.
```

## Receiver Compliance (`/receiver`)

Tests that Remote-Write endpoints properly handle incoming requests by sending various Remote-Write v2 requests and validating responses.

Tests cover:
- Float samples handling
- Native Histograms handling
- Exemplars handling
- Protocol headers and response codes
- Error conditions and edge cases
- Content-Type validation
- Response headers (`X-Prometheus-Remote-Write-*-Written`)

### Limitations

This test suite does not verify data ingestion by reading data back from the receiver. Some requests that are valid for one backend might be rejected by another. The suite tolerates both 200 and 400 series HTTP responses since actual data validation is up to the receiver. Therefore, passing all tests does not guarantee that a receiver supports every Remote-Write feature.

**Important**: You should review the detailed test output to judge compliance for your implementation. A successful `go test` run alone is not sufficient.

### Prerequisites

You need a receiving endpoint you want to test.

For example, a Prometheus server with Remote-Write Receiver enabled could be used as a baseline:

```bash
prometheus --web.enable-remote-write-receiver --enable-feature=native-histograms # Add config file.
```

### Configuration

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

You can configure which remote write endpoints to test from the provided `PROMETHEUS_RW2_COMPLIANCE_CONFIG_FILE` file
via the `PROMETHEUS_RW2_COMPLIANCE_RECEIVERS` environment variable:

```bash
export PROMETHEUS_RW2_COMPLIANCE_RECEIVERS="local-prometheus,mimir"
```

### Running Tests

Simply run `go test` in the `receiver` directory. For example:

```bash
pushd ./receiver
# Run compliance tests e.g. for the local-prometheus remote write endpoint defined in your PROMETHEUS_RW2_COMPLIANCE_CONFIG_FILE.
PROMETHEUS_RW2_COMPLIANCE_RECEIVERS="local-prometheus" go test -v
popd
```

To use visualization HTML page, export data to JSON and generate `results.json` e.g.

```bash
pushd ./receiver
PROMETHEUS_RW2_COMPLIANCE_RECEIVERS="local-prometheus" go test -json | tee results.json
popd

# Open ./index.html in your browser and see results.
```

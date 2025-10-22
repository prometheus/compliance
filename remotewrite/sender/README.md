# Prometheus Remote Write Sender Compliance Test

This repo contains a set of tests to check compliance with the Prometheus Remote Write specifications for senders:
- [Remote Write 1.0 specification](https://prometheus.io/docs/specs/remote_write_spec/)
- [Remote Write 2.0 specification](https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/)

The test suite works by forking an instance of the sender with some config to scrape the test running itself and send remote write requests to the test suite for a fixed period of time.
The test suite then examines the received requests for compliance.

## Running the tests

The tests are vanilla Golang unit tests, and can be run as such.

### Run all tests (RW 1.0 + RW 2.0):

```sh
$ go test --tags=compliance -v ./
```

### Run Remote Write 1.0 tests only:

```sh
$ go test --tags=compliance -run "TestRemoteWrite/prometheus/.+" -v ./
```

### Run Remote Write 2.0 protocol tests only:

```sh
$ go test --tags=compliance -run "TestRemoteWriteV2/prometheus/.+" -v ./
```

### Run a single test across all targets:

```sh
$ go test --tags=compliance -run "TestRemoteWrite/.+/Counter" -v ./
```

## Remote Write Senders

The repo tests the following remote write senders:
- [Prometheus](https://github.com/prometheus/prometheus/) itself.
- The [Grafana Agent](https://github.com/grafana/agent).
- [InfluxData's Telegraf](https://github.com/influxdata/telegraf).
- The [OpenTelemetry Collector](https://github.com/open-telemetry/opentelemetry-collector).
- The [VictoriaMetrics Agent](https://github.com/VictoriaMetrics/VictoriaMetrics/tree/master/app/vmagent), unless you're on [Mac OS X](https://github.com/VictoriaMetrics/VictoriaMetrics/issues/1042).
- [Timber.io Vector](https://github.com/timberio/vector).

If you want to test a dev version of your sender, simply put your binary in `bin/`.

To add another sender, see the examples in [the targets director](targets/) and recreate that pattern in a PR.

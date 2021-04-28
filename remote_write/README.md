# Prometheus Remote Write Compliance Test

This repo contains a set of tests to check compliance with the [Prometheus Remote Write specification](https://docs.google.com/document/d/1LPhVRSFkGNSuU1fBd81ulhsCPR4hkSZyyBj1SZ8fWOM/).

The test suit works by forking an instance of the sender with some config to scrape the test running itself and send remote write requests to the test suite for a fixed period of time.
The test suit than examines the received requests for compliance.

## Running the test

The test is a vanilla Golang unit test, and can be run as such.  To run all the tests:

```sh
$ go test --tags=compliance -v ./
```

To run all the tests for a single target:

```sh
$ go test --tags=compliance -run "TestRemoteWrite/prometheus/.+" -v ./
```

To run a single test across all the targets:

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

If you want to add another sender, see the examples in [the targets director](targets/) and recreate that pattern in a PR.

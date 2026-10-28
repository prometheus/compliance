# Prometheus Compliance Tests

This repo contains code to test compliance with various Prometheus standards. Anyone taking part in CNCF's [Prometheus Conformance](https://github.com/cncf/prometheus-conformance) Program will need to run the tests in here against their own implementations.

If you are reading this as someone testing their own implementation or considering to do so: There is a _LOT_ of work that's planned but not executed yet. If you have time or headcount to invest in uplifting everyone's compliance, [please talk to us](https://prometheus.io/community/).

There are several [software categories](https://docs.google.com/document/d/1VGMme9RgpclqF4CF2woNmgFqq0J7nqHn-l72uNmAxhA/) something can be tested in. If something does not seem to fit existing categories, please also talk to us.

## Alert Generator

The [alert_generator](alert_generator/README.md) directory contains a shim at the moment. It will test correct generation and emitting of alerts towards Alertmanager.

## OpenMetrics

The [openmetrics](openmetrics/README.md) directory contains a reference to the [OpenMetrics](https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md) test suite.

## PromQL

The [promql](promql/README.md) directory contains code to test compliance with the [native Prometheus PromQL implementation](https://github.com/prometheus/prometheus/tree/main/promql).

## PromQL E2E

The [promqle2e](promqle2e/README.md) directory contains a Go Module for performing
compliance tests for PromQL correctness. It's designed for smaller in-place quick unit tests, e.g. on per-PR basis, using docker based test framework. Useful as an acceptance tests
for vendors or those who wish to maintain high Prometheus compatibility over time.

## Remote Write: Sender

The [remotewrite/sender](remotewrite/sender/README.md) directory contains code to test compliance with the [Prometheus Remote Write specification](https://prometheus.io/docs/specs/remote_write_spec/) as a sender.

## Credits

Current maintainers are listed in [MAINTAINERS.md](MAINTAINERS.md). This project builds on the work of its original authors and previous maintainers:

* [Tom Wilkie](https://github.com/tomwilkie), who created this repository and authored the original Remote Write sender compliance test suite.
* [Julius Volz](https://github.com/juliusv) ([PromLabs](https://promlabs.com/)), who authored and donated the original [PromQL compliance tester](https://promlabs.com/blog/2020/08/06/comparing-promql-correctness-across-vendors) and query test suite.
* [Richard "RichiH" Hartmann](https://github.com/RichiH), who set up the repository structure, OpenMetrics compliance references, and initial Remote Write targets.
* [Ganesh Vernekar](https://github.com/codesome), who authored the Alert Generator compliance specification and test suite.

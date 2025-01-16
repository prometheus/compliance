# OpenMetrics compliance tester

While [OpenMetrics](https://openmetrics.io/) is an independent CNCF project, Prometheus designated it as the [official specification](https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md) for Prometheus exposition. OpenMetrics is, and will remain, closely aligned to Prometheus.

The test suite can be found on [GitHub](https://github.com/OpenObservability/OpenMetrics/tree/main/src/cmd/openmetricstest). Depending on your implementation, the [individual test cases](https://github.com/OpenObservability/OpenMetrics/tree/main/tests/testdata/parsers) might be of direct interest.

Prometheus considers OpenMetrics compliance part of Prometheus compliance and compatibility.

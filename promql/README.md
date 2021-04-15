# PromQL Compliance Tester

[![CircleCI](https://circleci.com/gh/promlabs/promql-compliance-tester/tree/master.svg?style=shield)](https://circleci.com/gh/promlabs/promql-compliance-tester)
[![Go Report Card](https://goreportcard.com/badge/github.com/promlabs/promql-compliance-tester)](https://goreportcard.com/report/github.com/promlabs/promql-compliance-tester)

The PromQL Compliance Tester is a tool for running comparison tests between native Prometheus and vendor PromQL API implementations.

The tool was first published and described in https://promlabs.com/blog/2020/08/06/comparing-promql-correctness-across-vendors, and some test results have been published at https://promlabs.com/promql-compliance-tests.

## Build Requirements

This tool is written in Go and requires a working Go setup to build. Library dependencies are handled via [Go Modules](https://blog.golang.org/using-go-modules).

## Building

To build the tool:

```bash
go build ./cmd/promql-compliance-tester
```

## Available flags

To list available flags:

```
$ ./promql-compliance-tester -h
Usage of ./promql-compliance-tester:
  -config-file string
    	The path to the configuration file. (default "promql-compliance-tester.yml")
  -output-format string
    	The comparison output format. Valid values: [text, html, json] (default "text")
  -output-html-template string
    	The HTML template to use when using HTML as the output format. (default "./output/example-output.html")
  -output-passing
    	Whether to also include passing test cases in the output.
```

## Configuration

The test cases, query tweaks, and PromQL API endpoints to use are specified in a configuration file.

An example configuration file with settings for Thanos, Cortex, TimescaleDB, and VictoriaMetrics is included.

## Contributing

It's still early days for the PromQL Compliance Tester. In particular, we would love to add and improve the following points:

* Test instant queries in addition to range queries.
* Add more variation and configurability to input timestamps.
* Flesh out a more comprehensive (and less overlapping) set of input test queries.
* Automate and integrate data loading into different systems.
* Test more vendor implementations of PromQL (for example, Sysdig and New Relic).
* Version test results and make pretty output presentations easier.

**Note:** Many people will be interested in benchmarking performance differences between PromQL implementations. While this is important as well, the PromQL Compliance Tester focuses solely on correctness testing.

If you would like to help flesh out the tester, please [file issues](https://github.com/promlabs/promql-compliance-tester/issues) or [pull requests](https://github.com/promlabs/promql-compliance-tester/pulls).

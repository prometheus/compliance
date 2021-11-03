# PromQL Compliance Tester

The PromQL Compliance Tester is a tool for running comparison tests between native Prometheus and vendor PromQL API implementations.

The tool was [first published and described](https://promlabs.com/blog/2020/08/06/comparing-promql-correctness-across-vendors) in August 2020. [Test results have been published](https://promlabs.com/promql-compliance-tests) on 2020-08-06 and 2020-12-01.

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
  -config-file value
    	The path to the configuration file. If repeated, the specified files will be concatenated before YAML parsing.
  -output-format string
    	The comparison output format. Valid values: [text, html, json] (default "text")
  -output-html-template string
    	The HTML template to use when using HTML as the output format. (default "./output/example-output.html")
  -output-passing
    	Whether to also include passing test cases in the output.
  -query-parallelism int
    	Maximum number of comparison queries to run in parallel. (default 20)
```

## Configuration

A standard suite of test cases is defined in the [`promql-test-queries.yml`](./promql-test-queries.yml) file, while separate `test-<vendor>.yml` config files specify test target configurations and query tweaks for a number of individual projects and vendors. To run the tester tool, you need to specify both the test suite config file as well as a config file for a single vendor.

For example, to run the tester against Cortex:

```bash
./promql-compliance-tester -config-file=promql-test-queries.yml -config-file=test-cortex.yml
```

Note that some of the vendor-specific configuration files require you to replace certain placeholder values for endpoints and credentials before using them.

## Contributing

It's still early days for the PromQL Compliance Tester. In particular, we would love to add and improve the following points:

* Test instant queries in addition to range queries.
* Add more variation and configurability to input timestamps.
* Flesh out a more comprehensive (and less overlapping) set of input test queries.
* Automate and integrate data loading into different systems.
* Test more vendor implementations of PromQL.
* Version test results and make pretty output presentations easier.

**Note:** Many people will be interested in benchmarking performance differences between PromQL implementations. While this is important as well, the PromQL Compliance Tester focuses solely on correctness testing. Please contact the [maintainers](../MAINTAINERS.md) if you want to work on performance testing.

If you would like to help flesh out the tester, please [file issues](https://github.com/prometheus/compliance/issues) or [pull requests](https://github.com/prometheus/compliance/pulls).

# Makefile for Prometheus Remote Write 2.0 Sender Compliance Tests

.PHONY: help test test-manual test-prometheus test-grafana test-all clean results

help:
	@echo "Prometheus Remote Write 2.0 Sender Compliance Test Suite"
	@echo ""
	@echo "Usage:"
	@echo "  make test                   Run tests (auto-downloads binaries)"
	@echo "  make test-prometheus        Run tests against Prometheus only"
	@echo "  make test-grafana           Run tests against Grafana Agent only"
	@echo "  make test-otel              Run tests against OpenTelemetry Collector only"
	@echo "  make test-vmagent           Run tests against VictoriaMetrics Agent only"
	@echo "  make test-manual            Run tests with manual config (requires config.yml)"
	@echo "  make results                Generate HTML results from test output"
	@echo "  make clean                  Clean build artifacts and downloaded binaries"
	@echo ""
	@echo "Environment Variables:"
	@echo "  PROMETHEUS_RW2_COMPLIANCE_SENDERS=name,...   Filter which senders to test"
	@echo "  USE_MANUAL_CONFIG=true                        Force manual config mode"
	@echo ""

# Run tests with automatic binary downloading (DEFAULT)
test:
	@echo "Running tests with automatic binary downloading..."
	go test  -timeout 30m

# Run tests against specific sender with auto-download
test-prometheus:
	@echo "Running tests against Prometheus..."
	PROMETHEUS_RW2_COMPLIANCE_SENDERS=prometheus go test  -timeout 15m

test-grafana:
	@echo "Running tests against Grafana Agent..."
	PROMETHEUS_RW2_COMPLIANCE_SENDERS=grafana_agent go test  -timeout 15m

test-otel:
	@echo "Running tests against OpenTelemetry Collector..."
	PROMETHEUS_RW2_COMPLIANCE_SENDERS=otelcollector go test  -timeout 15m

test-vmagent:
	@echo "Running tests against VictoriaMetrics Agent..."
	PROMETHEUS_RW2_COMPLIANCE_SENDERS=vmagent go test  -timeout 15m

test-telegraf:
	@echo "Running tests against Telegraf..."
	PROMETHEUS_RW2_COMPLIANCE_SENDERS=telegraf go test  -timeout 15m

test-vector:
	@echo "Running tests against Vector..."
	PROMETHEUS_RW2_COMPLIANCE_SENDERS=vector go test  -timeout 15m

# Run tests with manual configuration
test-manual:
	@echo "Running tests with manual configuration (config.yml)..."
	@if [ ! -f config.yml ]; then \
		echo "Error: config.yml not found. Copy config_example.yml to config.yml and customize it."; \
		exit 1; \
	fi
	USE_MANUAL_CONFIG=true go test  -timeout 30m

# Generate HTML results from test output
results:
	@echo "Generating test results..."
	go test --tags=compliance -json -timeout 30m | tee results.json
	@echo ""
	@echo "Results saved to results.json. Open index.html to view the test matrix."

# Clean up build artifacts and downloaded binaries
clean:
	@echo "Cleaning up..."
	rm -rf bin/
	rm -f *.test
	rm -f results.json
	rm -f coverage.txt
	rm -f coverage.html
	@echo "Clean complete"

# Build the test binary
build:
	@echo "Building test binary..."
	go test --tags=compliance -c -o sender.test
	@echo "Test binary created: sender.test"

# Run a specific test by name
test-run:
	@echo "Usage: make test-run TEST=TestProtocolCompliance"
	@if [ -z "$(TEST)" ]; then \
		echo "Error: TEST variable not set"; \
		exit 1; \
	fi
	go test  -run $(TEST) -timeout 15m

# Show available senders
list-senders:
	@echo "Available senders for automatic testing:"
	@echo "  - prometheus      (Prometheus v3.7.1)"
	@echo "  - grafana_agent   (Grafana Agent v0.19.0)"
	@echo "  - otelcollector   (OpenTelemetry Collector)"
	@echo "  - vmagent         (VictoriaMetrics Agent)"
	@echo "  - telegraf        (Telegraf)"
	@echo "  - vector          (Vector)"
	@echo ""
	@echo "Run tests against a specific sender:"
	@echo "  make test-prometheus"
	@echo "  make test-grafana"
	@echo "  make test-otel"
	@echo "  make test-vmagent"

# Install dependencies
deps:
	@echo "Installing Go dependencies..."
	go mod download
	go mod tidy
	@echo "Dependencies installed"

# Run linter
lint:
	@echo "Running linters..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint not installed. Install from https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi

# Run tests with coverage
coverage:
	@echo "Running tests with coverage..."
	go test --tags=compliance -coverprofile=coverage.txt -covermode=atomic -timeout 30m
	go tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report saved to coverage.html"

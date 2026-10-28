# Set the default target to 'help' so it runs when you just type 'make'
.DEFAULT_GOAL := help

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
ROOT_DIR := $(dir $(mkfile_path))

TIMEOUT="10m"
PROMETHEUS_OPENMETRICS_COMPLIANCE_TEST_RE=""
PROMETHEUS_OPENMETRICS_COMPLIANCE_SKIP_TEST_RE=""
RESULT_FILE="$(ROOT_DIR)/results.json"
RUN_TEST = go test -run $(PROMETHEUS_OPENMETRICS_COMPLIANCE_TEST_RE) -skip $(PROMETHEUS_OPENMETRICS_COMPLIANCE_SKIP_TEST_RE)

.PHONY: help
help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available commands:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: test
test: ## Run OpenMetrics compliance tests suitable for humans and CI.
	@$(RUN_TEST) . -v -timeout=$(TIMEOUT)

.PHONY: test-web
test-web: ## Run OpenMetrics compliance tests with a HTML results view.
	@$(RUN_TEST) . -json | tee $(RESULT_FILE)
	@$(MAKE) results-web

.PHONY: results-web
results-web: ## Generate HTML view with ./results.json from ../remotewrite/index.html and open in your browser.
	@cat ../remotewrite/index.html | perl -pe 'BEGIN{local $$/; open(F,"<results.json"); $$c=<F>; close F} s/const PRELOADED_RESULT_DATA = ``;/const PRELOADED_RESULT_DATA = `$$c`;/' > index.preloaded.html
	@open index.preloaded.html

ROOT_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
SUITE_TITLE := OpenMetrics — Test Matrix

PROMETHEUS_OPENMETRICS_COMPLIANCE_TEST_RE ?= ""
PROMETHEUS_OPENMETRICS_COMPLIANCE_SKIP_TEST_RE ?= ""
RUN_TEST = go test -run $(PROMETHEUS_OPENMETRICS_COMPLIANCE_TEST_RE) -skip $(PROMETHEUS_OPENMETRICS_COMPLIANCE_SKIP_TEST_RE)

include $(ROOT_DIR)../Makefile.base

.PHONY: test
test: ## Run OpenMetrics compliance tests suitable for humans and CI.
	@$(RUN_TEST) . -v -timeout=$(TIMEOUT)

.PHONY: test-web
test-web: ## Run OpenMetrics compliance tests with a HTML results view.
	@$(RUN_TEST) . -json | tee "$(RESULT_FILE)"
	@$(MAKE) results-web

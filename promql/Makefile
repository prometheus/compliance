ROOT_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
SUITE_TITLE := PromQL — Test Matrix

PROMETHEUS_PROMQL_COMPLIANCE_TEST_RE ?= ""
PROMETHEUS_PROMQL_COMPLIANCE_SKIP_TEST_RE ?= ""
RUN_TEST = go test -run $(PROMETHEUS_PROMQL_COMPLIANCE_TEST_RE) -skip $(PROMETHEUS_PROMQL_COMPLIANCE_SKIP_TEST_RE)

include $(ROOT_DIR)../Makefile.common

.PHONY: test
test: ## Run PromQL compliance tests suitable for humans and CI.
	@$(RUN_TEST) . -v -timeout=$(TIMEOUT)

.PHONY: test-web
test-web: ## Run PromQL compliance tests with a HTML results view.
	@$(RUN_TEST) . -json | tee "$(RESULT_FILE)"
	@$(MAKE) results-web

.PHONY: docker
docker: ## Build the legacy promql-compliance-tester Docker image.
	docker build . -t promql-compliance-tester:latest

ROOT_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
SUITE_TITLE := Remote-Write v2 — Test Matrix

PROMETHEUS_RW_COMPLIANCE_TEST_RE ?= ""
PROMETHEUS_RW_COMPLIANCE_SKIP_TEST_RE ?= ""
RUN_TEST = go test -run $(PROMETHEUS_RW_COMPLIANCE_TEST_RE) -skip $(PROMETHEUS_RW_COMPLIANCE_SKIP_TEST_RE)

include $(ROOT_DIR)../Makefile.common

.PHONY: sender
sender: ## Run sender compliance tests for a given senders suitable for humans and CI.
	@$(RUN_TEST) ./sender -v -timeout=$(TIMEOUT)

.PHONY: sender-web
sender-web: ## Run sender compliance tests for a given senders with a HTML results view.
	@$(RUN_TEST) ./sender -json | tee "$(RESULT_FILE)"
	@$(MAKE) results-web

.PHONY: receiver
receiver: ## Run receiver compliance tests for a given receivers suitable for humans and CI.
	@$(RUN_TEST) ./receiver -v -timeout=$(TIMEOUT)

.PHONY: receiver-web
receiver-web: ## Run receiver compliance tests for a given receivers with a HTML results view.
	@$(RUN_TEST) ./receiver -json | tee "$(RESULT_FILE)"
	@$(MAKE) results-web

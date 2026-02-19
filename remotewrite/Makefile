# Set the default target to 'help' so it runs when you just type 'make'
.DEFAULT_GOAL := help

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
current_dir := $(notdir $(patsubst %/,%,$(dir $(mkfile_path))))
ROOT_DIR := $(dir $(mkfile_path))

SED=$(shell which gsed 2>/dev/null || which sed)

# Shared variables
TIMEOUT="10m"
PROMETHEUS_RW_COMPLIANCE_TEST_RE=""
PROMETHEUS_RW_COMPLIANCE_SKIP_TEST_RE=""
RESULT_FILE="$(ROOT_DIR)/results.json"
RUN_TEST = go test -run $(PROMETHEUS_RW_COMPLIANCE_TEST_RE) -skip $(PROMETHEUS_RW_COMPLIANCE_SKIP_TEST_RE)

.PHONY: help
help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available commands:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: sender
sender: ## Run sender compliance tests for a given senders suitable for humans and CI.
	@$(RUN_TEST) ./sender -v -timeout=$(TIMEOUT)

.PHONY: sender-web
sender-web: ## Run sender compliance tests for a given senders with a HTML results view.
	@$(RUN_TEST) ./sender -json | tee $(RESULT_FILE)
	@$(MAKE) results-web

.PHONY: receiver
receiver: ## Run receiver compliance tests for a given receivers suitable for humans and CI.
	@$(RUN_TEST) ./receiver -v -timeout=$(TIMEOUT)

.PHONY: receiver-web
receiver-web: ## Run receiver compliance tests for a given receivers with a HTML results view.
	@$(RUN_TEST) ./receiver -json | tee $(RESULT_FILE)
	@$(MAKE) results-web

.PHONY: results-web
results-web: ## Generate HTML view with ./results.json from the index.html file and open in your browser. We re-generate HTML file to avoid CORS issue on secure browsers.
	@cat index.html | perl -pe 'BEGIN{local $$/; open(F,"<results.json"); $$c=<F>; close F} s/const PRELOADED_RESULT_DATA = ``;/const PRELOADED_RESULT_DATA = `$$c`;/' > index.preloaded.html
	@open index.preloaded.html
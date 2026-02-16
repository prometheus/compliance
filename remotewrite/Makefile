# Set the default target to 'help' so it runs when you just type 'make'
.DEFAULT_GOAL := help

DEBUG=? ""

.PHONY: help
help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available commands:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

PROMETHEUS_RW2_COMPLIANCE_SENDERS =? "prometheus"

.PHONY: sender
sender: ## Run sender compliance tests for a given PROMETHEUS_RW2_COMPLIANCE_SENDERS with a HTML results view.
	cd ./sender && DEBUG=$(DEBUG) PROMETHEUS_RW2_COMPLIANCE_SENDERS="$(PROMETHEUS_RW2_COMPLIANCE_SENDERS)" go test -json | tee results.json
	@echo "Opening HTML result view; load the `results.json` file"
	@open ./index.html

.PHONY: sender-check
sender-ci: ## Run sender compliance tests for a given PROMETHEUS_RW2_COMPLIANCE_SENDERS suitable for CI.
	cd ./sender && DEBUG=$(DEBUG) PROMETHEUS_RW2_COMPLIANCE_SENDERS="$(PROMETHEUS_RW2_COMPLIANCE_SENDERS)" go test -v

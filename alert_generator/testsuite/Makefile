build:
	go build ./cmd/alert_generator_compliance_tester/

run:
	@echo "Running alert_generator_compliance_tester for development. Use binaries to run actual tests."
	go run ./cmd/alert_generator_compliance_tester/

build-rules:
	go run ./cmd/rule_config_builder/main.go -rules-file-path="./rules.yaml"

.PHONY: check-rules
check-rules: build-rules
	@git diff --exit-code -- ./*.yaml

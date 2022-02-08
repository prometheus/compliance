build:
	go build ./cmd/alert_generator_compliance_tester/

run: build
	./alert_generator_compliance_tester

run-cortex: build
	./alert_generator_compliance_tester -api-base-url=http://localhost:8021/api/prom -promql-base-url=http://localhost:8007/api/prom -remote-write-url=http://localhost:8001/api/v1/push

build-rules:
	go run ./cmd/rule_config_builder/main.go -rules-file-path="./rules.yaml"

.PHONY: check-rules
check-rules: build-rules
	@git diff --exit-code -- ./*.yaml

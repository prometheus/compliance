build:
	go build ./cmd/alert_generator_compliance_tester/

run: build
	./alert_generator_compliance_tester -config-file=$(CONFIG)

rules:
	go run ./cmd/rule_config_builder/main.go -rules-file-path="./rules.yaml"

clean:
	rm -f alert_generator_compliance_tester

.PHONY: check-rules
check-rules: build-rules
	@git diff --exit-code -- ./*.yaml

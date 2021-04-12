package targets

import (
	"fmt"
	"os"
)

const otelDownloadURL = "https://github.com/open-telemetry/opentelemetry-collector/releases/download/v0.24.0/otelcol_{{.OS}}_{{.Arch}}"

func RunOtelCollector(opts TargetOptions) error {
	binary, err := downloadBinary(otelDownloadURL, "")
	if err != nil {
		return err
	}

	cfg := fmt.Sprintf(`
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: 'test'
          scrape_interval: 1s
          static_configs:
            - targets: [ '%s' ]

processors:
  batch:

exporters:
  prometheusremotewrite:
    endpoint: '%s'

service:
  pipelines:
    metrics:
      receivers: [prometheus]
      processors: [batch]
      exporters: [prometheusremotewrite]
`, opts.ScrapeTarget, opts.ReceiveEndpoint)
	configFileName, err := writeTempFile(cfg, "config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(configFileName)

	return runCommand(binary, opts.Timeout, `--metrics-addr=:0`, fmt.Sprintf("--config=%s", configFileName))
}

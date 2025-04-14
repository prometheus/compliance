package targets

import (
	"fmt"
	"os"
)

const otelDownloadURL = "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.123.1/otelcol_0.123.1_{{.OS}}_{{.Arch}}.tar.gz"

func RunOtelCollector(opts TargetOptions) error {
	binary, err := downloadBinary(otelDownloadURL, "otelcol")
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
    add_metric_suffixes: false

service:  
  telemetry:
    metrics:
      readers:
        - pull:
            exporter:
              prometheus:
                host: 'localhost'
                port: 0
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

	return runCommand(binary, opts.Timeout, fmt.Sprintf("--config=%s", configFileName))
}

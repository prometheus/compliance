package targets

import (
	"fmt"
	"os"
	"strings"
)

const prometheusDownloadURL = "https://github.com/prometheus/prometheus/releases/download/v3.7.1/prometheus-3.7.1.{{.OS}}-{{.Arch}}.tar.gz"

func RunPrometheus(opts TargetOptions) error {
	binary, err := downloadBinary(prometheusDownloadURL, "prometheus")
	if err != nil {
		return err
	}

	// Extract host:port from scrape target URL (Prometheus expects just host:port, not full URL)
	scrapeTarget := strings.TrimPrefix(opts.ScrapeTarget, "http://")
	scrapeTarget = strings.TrimPrefix(scrapeTarget, "https://")

	// Write out config file.
	cfg := fmt.Sprintf(`
global:
  scrape_interval: 1s

remote_write:
  - url: '%s'
    protobuf_message: "io.prometheus.write.v2.Request"
    send_exemplars: true
    metadata_config:
      send: true

scrape_configs:
  - job_name: 'test'
    scrape_interval: 1s
    scrape_protocols:
      - OpenMetricsText1.0.0
      - PrometheusText0.0.4
    static_configs:
    - targets: ['%s']
`, opts.ReceiveEndpoint, scrapeTarget)
	configFileName, err := writeTempFile(cfg, "config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(configFileName)

	return runCommand(binary, opts.Timeout, `--web.listen-address=0.0.0.0:0`, fmt.Sprintf("--config.file=%s", configFileName))
}

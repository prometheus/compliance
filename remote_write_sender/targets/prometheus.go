package targets

import (
	"fmt"
	"os"
)

const prometheusDownloadURL = "https://github.com/prometheus/prometheus/releases/download/v2.30.3/prometheus-2.30.3.{{.OS}}-{{.Arch}}.tar.gz"

func RunPrometheus(opts TargetOptions) error {
	binary, err := downloadBinary(prometheusDownloadURL, "prometheus")
	if err != nil {
		return err
	}

	// Write out config file.
	cfg := fmt.Sprintf(`
global:
  scrape_interval: 1s

remote_write:
  - url: '%s'

scrape_configs:
  - job_name: 'test'
    static_configs:
    - targets: ['%s']
`, opts.ReceiveEndpoint, opts.ScrapeTarget)
	configFileName, err := writeTempFile(cfg, "config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(configFileName)

	return runCommand(binary, opts.Timeout, `--web.listen-address=0.0.0.0:0`, fmt.Sprintf("--config.file=%s", configFileName))
}

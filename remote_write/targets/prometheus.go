package targets

import (
	"fmt"
	"os"

        "github.com/prometheus/compliance/remote_write/latest"
)

func RunPrometheus(opts TargetOptions) error {
	binary, err := downloadBinary(latest.GetDownloadURL("https://github.com/prometheus/prometheus/releases/download/vVERSION/prometheus-VERSION.{{.OS}}-{{.Arch}}.tar.gz"), "prometheus")
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

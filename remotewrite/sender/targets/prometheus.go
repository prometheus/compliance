package targets

import (
	"fmt"
	"os"
)

const prometheusDownloadURL = "https://github.com/prometheus/prometheus/releases/download/v3.7.1/prometheus-3.7.1.{{.OS}}-{{.Arch}}.tar.gz"

func RunPrometheus(opts TargetOptions) error {
	binary, err := downloadBinary(prometheusDownloadURL, "prometheus")
	if err != nil {
		return err
	}

	// Write out config file.
	// Conditionally add protobuf_message for RW 2.0 protocol
	protobufConfig := ""
	if opts.UseRW2Protocol {
		protobufConfig = `    protobuf_message: "io.prometheus.write.v2.Request"`
	}

	cfg := fmt.Sprintf(`
global:
  scrape_interval: 1s

remote_write:
  - url: '%s'
%s

scrape_configs:
  - job_name: 'test'
    static_configs:
    - targets: ['%s']
`, opts.ReceiveEndpoint, protobufConfig, opts.ScrapeTarget)
	configFileName, err := writeTempFile(cfg, "config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(configFileName)

	return runCommand(binary, opts.Timeout, `--web.listen-address=0.0.0.0:0`, fmt.Sprintf("--config.file=%s", configFileName))
}

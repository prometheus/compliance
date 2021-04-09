package targets

import (
	"fmt"
	"os"
)

const grafanaAgentDownloadURL = "https://github.com/grafana/agent/releases/download/v0.13.0/agent-{{.OS}}-{{.Arch}}.zip"

func RunGrafanaAgent(opts TargetOptions) error {
	binary, err := downloadBinary(grafanaAgentDownloadURL, "agent-darwin-amd64")
	if err != nil {
		return err
	}

	// Write out config file.
	cfg := fmt.Sprintf(`
server:
  http_listen_port: 8000
prometheus:
  wal_directory: ./
  global:
    scrape_interval: 1s
  configs:
  - name: test
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

	return runCommand(binary, opts.Timeout, fmt.Sprintf("--config.file=%s", configFileName))
}

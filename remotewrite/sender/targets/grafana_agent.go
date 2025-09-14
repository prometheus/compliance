package targets

import (
	"fmt"
	"os"
)

const grafanaAgentDownloadURL = "https://github.com/grafana/agent/releases/download/v0.19.0/agent-{{.OS}}-{{.Arch}}.zip"

func RunGrafanaAgent(opts TargetOptions) error {
	binary, err := downloadBinary(grafanaAgentDownloadURL, "agent-{{.OS}}-{{.Arch}}")
	if err != nil {
		return err
	}

	// Write out config file.
	protobufConfig := ""
	if opts.ProtocolVersion == 2 {
		protobufConfig = "      protobuf_message: 'io.prometheus.write.v2.Request'"
	}

	cfg := fmt.Sprintf(`
prometheus:
  wal_directory: ./
  global:
    scrape_interval: 1s
  configs:
  - name: test
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

	return runCommand(binary, opts.Timeout, "-server.http-listen-port=0", "-server.grpc-listen-port=0", fmt.Sprintf("--config.file=%s", configFileName))
}

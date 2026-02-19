package targets

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"text/template"
)

const (
	prometheusDownloadURL = "https://github.com/prometheus/prometheus/releases/download/v3.9.1/prometheus-3.9.1.{{.OS}}-{{.Arch}}.tar.gz"
	scrapeConfigTemplate  = `
global:
  scrape_interval: 1s

remote_write:
  - url: "{{.RemoteWriteEndpointURL}}"
    protobuf_message: "{{.RemoteWriteMessage}}"
    send_exemplars: true
    queue_config:
      retry_on_http_429: true
    metadata_config:
      send: true

scrape_configs:
  - job_name: "{{.ScrapeTargetJobName}}"
    scrape_interval: 1s
    scrape_protocols:
      - PrometheusProto
      - OpenMetricsText1.0.0
      - PrometheusText0.0.4
    static_configs:
    - targets: ["{{.ScrapeTargetHostPort}}"]
`
)

var scrapeConfigTmpl = template.Must(template.New("config").Parse(scrapeConfigTemplate))

// RunPrometheus runs a Prometheus process for a test target options, until ctx is done.
//
// It auto-downloads Prometheus binary from the official release URL (see prometheusDownloadURL).
// TODO(bwplotka): Process based runners are prone to leaking processes; add docker runner and/or figure out cleanup. Manually this could be done with 'killall -m "prometheus-3." -kill'.
func RunPrometheus(ctx context.Context, opts TargetOptions) error {
	binary, err := downloadBinary(prometheusDownloadURL, "prometheus")
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := scrapeConfigTmpl.Execute(&buf, opts); err != nil {
		return fmt.Errorf("failed to execute config template: %w", err)
	}

	configFileName, err := writeTempFile(buf.String(), "config-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(configFileName)

	return runCommand(ctx, nil, binary, `--web.listen-address=0.0.0.0:0`, fmt.Sprintf("--config.file=%s", configFileName))
}

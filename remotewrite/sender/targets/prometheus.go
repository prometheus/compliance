package targets

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"text/template"
)

const (
	prometheusDownloadURL = "https://github.com/prometheus/prometheus/releases/download/v3.8.0/prometheus-3.8.0.{{.OS}}-{{.Arch}}.tar.gz"
	scrapeConfigTemplate  = `
global:
  scrape_interval: 1s

remote_write:
  - url: "{{.ReceiveEndpointURL}}"
    protobuf_message: "{{.RemoteWriteMessage}}"
    send_exemplars: true
    metadata_config:
      send: true

scrape_configs:
  - job_name: "test"
    scrape_interval: 1s
    scrape_protocols:
      - PrometheusProto
      - OpenMetricsText1.0.0
      - PrometheusText0.0.4
    static_configs:
    - targets: ["{{ToHostPort .ScrapeTargetURL}}"]
`
)

var (
	customTmplFuncs = template.FuncMap{
		"ToHostPort": func(rawURL string) string {
			u, err := url.Parse(rawURL)
			if err != nil {
				return rawURL
			}
			return u.Host // host:port
		},
	}
	scrapeConfigTmpl = template.Must(template.New("config").Funcs(customTmplFuncs).Parse(scrapeConfigTemplate))
)

// RunPrometheus runs a Prometheus process for a test target options, until ctx is canceled or error.
// It auto-downloads Prometheus binary from the official release URL (see prometheusDownloadURL).
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

	return runCommand(ctx, binary, `--web.listen-address=0.0.0.0:0`, fmt.Sprintf("--config.file=%s", configFileName))
}

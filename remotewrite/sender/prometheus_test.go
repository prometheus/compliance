// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sender_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/prometheus/compliance/common"
	"github.com/prometheus/compliance/remotewrite/sender"
)

const scrapeConfigTemplate = `
global:
  scrape_interval: 1s

remote_write:
  - url: "{{.RemoteWriteEndpointURL}}"
    protobuf_message: "{{.RemoteWriteMessage}}"
{{- if .RemoteWriteMode}}
    mode: "{{.RemoteWriteMode}}"
{{- end}}
    send_exemplars: true
    send_native_histograms: true
    queue_config:
      retry_on_http_429: true
    metadata_config:
      send: true

scrape_configs:
  - job_name: "{{.ScrapeTargetJobName}}"
    scrape_interval: 1s
    scrape_native_histograms: true
{{- if .RemoteWriteMode}}
    convert_classic_histograms_to_nhcb: true
{{- end}}
    scrape_protocols:
      - PrometheusProto
      - OpenMetricsText1.0.0
      - PrometheusText0.0.4
    static_configs:
    - targets: ["{{.ScrapeTargetHostPort}}"]
`

var scrapeConfigTmpl = template.Must(template.New("config").Parse(scrapeConfigTemplate))

type prometheus struct{}

func (p prometheus) Name() string { return "prometheus" }

// Run runs a Prometheus process for a test target options, until ctx is done.
//
// It auto-downloads or builds the Prometheus binary (see common.GetPrometheusBinary).
// TODO(bwplotka): Process based runners are prone to leaking processes; add docker runner and/or figure out cleanup.
// Manually this could be done with 'killall -m "prometheus-3." -kill'.
func (p prometheus) Run(ctx context.Context, opts sender.Options) error {
	binary, err := common.GetPrometheusBinary()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := scrapeConfigTmpl.Execute(&buf, opts); err != nil {
		return fmt.Errorf("failed to execute config template: %w", err)
	}

	dir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, buf.Bytes(), 0o600); err != nil {
		return err
	}

	return sender.RunCommand(ctx, ".", nil,
		binary,
		`--web.listen-address=0.0.0.0:0`,
		fmt.Sprintf("--storage.tsdb.path=%v", dir),
		fmt.Sprintf("--config.file=%s", configFile),
		"--enable-feature=st-storage,exemplar-storage,metadata-wal-records",
	)
}

var _ sender.Sender = prometheus{}

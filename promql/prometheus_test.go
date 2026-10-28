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

package promql_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/prometheus/common/model"
	"github.com/prometheus/compliance/common"
	"github.com/prometheus/compliance/promql"
)

const (
	prometheusDownloadURL = "https://github.com/prometheus/prometheus/releases/download/v3.13.5/prometheus-3.13.5.{{.OS}}-{{.Arch}}.tar.gz"
	scrapeConfigTemplate  = `
global:
  scrape_interval: {{.ScrapeInterval}}

scrape_configs:
  - job_name: "{{.ScrapeTargetJobName}}"
    scrape_interval: {{.ScrapeInterval}}
    honor_labels: true
    honor_timestamps: true
    scrape_native_histograms: true
    scrape_protocols:
      - PrometheusProto
      - OpenMetricsText1.0.0
      - PrometheusText0.0.4
    static_configs:
      - targets: ["{{.ScrapeTargetHostPort}}"]
`
)

var scrapeConfigTmpl = template.Must(template.New("config").Parse(scrapeConfigTemplate))

type prometheus struct {
	name string
}

func (p prometheus) Name() string {
	if p.name != "" {
		return p.name
	}
	return "prometheus"
}

// Run runs a Prometheus process for the given test target options until ctx is done.
//
// It auto-downloads the Prometheus binary from the official release URL unless
// PROMETHEUS_COMPLIANCE_PROMQL_BINARY is set.
func (p prometheus) Run(ctx context.Context, opts promql.Options) error {
	binary := os.Getenv("PROMETHEUS_COMPLIANCE_PROMQL_BINARY")
	if binary == "" {
		var err error
		binary, err = common.DownloadBinary(prometheusDownloadURL, "prometheus")
		if err != nil {
			return err
		}
	}

	var buf bytes.Buffer
	if err := scrapeConfigTmpl.Execute(&buf, struct {
		ScrapeTargetJobName  string
		ScrapeTargetHostPort string
		ScrapeInterval       string
	}{
		ScrapeTargetJobName:  opts.ScrapeTargetJobName,
		ScrapeTargetHostPort: opts.ScrapeTargetHostPort,
		ScrapeInterval:       model.Duration(opts.ScrapeInterval).String(),
	}); err != nil {
		return fmt.Errorf("failed to execute config template: %w", err)
	}

	dir, err := os.MkdirTemp("", "promql-test-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, buf.Bytes(), 0o600); err != nil {
		return err
	}

	return promql.RunCommand(ctx, ".", nil,
		binary,
		fmt.Sprintf("--web.listen-address=%s", opts.QueryHostPort),
		fmt.Sprintf("--storage.tsdb.path=%v", dir),
		fmt.Sprintf("--config.file=%s", configFile),
	)
}

var _ promql.Target = prometheus{}

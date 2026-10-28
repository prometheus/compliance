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

package promql

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/compliance/common"
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
    scrape_protocols:
      - OpenMetricsText1.0.0
      - PrometheusText0.0.4
    static_configs:
      - targets: ["{{.ScrapeTargetHostPort}}"]
`
)

var scrapeConfigTmpl = template.Must(template.New("config").Parse(scrapeConfigTemplate))

// Target represents a PromQL-compatible backend under test.
// Target needs to collect (scrape) metrics from Options.ScrapeTargetHostPort
// and expose a Prometheus HTTP query API at Options.QueryHostPort.
type Target interface {
	// Name returns a unique target name.
	Name() string
	// Run starts the test target until the context is done.
	// Premature stops are assumed to be failures.
	Run(ctx context.Context, opts Options) error
}

// Options represents the target configuration for a PromQL compliance test run.
type Options struct {
	ScrapeTargetHostPort string
	ScrapeTargetJobName  string
	ScrapeInterval       time.Duration
	QueryHostPort        string
}

// RunCommand runs the given command with the given args until context is done.
//
// This is useful when starting process-based PromQL targets.
func RunCommand(ctx context.Context, dir string, extraEnvVars []string, prog string, args ...string) error {
	return common.RunCommand(ctx, dir, extraEnvVars, prog, args...)
}

type prometheusTarget struct {
	name   string
	binary string
}

// NewPrometheus returns a Target that runs a Prometheus process for the given target name and binary path.
// If binary is empty, the official reference Prometheus release is auto-downloaded into ./bin/.
func NewPrometheus(name, binary string) Target {
	return prometheusTarget{
		name:   name,
		binary: binary,
	}
}

func (p prometheusTarget) Name() string {
	if p.name != "" {
		return p.name
	}
	return "prometheus"
}

func (p prometheusTarget) Run(ctx context.Context, opts Options) error {
	binary := p.binary
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

	return RunCommand(ctx, ".", nil,
		binary,
		fmt.Sprintf("--web.listen-address=%s", opts.QueryHostPort),
		fmt.Sprintf("--storage.tsdb.path=%v", dir),
		fmt.Sprintf("--config.file=%s", configFile),
	)
}

var _ Target = prometheusTarget{}

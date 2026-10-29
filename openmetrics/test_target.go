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

package openmetrics

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"github.com/prometheus/compliance"
)

// Target represents an OpenMetrics metric producer (SDK test application or exporter) under test.
// Target must expose an HTTP metrics endpoint at http://<Options.MetricsHostPort><Options.MetricsPath>.
type Target = compliance.Target[Options]

// Options represents the target configuration for an OpenMetrics compliance test run.
type Options struct {
	// MetricsHostPort is the host:port address where the target must listen and serve metrics.
	MetricsHostPort string
	// MetricsPath is the HTTP path where metrics are served (e.g. "/metrics").
	MetricsPath string
}

// Version specifies the OpenMetrics specification version under test.
type Version string

const (
	// Version1_0_0 is the OpenMetrics 1.0.0 specification version.
	Version1_0_0 Version = "1.0.0"
	// Version2_0_0 is the OpenMetrics 2.0.0 specification version.
	Version2_0_0 Version = "2.0.0"
	// Version2_0_0RC is the OpenMetrics 2.0.0-rc specification version.
	Version2_0_0RC Version = "2.0.0-rc"
)

func (v Version) shortName() string {
	switch v {
	case Version1_0_0:
		return "om1"
	case Version2_0_0:
		return "om2"
	case Version2_0_0RC:
		return "om2-rc"
	default:
		return string(v)
	}
}

// AcceptHeader returns the canonical OpenMetrics Accept header for this version
// and optional escaping scheme.
func (v Version) AcceptHeader(escaping string) string {
	h := fmt.Sprintf("application/openmetrics-text; version=%s", v)
	if escaping != "" {
		h += fmt.Sprintf("; escaping=%s", escaping)
	}
	return h
}

// RunCommand runs the given command with the given args until context is done.
//
// This is useful when starting process-based OpenMetrics exporter or SDK targets.
func RunCommand(ctx context.Context, dir string, extraEnvVars []string, prog string, args ...string) error {
	return compliance.RunCommand(ctx, dir, extraEnvVars, prog, args...)
}

type handlerTarget struct {
	name    string
	handler http.Handler
}

// NewHandlerTarget wraps an in-process http.Handler as an OpenMetrics compliance Target.
func NewHandlerTarget(name string, handler http.Handler) Target {
	return &handlerTarget{
		name:    name,
		handler: handler,
	}
}

func (h *handlerTarget) Name() string { return h.name }

func (h *handlerTarget) Run(ctx context.Context, opts Options) error {
	mux := http.NewServeMux()
	path := opts.MetricsPath
	if path == "" {
		path = "/metrics"
	}
	mux.Handle(path, h.handler)

	ln, err := net.Listen("tcp", opts.MetricsHostPort)
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: mux}
	errCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		_ = srv.Close()
		return <-errCh
	case err := <-errCh:
		return err
	}
}

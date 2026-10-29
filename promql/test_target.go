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
	"context"
	"time"

	"github.com/prometheus/compliance"
)

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
	return compliance.RunCommand(ctx, dir, extraEnvVars, prog, args...)
}

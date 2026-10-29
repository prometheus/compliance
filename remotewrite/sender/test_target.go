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

package sender

import (
	"context"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance"
)

// Sender represents a test target.
// Sender generally needs to be able to collect (scrape) data from a Prometheus metric endpoint using OpenMetrics 1.0
// and send Prometheus metrics to the designated endpoint using Remote Write protocol.
type Sender = compliance.Target[Options]

// Options represents the sender options for a given test.
type Options struct {
	ScrapeTargetHostPort   string
	ScrapeTargetJobName    string
	RemoteWriteEndpointURL string
	RemoteWriteMessage     remote.WriteMessageType
	RemoteWriteMode        remote.WriteMode
}

// RunCommand runs the given command with the given args until context is done.
//
// This is useful when starting process-based sender targets.
func RunCommand(ctx context.Context, dir string, extraEnvVars []string, prog string, args ...string) error {
	return compliance.RunCommand(ctx, dir, extraEnvVars, prog, args...)
}

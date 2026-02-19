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
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"

	"github.com/prometheus/client_golang/exp/api/remote"
)

// Sender represents a test target.
// Sender generally needs to be able to collect (scrape) data from a Prometheus metric endpoint using OpenMetrics 1.0
// and send Prometheus metrics to the designated endpoint using Remote Write protocol.
type Sender interface {
	// Name returns unique sender name.
	Name() string
	// Run runs a test sender until the context is done.
	// Premature stops are assumed to be failures.
	Run(context.Context, Options) error
}

// Options represents the sender options for a given test.
type Options struct {
	ScrapeTargetHostPort   string
	ScrapeTargetJobName    string
	RemoteWriteEndpointURL string
	RemoteWriteMessage     remote.WriteMessageType
}

// RunCommand runs the given command with the given args until context is done.
//
// This is useful when starting process-based sender targets.
func RunCommand(ctx context.Context, dir string, extraEnvVars []string, prog string, args ...string) error {
	output := io.Discard
	// Suppress output to avoid cluttering test results.
	suppressOutput := os.Getenv("DEBUG") == ""
	if suppressOutput {
		output = io.Discard
	} else {
		output = os.Stdout
	}

	cmd := exec.Command(prog, args...)
	// Required for group process signalling on close.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Dir = dir
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, extraEnvVars...)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		return err
	}

	cmdStopped := make(chan error)
	defer close(cmdStopped)
	go func() {
		cmdStopped <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Use group process ID. This allows using actually passing signal correctly
		// to all processes when the parent does not support it (e.g. go run).
		pgid, err := syscall.Getpgid(cmd.Process.Pid)
		if err != nil {
			return fmt.Errorf("failed to get pgid: %w", err)
		}
		if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
			return fmt.Errorf("failed to send signal: %w", err)
		}
		return <-cmdStopped
	case err := <-cmdStopped:
		return err
	}
}

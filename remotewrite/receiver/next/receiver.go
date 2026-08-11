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

// Package receiver provides a programmatic Remote Write receiver compliance
// test suite, so that it can be imported and run against a target Receiver
// implementation (e.g. an in-process or subprocess Prometheus build), mirroring
// the pattern used by the sibling package "sender".
//
// NOTE: this package currently lives at remotewrite/receiver/next as a staging
// location so it doesn't collide with the existing remotewrite/receiver
// (package main) suite while under review (see #<PR>). Once the conversion of
// the remaining test files is complete and reviewed, this should move up to
// remotewrite/receiver, replacing the old package main suite.
package receiver

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
)

// Receiver represents a Remote Write receiver under test.
// It generally needs to accept Prometheus Remote Write (RW1 and/or RW2)
// requests on an HTTP endpoint.
type Receiver interface {
	// Name returns a unique receiver name.
	Name() string
	// Run starts the receiver under test until ctx is done. Once the receiver
	// is ready to accept remote write requests, Run must invoke ready exactly
	// once with the base URL of its remote-write endpoint.
	// Premature stops (before ctx is done) are assumed to be failures.
	Run(ctx context.Context, ready func(remoteWriteURL string)) error
}

// RunCommand runs the given command with the given args until context is done.
//
// This is useful when starting process-based receiver targets.
func RunCommand(ctx context.Context, dir string, extraEnvVars []string, prog string, args ...string) error {
	output := io.Discard
	// Suppress output to avoid cluttering test results.
	if os.Getenv("DEBUG") != "" {
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

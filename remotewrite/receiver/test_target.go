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

package receiver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	cconfig "github.com/prometheus/common/config"
	"github.com/prometheus/compliance/common"
	"github.com/prometheus/prometheus/config"
)

const receiverConfig = `
storage:
  exemplars:
    max_exemplars: 100000
`

// Receiver represents a Remote-Write receiver target under test.
type Receiver interface {
	// Name returns a unique receiver name.
	Name() string
	// Run runs a test receiver listening on Options.ReceiveHostPort until the context is done.
	// Premature stops are assumed to be failures.
	Run(ctx context.Context, opts Options) error
}

// Options represents the receiver options for a test run.
type Options struct {
	ReceiveHostPort string
}

// RunCommand runs the given command with the given args until context is done.
//
// This is useful when starting process-based receiver targets.
func RunCommand(ctx context.Context, dir string, extraEnvVars []string, prog string, args ...string) error {
	return common.RunCommand(ctx, dir, extraEnvVars, prog, args...)
}

type prometheusReceiver struct {
	name   string
	binary string
}

// NewPrometheus returns a Receiver that runs a Prometheus process with the Remote-Write receiver enabled.
// If binary is empty, the default Prometheus binary from common.GetPrometheusBinary is used.
func NewPrometheus(name, binary string) Receiver {
	return prometheusReceiver{
		name:   name,
		binary: binary,
	}
}

func (p prometheusReceiver) Name() string {
	if p.name != "" {
		return p.name
	}
	return "prometheus"
}

func (p prometheusReceiver) Run(ctx context.Context, opts Options) error {
	binary := p.binary
	if binary == "" {
		var err error
		binary, err = common.GetPrometheusBinary()
		if err != nil {
			return err
		}
	}

	dir, err := os.MkdirTemp("", "test-receiver-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(receiverConfig), 0o600); err != nil {
		return err
	}

	return RunCommand(ctx, ".", nil,
		binary,
		fmt.Sprintf("--web.listen-address=%s", opts.ReceiveHostPort),
		"--web.enable-remote-write-receiver",
		fmt.Sprintf("--storage.tsdb.path=%v", dir),
		fmt.Sprintf("--config.file=%s", configFile),
		"--enable-feature=st-storage,exemplar-storage,metadata-wal-records",
	)
}

var _ Receiver = prometheusReceiver{}

func isReceiverReachable(u *url.URL) bool {
	if u == nil || u.Host == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", u.Host, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func startReceiver(ctx context.Context, r Receiver) (*config.RemoteWriteConfig, func(), error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find free port: %w", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- r.Run(runCtx, Options{ReceiveHostPort: addr})
	}()

	cleanup := func() {
		cancel()
		<-done
	}

	readyURL := fmt.Sprintf("http://%s/-/ready", addr)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case runErr := <-done:
			return nil, nil, fmt.Errorf("receiver %s exited prematurely: %w", r.Name(), runErr)
		default:
		}

		resp, reqErr := http.Get(readyURL)
		if reqErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				u, parseErr := url.Parse(fmt.Sprintf("http://%s/api/v1/write", addr))
				if parseErr != nil {
					cleanup()
					return nil, nil, parseErr
				}
				return &config.RemoteWriteConfig{
					Name: r.Name(),
					URL:  &cconfig.URL{URL: u},
				}, cleanup, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	cleanup()
	return nil, nil, fmt.Errorf("timed out waiting for receiver %s at %s to become ready", r.Name(), readyURL)
}

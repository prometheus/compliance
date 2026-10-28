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

package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/prometheus/compliance/common"
)

const (
	// TODO(bwplotka): Switch back to prometheusDownloadURL once a Prometheus release includes https://github.com/prometheus/prometheus/pull/20098.
	prometheusDownloadURL = ""
	prometheusGitRef      = "6f22682586f217ba0e1172f8111a61db47179798" // https://github.com/prometheus/prometheus/pull/20098
	receiverConfig        = `
storage:
  exemplars:
    max_exemplars: 100000
`
)

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

// startLocalPrometheusReceiver auto-downloads or builds the Prometheus binary
// (see prometheusDownloadURL and prometheusGitRef) and runs it with the
// Remote-Write receiver enabled until cleanup is called.
func startLocalPrometheusReceiver(ctx context.Context) (*url.URL, func(), error) {
	binary, err := getPrometheusBinary()
	if err != nil {
		return nil, nil, err
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find free port: %w", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	dir, err := os.MkdirTemp("", "test-receiver-*")
	if err != nil {
		return nil, nil, err
	}

	configFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(configFile, []byte(receiverConfig), 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, nil, err
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- common.RunCommand(runCtx, ".", nil,
			binary,
			fmt.Sprintf("--web.listen-address=%s", addr),
			"--web.enable-remote-write-receiver",
			fmt.Sprintf("--storage.tsdb.path=%v", dir),
			fmt.Sprintf("--config.file=%s", configFile),
			"--enable-feature=st-storage,exemplar-storage,metadata-wal-records",
		)
	}()

	cleanup := func() {
		cancel()
		<-done
		_ = os.RemoveAll(dir)
	}

	readyURL := fmt.Sprintf("http://%s/-/ready", addr)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case runErr := <-done:
			_ = os.RemoveAll(dir)
			return nil, nil, fmt.Errorf("prometheus exited prematurely: %w", runErr)
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
				return u, cleanup, nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	cleanup()
	return nil, nil, fmt.Errorf("timed out waiting for prometheus receiver at %s to become ready", readyURL)
}

func getPrometheusBinary() (string, error) {
	if prometheusDownloadURL != "" {
		return common.DownloadBinary(prometheusDownloadURL, "prometheus")
	}
	return common.BuildPrometheusBinary(prometheusGitRef)
}

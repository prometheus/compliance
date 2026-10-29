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

package receiver_test

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/prometheus/compliance"
	"github.com/prometheus/compliance/remotewrite/receiver"
)

const receiverConfig = `
storage:
  exemplars:
    max_exemplars: 100000
`

type prometheus struct {
	name string
}

func (p prometheus) Name() string {
	if p.name != "" {
		return p.name
	}
	return "prometheus"
}

// Run runs a Prometheus process with the Remote-Write receiver enabled until ctx is done.
//
// It auto-downloads or builds the Prometheus binary (see compliance.GetPrometheusBinary).
func (p prometheus) Run(ctx context.Context, opts receiver.Options) error {
	binary, err := compliance.GetPrometheusBinary()
	if err != nil {
		return err
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

	return receiver.RunCommand(ctx, ".", nil,
		binary,
		fmt.Sprintf("--web.listen-address=%s", opts.ReceiveHostPort),
		"--web.enable-remote-write-receiver",
		fmt.Sprintf("--storage.tsdb.path=%v", dir),
		fmt.Sprintf("--config.file=%s", configFile),
		"--enable-feature=st-storage,exemplar-storage,metadata-wal-records",
	)
}

var _ receiver.Receiver = prometheus{}

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

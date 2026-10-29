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

// Package receiver provides the Prometheus Remote-Write 2.0 receiver compliance test suite.
package receiver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	cconfig "github.com/prometheus/common/config"
	"github.com/prometheus/compliance"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage/remote/googleiam"
	"github.com/prometheus/sigv4"
)

// Receiver represents a Remote-Write receiver target under test.
// Receiver must listen for incoming HTTP Remote-Write requests on Options.ReceiveHostPort
// until the passed context is cancelled.
type Receiver = compliance.Target[Options]

// Options configures a Receiver instance for a compliance test run.
type Options struct {
	// ReceiveHostPort is the "host:port" address where the receiver must accept HTTP requests.
	ReceiveHostPort string
}

// RunCommand runs prog with args until ctx is cancelled.
//
// This is useful when starting process-based receiver targets.
func RunCommand(ctx context.Context, dir string, extraEnvVars []string, prog string, args ...string) error {
	return compliance.RunCommand(ctx, dir, extraEnvVars, prog, args...)
}

// endpointOverride is implemented by receivers backed by a pre-running external HTTP endpoint.
type endpointOverride interface {
	endpoint() (*url.URL, *http.Client)
}

type remoteWriteConfigReceiver struct {
	name   string
	url    *url.URL
	client *http.Client
}

// NewRemoteWriteConfigReceiver wraps a Prometheus RemoteWriteConfig (supporting TLS, BasicAuth,
// OAuth2, SigV4, Google IAM, and custom HTTP headers) as a Receiver target.
func NewRemoteWriteConfigReceiver(rw *config.RemoteWriteConfig) (Receiver, error) {
	if rw == nil {
		return nil, errors.New("remote write config must not be nil")
	}
	if rw.URL == nil || rw.URL.URL == nil {
		return nil, fmt.Errorf("remote write config %q has nil URL", rw.Name)
	}

	transport, err := cconfig.NewRoundTripperFromConfig(rw.HTTPClientConfig, "remote_write")
	if err != nil {
		return nil, fmt.Errorf("create round tripper for %q: %w", rw.Name, err)
	}
	if rw.SigV4Config != nil {
		transport, err = sigv4.NewSigV4RoundTripper(rw.SigV4Config, transport)
		if err != nil {
			return nil, fmt.Errorf("create SigV4 round tripper for %q: %w", rw.Name, err)
		}
	}
	if rw.GoogleIAMConfig != nil {
		transport, err = googleiam.NewRoundTripper(rw.GoogleIAMConfig, transport)
		if err != nil {
			return nil, fmt.Errorf("create Google IAM round tripper for %q: %w", rw.Name, err)
		}
	}
	if len(rw.Headers) > 0 {
		transport = &injectHeadersRoundTripper{headers: rw.Headers, RoundTripper: transport}
	}

	name := rw.Name
	if name == "" {
		name = rw.URL.Host
	}

	return &remoteWriteConfigReceiver{
		name:   name,
		url:    rw.URL.URL,
		client: &http.Client{Transport: transport, Timeout: 30 * time.Second},
	}, nil
}

func (r *remoteWriteConfigReceiver) Name() string { return r.name }

func (r *remoteWriteConfigReceiver) Run(ctx context.Context, _ Options) error {
	<-ctx.Done()
	return nil
}

func (r *remoteWriteConfigReceiver) endpoint() (*url.URL, *http.Client) {
	return r.url, r.client
}

// injectHeadersRoundTripper injects static HTTP headers into each outgoing request.
type injectHeadersRoundTripper struct {
	headers map[string]string
	http.RoundTripper
}

func (t *injectHeadersRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}
	return t.RoundTripper.RoundTrip(req)
}

// FilterRemoteWriteConfigs returns the subset of configs whose Name matches one of the
// comma-separated names in receiverNames.
func FilterRemoteWriteConfigs(configs []*config.RemoteWriteConfig, receiverNames string) []*config.RemoteWriteConfig {
	allowed := make(map[string]bool)
	for name := range strings.SplitSeq(receiverNames, ",") {
		name = strings.TrimSpace(name)
		if name != "" {
			allowed[name] = true
		}
	}

	var filtered []*config.RemoteWriteConfig
	for _, cfg := range configs {
		if allowed[cfg.Name] {
			filtered = append(filtered, cfg)
		}
	}
	return filtered
}

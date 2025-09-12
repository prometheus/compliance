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
	"bytes"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	cconfig "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/config"
	"github.com/stretchr/testify/require"
)

// remoteWriteConfigs holds the Remote-Write configurations loaded from the Prometheus config file.
var remoteWriteConfigs []*config.RemoteWriteConfig

// TestMain sets up the test environment by loading configuration and filtering receivers.
// It loads the Prometheus configuration file specified by PROMETHEUS_RW2_COMPLIANCE_CONFIG_FILE
// environment variable (or falls back to config.yml/config_example.yml) and extracts remote write
// configurations. If PROMETHEUS_RW2_COMPLIANCE_RECEIVERS is set, only matching receivers are tested.
func TestMain(m *testing.M) {
	configFile := os.Getenv("PROMETHEUS_RW2_COMPLIANCE_CONFIG_FILE")
	if configFile == "" {
		if _, err := os.Stat("config.yml"); err == nil {
			configFile = "config.yml"
		} else {
			configFile = "config_example.yml"
		}
	}

	promConfig, err := config.LoadFile(configFile, false, nil)
	if err != nil {
		log.Printf("failed to load config file: %s", err.Error())
		os.Exit(1)
	}

	// Extract Remote-Write configs.
	remoteWriteConfigs = promConfig.RemoteWriteConfigs

	// Filter receivers based on PROMETHEUS_RW2_COMPLIANCE_RECEIVERS environment variable.
	receiverNames := os.Getenv("PROMETHEUS_RW2_COMPLIANCE_RECEIVERS")
	if receiverNames != "" {
		filteredConfigs := filterReceivers(remoteWriteConfigs, receiverNames)
		if len(filteredConfigs) > 0 {
			remoteWriteConfigs = filteredConfigs
			log.Printf("Using filtered receivers: %s", receiverNames)
		} else {
			log.Printf("No receivers found matching %q", receiverNames)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

// filterReceivers filters Remote-Write configs based on comma-separated receiver names.
// It returns a subset of configs that match the provided receiver names.
// Pre-requisite: receiverNames is not empty.
func filterReceivers(configs []*config.RemoteWriteConfig, receiverNames string) []*config.RemoteWriteConfig {
	receiverList := strings.Split(receiverNames, ",")
	receiverMap := make(map[string]bool)
	for _, receiver := range receiverList {
		receiver = strings.TrimSpace(receiver)
		if receiver != "" {
			receiverMap[receiver] = true
		}
	}

	var filtered []*config.RemoteWriteConfig
	for _, config := range configs {
		if receiverMap[config.Name] {
			filtered = append(filtered, config)
		}
	}

	return filtered
}

// newInjectHeadersRoundTripper creates a new injectHeadersRoundTripper that wraps
// the underlying RoundTripper and injects the provided headers into each request.
func newInjectHeadersRoundTripper(h map[string]string, underlyingRT http.RoundTripper) *injectHeadersRoundTripper {
	return &injectHeadersRoundTripper{headers: h, RoundTripper: underlyingRT}
}

// injectHeadersRoundTripper is an http.RoundTripper that injects headers into
// each HTTP request before forwarding it to the underlying RoundTripper.
type injectHeadersRoundTripper struct {
	headers map[string]string
	http.RoundTripper
}

// RoundTrip injects the configured headers into the request and forwards it
// to the underlying RoundTripper.
func (t *injectHeadersRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for key, value := range t.headers {
		req.Header.Set(key, value)
	}
	return t.RoundTripper.RoundTrip(req)
}

// forEachRemoteWriteConfig runs the provided function for each Remote-Write config.
func forEachRemoteWriteConfig(t *testing.T, f func(*testing.T, *config.RemoteWriteConfig)) {
	for _, rw := range remoteWriteConfigs {
		t.Run(rw.Name, func(t *testing.T) {
			t.Attr("rw", rw.Name)
			f(t, rw)
		})
	}
}

// runRequest runs the request for each Remote-Write config.
func runRequest(t *testing.T, req *http.Request, expectedResp requestParams) {
	forEachRemoteWriteConfig(t, func(t *testing.T, rw *config.RemoteWriteConfig) {
		transport, err := cconfig.NewRoundTripperFromConfig(rw.HTTPClientConfig, "remote_write")
		require.NoError(t, err, "failed to create round tripper for Remote-Write config %q", rw.Name)

		if len(rw.Headers) > 0 {
			transport = newInjectHeadersRoundTripper(rw.Headers, transport)
		}

		client := &http.Client{Transport: transport}

		var bodyCopy io.ReadCloser
		if req.Body != nil {
			bodyBytes, err := io.ReadAll(req.Body)
			require.NoError(t, err, "failed to read request body for cloning")
			req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			bodyCopy = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}
		reqC := req.Clone(t.Context())
		reqC.Body = bodyCopy

		reqC.URL = rw.URL.URL
		resp, err := client.Do(reqC)
		require.NoError(t, err, "request failed for Remote-Write config %q", rw.Name)
		defer resp.Body.Close()

		respBody, err := io.ReadAll(resp.Body)
		require.NoError(t, err, "failed to read response body")
		t.Logf("Response code from %q: %d", rw.Name, resp.StatusCode)
		if len(respBody) > 0 {
			t.Logf("Response body from %q: %q", rw.Name, string(respBody))
		}

		validateResponse(t, expectedResp, resp)
	})
}

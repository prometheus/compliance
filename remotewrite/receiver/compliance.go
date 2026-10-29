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
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/compliance"
	"github.com/stretchr/testify/require"
)

// ComplianceTests returns the official Remote-Write 2.0 receiver compliance tests.
func ComplianceTests() (ret []Test) {
	ret = append(ret, samplesTests()...)
	ret = append(ret, histogramsTests()...)
	ret = append(ret, exemplarsTests()...)
	ret = append(ret, metadataTests()...)
	ret = append(ret, combinedTests()...)
	ret = append(ret, requestTests()...)
	ret = append(ret, rw1CompatTests()...)
	ret = append(ret, modesTests()...)
	return ret
}

// RFCLevel represents the compliance requirement level of a test case.
type RFCLevel = compliance.RFCLevel

const (
	MustLevel        = compliance.MustLevel
	ShouldLevel      = compliance.ShouldLevel
	MayLevel         = compliance.MayLevel
	RecommendedLevel = compliance.RecommendedLevel
)

// ResponseResult captures an HTTP response returned by a receiver under test.
type ResponseResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// ValidateFunc validates a captured ResponseResult.
type ValidateFunc = compliance.ValidateFunc[ResponseResult]

// ValidateCase defines a validation subtest for a captured ResponseResult.
type ValidateCase = compliance.ValidateCase[ResponseResult]

// RequestBuilder builds an HTTP Remote-Write request, merging externalLabels into non-empty series label sets.
type RequestBuilder func(externalLabels map[string]string) (*http.Request, error)

// Test defines a compliance test scenario to run against a Remote-Write receiver.
type Test struct {
	// Name is a unique slash-separated subtest path for this test case.
	Name string
	// Description describes what this test case verifies.
	Description string
	// RFCLevel is the compliance requirement level when Validate is used without ValidateCases.
	RFCLevel RFCLevel
	// SetupRequests are optional prerequisite requests executed sequentially before BuildRequest
	// (for example, committing an initial sample before testing out-of-order rejection).
	SetupRequests []RequestBuilder
	// BuildRequest constructs the HTTP request sent to the receiver under test.
	BuildRequest RequestBuilder
	// Validate checks the HTTP response when the test has a single RFC requirement level.
	Validate ValidateFunc
	// ValidateCases configures multiple validation subtests (for example, MUST 2xx and SHOULD 204)
	// sharing the same HTTP request execution.
	ValidateCases []ValidateCase
}

// RunOption customizes a RunTests execution.
type RunOption func(*runConfig)

type runConfig struct {
	externalLabels map[string]string
	writePath      string
}

// WithExternalLabels merges lbls into every non-empty time series label set generated during RunTests
// without overwriting labels explicitly defined by a test case.
func WithExternalLabels(lbls map[string]string) RunOption {
	return func(cfg *runConfig) {
		if len(lbls) == 0 {
			cfg.externalLabels = nil
			return
		}
		cfg.externalLabels = maps.Clone(lbls)
	}
}

// WithWritePath overrides the URL path appended to Options.ReceiveHostPort (default "/api/v1/write").
func WithWritePath(path string) RunOption {
	return func(cfg *runConfig) {
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		cfg.writePath = path
	}
}

// RunTests starts (or connects to) rec and executes all compliance test cases in tcs against it.
func RunTests(t *testing.T, rec Receiver, tcs []Test, opts ...RunOption) {
	t.Helper()

	require.NotNil(t, rec)
	require.NotEmpty(t, tcs)

	rcfg := &runConfig{
		writePath: "/api/v1/write",
	}
	for _, o := range opts {
		o(rcfg)
	}

	var (
		writeURL   *url.URL
		httpClient *http.Client
		setupOk    bool
	)

	if eo, ok := rec.(endpointOverride); ok {
		writeURL, httpClient = eo.endpoint()
		setupOk = compliance.SafeRun(t, fmt.Sprintf("%s/_setup", rec.Name()), rec.Name(), "Verify configured receiver endpoint", MustLevel, func(t *testing.T) {
			require.NotNil(t, writeURL, "receiver write URL must not be nil")
			require.NotNil(t, httpClient, "receiver HTTP client must not be nil")
		})
	} else {
		hostPort := compliance.FreeLocalHostPort(t)
		parsedURL, err := url.Parse(fmt.Sprintf("http://%s%s", hostPort, rcfg.writePath))
		require.NoError(t, err)
		writeURL = parsedURL
		httpClient = &http.Client{Timeout: 30 * time.Second}

		ctx, cancel := context.WithCancel(t.Context())
		var wg sync.WaitGroup
		t.Cleanup(func() {
			cancel()
			wg.Wait()
		})

		errCh := make(chan error, 1)
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- rec.Run(ctx, Options{ReceiveHostPort: hostPort})
		}()

		setupOk = compliance.SafeRun(t, fmt.Sprintf("%s/_setup", rec.Name()), rec.Name(), "Start receiver and wait until HTTP endpoint is ready", MustLevel, func(t *testing.T) {
			waitCtx, waitCancel := context.WithTimeout(ctx, 30*time.Second)
			defer waitCancel()
			if err := waitReceiverReady(waitCtx, writeURL.String(), errCh, rec.Name()); err != nil {
				t.Fatalf("receiver %q setup failed: %v", rec.Name(), err)
			}
		})
	}

	for _, tc := range tcs {
		if len(tc.ValidateCases) == 0 {
			compliance.SafeRun(t, fmt.Sprintf("%s/%s", rec.Name(), tc.Name), rec.Name(), tc.Description, tc.RFCLevel, func(t *testing.T) {
				if !setupOk {
					t.Fatal("setup failed")
				}
				for i, buildSetup := range tc.SetupRequests {
					setupReq, err := buildSetup(rcfg.externalLabels)
					require.NoError(t, err, "failed to build setup request %d", i)
					setupRes, err := sendRequest(t, httpClient, writeURL, rec.Name(), setupReq)
					require.NoError(t, err, "setup request %d failed", i)
					require.Equal(t, 2, setupRes.StatusCode/100, "setup request %d returned non-2xx status %d", i, setupRes.StatusCode)
				}
				req, err := tc.BuildRequest(rcfg.externalLabels)
				require.NoError(t, err, "failed to build request")
				res, err := sendRequest(t, httpClient, writeURL, rec.Name(), req)
				require.NoError(t, err, "request failed for receiver %q", rec.Name())
				if tc.Validate != nil {
					tc.Validate(t, res)
				}
			})
			continue
		}

		compliance.SafeRun(t, fmt.Sprintf("%s/%s", rec.Name(), tc.Name), rec.Name(), tc.Description, tc.RFCLevel, func(t *testing.T) {
			var res ResponseResult
			reqOk := setupOk
			if reqOk {
				for i, buildSetup := range tc.SetupRequests {
					setupReq, err := buildSetup(rcfg.externalLabels)
					if err != nil {
						t.Errorf("failed to build setup request %d: %v", i, err)
						reqOk = false
						break
					}
					setupRes, err := sendRequest(t, httpClient, writeURL, rec.Name(), setupReq)
					if err != nil || setupRes.StatusCode/100 != 2 {
						t.Errorf("setup request %d failed (status=%d, err=%v)", i, setupRes.StatusCode, err)
						reqOk = false
						break
					}
				}
			}
			if reqOk {
				req, err := tc.BuildRequest(rcfg.externalLabels)
				if err != nil {
					t.Errorf("failed to build request: %v", err)
					reqOk = false
				} else {
					res, err = sendRequest(t, httpClient, writeURL, rec.Name(), req)
					if err != nil {
						t.Errorf("request failed for receiver %q: %v", rec.Name(), err)
						reqOk = false
					}
				}
			}
			compliance.RunValidateCases(t, rec.Name(), reqOk, res, tc.ValidateCases)
		})
	}
}

// waitReceiverReady polls writeURL with an empty POST request until the HTTP handler responds
// with any non-5xx status. Using POST ensures routers with method-scoped readiness middleware
// (such as Prometheus returning 503 on POST /api/v1/write until TSDB opens, then 415 for an empty
// Content-Type) finish initializing before compliance tests begin, without writing any data.
func waitReceiverReady(ctx context.Context, writeURL string, errCh <-chan error, name string) error {
	pollTicker := time.NewTicker(50 * time.Millisecond)
	defer pollTicker.Stop()

	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, writeURL, nil)
		if err != nil {
			return err
		}
		resp, reqErr := client.Do(req)
		if reqErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode < http.StatusInternalServerError {
				return nil
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = reqErr
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for %s at %s (last err: %v): %w", name, writeURL, lastErr, ctx.Err())
		case runErr := <-errCh:
			return fmt.Errorf("%s exited prematurely before becoming ready: %w", name, runErr)
		case <-pollTicker.C:
		}
	}
}

func sendRequest(t *testing.T, client *http.Client, writeURL *url.URL, name string, req *http.Request) (ResponseResult, error) {
	t.Helper()

	var bodyCopy io.ReadCloser
	if req.Body != nil {
		bodyBytes, err := io.ReadAll(req.Body)
		if err != nil {
			return ResponseResult{}, fmt.Errorf("read request body: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		bodyCopy = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	reqC := req.Clone(t.Context())
	reqC.Body = bodyCopy
	reqC.URL = writeURL

	resp, err := client.Do(reqC)
	if err != nil {
		return ResponseResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ResponseResult{}, fmt.Errorf("read response body: %w", err)
	}
	t.Logf("Response code from %q: %d", name, resp.StatusCode)
	if len(respBody) > 0 {
		t.Logf("Response body from %q: %q", name, string(respBody))
	}

	return ResponseResult{
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
		Body:       respBody,
	}, nil
}

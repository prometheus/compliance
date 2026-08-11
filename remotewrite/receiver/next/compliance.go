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
	"net/http"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// defaultReadyTimeout is generous by default because a Receiver implementation
// may need to download or build a binary before it can start (see e.g. the
// release-binary-based prometheus target). Override with
// PROMETHEUS_RW2_COMPLIANCE_READY_TIMEOUT (a value parseable by time.ParseDuration),
// mirroring the sender suite's PROMETHEUS_RW2_COMPLIANCE_TEST_TIMEOUT.
const defaultReadyTimeout = 3 * time.Minute

func readyTimeout() time.Duration {
	if v := os.Getenv("PROMETHEUS_RW2_COMPLIANCE_READY_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultReadyTimeout
}

// ComplianceTests returns the official Remote Write receiver compliance tests.
func ComplianceTests() (ret []Test) {
	ret = append(ret, metricTests()...)
	return ret
}

type RFCLevel string

const (
	MustLevel   RFCLevel = "MUST"
	ShouldLevel RFCLevel = "SHOULD"
	MayLevel    RFCLevel = "MAY"
)

func (r RFCLevel) annotate(t *testing.T) {
	t.Attr("rfcLevel", string(r))
}

// ExpectedResponse describes the expected outcome of sending a request to the receiver.
type ExpectedResponse struct {
	// Samples, Exemplars, Histograms are the expected counts reported via the
	// X-Prometheus-Remote-Write-*-Written response headers.
	Samples    int
	Exemplars  int
	Histograms int
	// ExactStatusCode, if non-zero, is asserted exactly. Otherwise, any 2xx implies
	// success and any 4xx defaults to expecting http.StatusBadRequest.
	ExactStatusCode int
}

// Test defines a single Remote Write receiver compliance test case.
//
// Each Test produces a MUST sub-test that asserts basic (non-strict) compliance,
// plus, when ExpectSuccess is true, an additional SHOULD sub-test that asserts the
// receiver responds with exactly http.StatusNoContent.
type Test struct {
	// Name is a unique name for the test case; adds a "/<name>/" sub-test.
	Name string
	// Description describes what the test case is verifying.
	Description string
	// Opts is the request to send to the receiver under test.
	Opts RequestOpts
	// Expect describes the expected outcome.
	Expect ExpectedResponse
	// ExpectSuccess indicates whether the request as a whole is expected to succeed (2xx).
	ExpectSuccess bool
}

// RunTests starts target and runs each compliance test case against it.
//
// Unlike the sender suite (which restarts a fresh sender per test case), RunTests
// starts the receiver under test once for the whole run: real receivers (e.g. a full
// Prometheus binary) are too costly to restart per case, and these tests only assert
// on the synchronous HTTP response to each write, so a shared, long-lived receiver
// instance is sufficient and does not leak state between assertions.
func RunTests(t *testing.T, target Receiver, tcs []Test) {
	t.Helper()

	require.NotNil(t, target)
	require.NotEmpty(t, tcs)

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	readyCh := make(chan string, 1)
	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- target.Run(ctx, func(url string) {
			select {
			case readyCh <- url:
			default:
			}
		})
	}()

	var baseURL string
	select {
	case baseURL = <-readyCh:
	case err := <-runErrCh:
		t.Fatalf("receiver %q stopped before becoming ready: %v", target.Name(), err)
	case <-time.After(readyTimeout()):
		cancel()
		t.Fatalf("receiver %q did not become ready in time", target.Name())
	}

	client := &http.Client{Timeout: 10 * time.Second}

	for _, tc := range tcs {
		runComplianceTest(t, client, baseURL, target.Name(), tc)
	}
}

// runComplianceTest runs tc against the already-running receiver at baseURL, producing
// both a SHOULD (strict, 204-only) sub-test for expected-success cases and a MUST
// (basic compliance) sub-test for all cases, mirroring the pre-conversion behaviour.
func runComplianceTest(t *testing.T, client *http.Client, baseURL, targetName string, tc Test) {
	t.Helper()

	if tc.ExpectSuccess {
		t.Run(fmt.Sprintf("%s/%s returns 204", targetName, tc.Name), func(t *testing.T) {
			ShouldLevel.annotate(t)
			t.Attr("description", tc.Description)

			expect := tc.Expect
			expect.ExactStatusCode = http.StatusNoContent
			doAndValidate(t, client, baseURL, tc.Opts, expect, true)
		})
	}

	t.Run(fmt.Sprintf("%s/%s", targetName, tc.Name), func(t *testing.T) {
		MustLevel.annotate(t)
		t.Attr("description", tc.Description)

		doAndValidate(t, client, baseURL, tc.Opts, tc.Expect, tc.ExpectSuccess)
	})
}

func doAndValidate(t *testing.T, client *http.Client, baseURL string, opts RequestOpts, expect ExpectedResponse, expectSuccess bool) {
	t.Helper()

	req := generateRequest(opts)
	req.URL = mustParseURL(t, baseURL)

	resp, err := client.Do(req)
	require.NoError(t, err, "request to receiver failed")
	defer resp.Body.Close()

	validateResponse(t, expect, expectSuccess, resp)
}

func getHeaderValue(t *testing.T, header http.Header, key string) int {
	t.Helper()
	v := header.Get("X-Prometheus-Remote-Write-" + key + "-Written")
	if v == "" {
		// Receivers CAN assume that any missing X-Prometheus-Remote-Write-*-Written
		// response header means no element from this category was written (count of 0).
		return 0
	}
	i, err := strconv.Atoi(v)
	require.NoError(t, err)
	return i
}

// validateResponse asserts resp matches expect/expectSuccess, mirroring the RFC-defined
// status code and X-Prometheus-Remote-Write-*-Written header semantics.
func validateResponse(t *testing.T, expect ExpectedResponse, expectSuccess bool, resp *http.Response) {
	t.Helper()

	samplesWritten := getHeaderValue(t, resp.Header, "Samples")
	exemplarsWritten := getHeaderValue(t, resp.Header, "Exemplars")
	histogramsWritten := getHeaderValue(t, resp.Header, "Histograms")

	if expect.ExactStatusCode != 0 {
		require.Equal(t, expect.ExactStatusCode, resp.StatusCode, "response code should be exactly %d", expect.ExactStatusCode)
	}

	switch resp.StatusCode / 100 {
	case 2:
		require.True(t, expectSuccess, "response code is %d but success is false", resp.StatusCode)
		require.Equal(t, expect.Samples, samplesWritten, "%d samples written", samplesWritten)
		require.Equal(t, expect.Exemplars, exemplarsWritten, "%d exemplars written", exemplarsWritten)
		require.Equal(t, expect.Histograms, histogramsWritten, "%d histograms written", histogramsWritten)
	case 4:
		if expect.ExactStatusCode == 0 {
			require.Equal(t, http.StatusBadRequest, resp.StatusCode, "response code should be exactly %d", http.StatusBadRequest)
		}
		require.GreaterOrEqual(t, expect.Samples, samplesWritten, "%d samples written", samplesWritten)
		require.GreaterOrEqual(t, expect.Exemplars, exemplarsWritten, "%d exemplars written", exemplarsWritten)
		require.GreaterOrEqual(t, expect.Histograms, histogramsWritten, "%d histograms written", histogramsWritten)
	case 5:
		require.False(t, expectSuccess, "response code is %d but success is true", resp.StatusCode)
		require.GreaterOrEqual(t, expect.Samples, samplesWritten, "%d samples written", samplesWritten)
		require.GreaterOrEqual(t, expect.Exemplars, exemplarsWritten, "%d exemplars written", exemplarsWritten)
		require.GreaterOrEqual(t, expect.Histograms, histogramsWritten, "%d histograms written", histogramsWritten)
	default:
		require.Fail(t, fmt.Sprintf("response code is %d but should be 2xx, 4xx, or 5xx", resp.StatusCode))
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

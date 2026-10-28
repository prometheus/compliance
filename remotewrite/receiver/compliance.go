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
	"io"
	"net/http"
	"strings"
	"testing"

	cconfig "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage/remote/googleiam"
	"github.com/prometheus/sigv4"
	"github.com/stretchr/testify/require"
)

// remoteWriteConfigs holds the Remote-Write configurations active for the current test execution.
var remoteWriteConfigs []*config.RemoteWriteConfig

// Test defines a receiver compliance test scenario.
type Test struct {
	Name string
	Fn   func(t *testing.T)
}

// ComplianceTests returns all baseline Remote-Write 2.0 receiver compliance tests.
func ComplianceTests() []Test {
	return []Test{
		{Name: "TestSamplesWithMetadataAndExemplars", Fn: TestSamplesWithMetadataAndExemplars},
		{Name: "TestHistogramsWithMetadataAndExemplars", Fn: TestHistogramsWithMetadataAndExemplars},
		{Name: "TestComplexSamplesRequest", Fn: TestComplexSamplesRequest},
		{Name: "TestComplexHistogramsRequest", Fn: TestComplexHistogramsRequest},
		{Name: "TestMultipleMetricFamiliesSamples", Fn: TestMultipleMetricFamiliesSamples},
		{Name: "TestMultipleMetricFamiliesHistograms", Fn: TestMultipleMetricFamiliesHistograms},
		{Name: "TestEdgeCaseCombinations", Fn: TestEdgeCaseCombinations},
		{Name: "TestManySamples", Fn: TestManySamples},
		{Name: "TestManyHistograms", Fn: TestManyHistograms},
		{Name: "TestSpecialCharactersCombined", Fn: TestSpecialCharactersCombined},
		{Name: "TestCustomBucketsHistogramWithExemplar", Fn: TestCustomBucketsHistogramWithExemplar},
		{Name: "TestExemplars", Fn: TestExemplars},
		{Name: "TestExemplarWithCreatedTimestamp", Fn: TestExemplarWithCreatedTimestamp},
		{Name: "TestMultipleExemplars", Fn: TestMultipleExemplars},
		{Name: "TestExemplarTimestamps", Fn: TestExemplarTimestamps},
		{Name: "TestExemplarValidation", Fn: TestExemplarValidation},
		{Name: "TestExemplarSpecialValues", Fn: TestExemplarSpecialValues},
		{Name: "TestHistograms", Fn: TestHistograms},
		{Name: "TestHistogramWithCreatedTimestamp", Fn: TestHistogramWithCreatedTimestamp},
		{Name: "TestMultipleHistograms", Fn: TestMultipleHistograms},
		{Name: "TestEmptyHistogram", Fn: TestEmptyHistogram},
		{Name: "TestHistogramWithMetadata", Fn: TestHistogramWithMetadata},
		{Name: "TestHistogramWithExemplar", Fn: TestHistogramWithExemplar},
		{Name: "TestHistogramWithMetadataAndExemplar", Fn: TestHistogramWithMetadataAndExemplar},
		{Name: "TestMultipleHistogramsWithMetadata", Fn: TestMultipleHistogramsWithMetadata},
		{Name: "TestFloatHistograms", Fn: TestFloatHistograms},
		{Name: "TestHistogramResetHintsAndSchemas", Fn: TestHistogramResetHintsAndSchemas},
		{Name: "TestInvalidHistogramsRejected", Fn: TestInvalidHistogramsRejected},
		{Name: "TestMetadata", Fn: TestMetadata},
		{Name: "TestCounterMetadataWithCreatedTimestamp", Fn: TestCounterMetadataWithCreatedTimestamp},
		{Name: "TestMetadataWithSamples", Fn: TestMetadataWithSamples},
		{Name: "TestMultipleMetadata", Fn: TestMultipleMetadata},
		{Name: "TestMetadataWithComplexLabels", Fn: TestMetadataWithComplexLabels},
		{Name: "TestMetadataValidation", Fn: TestMetadataValidation},
		{Name: "TestSingleMetric", Fn: TestSingleMetric},
		{Name: "TestCounterWithCreatedTimestamp", Fn: TestCounterWithCreatedTimestamp},
		{Name: "TestMultipleMetrics", Fn: TestMultipleMetrics},
		{Name: "TestChronologicalSamplesInSingleTimeSeries", Fn: TestChronologicalSamplesInSingleTimeSeries},
		{Name: "TestOutOfOrderSamplesRejected", Fn: TestOutOfOrderSamplesRejected},
		{Name: "TestSymbolTableAndLabelRefsValidation", Fn: TestSymbolTableAndLabelRefsValidation},
		{Name: "TestContentTypeModes", Fn: TestContentTypeModes},
		{Name: "TestBadContentTypes", Fn: TestBadContentTypes},
		{Name: "TestBadContentTypeModes", Fn: TestBadContentTypeModes},
		{Name: "TestUnsupportedContentTypeAcceptHeader", Fn: TestUnsupportedContentTypeAcceptHeader},
		{Name: "TestBadContentEncodings", Fn: TestBadContentEncodings},
		{Name: "TestUnsupportedContentEncodingAcceptEncodingHeader", Fn: TestUnsupportedContentEncodingAcceptEncodingHeader},
		{Name: "TestCorruptRequestBody", Fn: TestCorruptRequestBody},
		{Name: "TestSnappyFramedFormatRejected", Fn: TestSnappyFramedFormatRejected},
		{Name: "TestUncompressedProtobufRejected", Fn: TestUncompressedProtobufRejected},
		{Name: "TestInvalidProtobufBodyRejected", Fn: TestInvalidProtobufBodyRejected},
		{Name: "TestUnsupportedHTTPMethods", Fn: TestUnsupportedHTTPMethods},
		{Name: "TestRW1BasicCompatibility", Fn: TestRW1BasicCompatibility},
		{Name: "TestRW1ErrorHandling", Fn: TestRW1ErrorHandling},
	}
}

// StatelessComplianceTests returns compliance receiver tests for the "stateless"
// Remote-Write 2.0 mode.
//
// See: https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/#stateless
func StatelessComplianceTests() []Test {
	return []Test{
		{Name: "TestStatelessMode", Fn: TestStatelessMode},
		{Name: "TestStatelessModeValidation", Fn: TestStatelessModeValidation},
	}
}

// StatelessSTComplianceTests returns compliance receiver tests for the "stateless-st"
// Remote-Write 2.0 mode.
//
// See: https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/#stateless-st
func StatelessSTComplianceTests() []Test {
	return []Test{
		{Name: "TestStatelessSTMode", Fn: TestStatelessSTMode},
		{Name: "TestStatelessSTModeValidation", Fn: TestStatelessSTModeValidation},
	}
}

// RunTests starts the given Receiver target and runs the provided compliance tests against it.
func RunTests(t *testing.T, r Receiver, tcs []Test) {
	t.Helper()
	require.NotNil(t, r)
	require.NotEmpty(t, tcs)

	rwCfg, cleanup, err := startReceiver(t.Context(), r)
	require.NoError(t, err)
	t.Cleanup(cleanup)

	RunTestsWithConfig(t, []*config.RemoteWriteConfig{rwCfg}, tcs)
}

// RunTestsWithConfig runs the provided compliance tests against the given Remote-Write configurations.
func RunTestsWithConfig(t *testing.T, rwConfigs []*config.RemoteWriteConfig, tcs []Test) {
	t.Helper()
	require.NotEmpty(t, rwConfigs)
	require.NotEmpty(t, tcs)

	prev := remoteWriteConfigs
	remoteWriteConfigs = rwConfigs
	t.Cleanup(func() {
		remoteWriteConfigs = prev
	})

	for _, tc := range tcs {
		t.Run(tc.Name, tc.Fn)
	}
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

		// Add SigV4 signing if configured
		if rw.SigV4Config != nil {
			transport, err = sigv4.NewSigV4RoundTripper(rw.SigV4Config, transport)
			require.NoError(t, err, "failed to create SigV4 round tripper for Remote-Write config %q", rw.Name)
		}

		// Add Google IAM authentication if configured
		if rw.GoogleIAMConfig != nil {
			transport, err = googleiam.NewRoundTripper(rw.GoogleIAMConfig, transport)
			require.NoError(t, err, "failed to create Google IAM round tripper for Remote-Write config %q", rw.Name)
		}

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

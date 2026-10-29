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

package openmetrics

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sync"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/compliance"
	"github.com/stretchr/testify/require"
)

// ComplianceTests returns all official OpenMetrics 1.0 and 2.0 compliance tests.
func ComplianceTests() []Test {
	ret := make([]Test, 0, 32)
	ret = append(ret, V1ComplianceTests()...)
	ret = append(ret, V2ComplianceTests()...)
	return ret
}

// V1ComplianceTests returns official OpenMetrics 1.0.0 compliance tests.
//
// See: https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md
func V1ComplianceTests() []Test {
	return complianceTestsForVersion(Version1_0_0)
}

// V2ComplianceTests returns official OpenMetrics 2.0.0 compliance tests.
//
// See: https://github.com/prometheus/OpenMetrics/blob/main/specification/OpenMetrics.md
func V2ComplianceTests() []Test {
	return complianceTestsForVersion(Version2_0_0)
}

func complianceTestsForVersion(v Version) []Test {
	var ret []Test
	ret = append(ret, protocolTests(v)...)
	ret = append(ret, syntaxTests(v)...)
	ret = append(ret, metadataTests(v)...)
	ret = append(ret, escapingTests(v)...)
	ret = append(ret, samplesTests(v)...)
	ret = append(ret, histogramsTests(v)...)
	ret = append(ret, exemplarsTests(v)...)
	for i := range ret {
		if ret[i].Version == "" {
			ret[i].Version = v
		}
	}
	return ret
}

// ScrapeRequest configures the HTTP request sent to the target's metrics endpoint.
type ScrapeRequest struct {
	// Accept overrides the HTTP Accept header. If empty, defaults to Version.AcceptHeader(Escaping).
	Accept string
	// Escaping specifies the escaping scheme parameter (e.g. "allow-utf-8", "underscores", "dots", "values")
	// appended to the default Accept header when Accept is empty.
	Escaping string
	// AcceptEncoding specifies the HTTP Accept-Encoding header (e.g. "gzip").
	AcceptEncoding string
	// Headers specifies additional HTTP request headers.
	Headers map[string]string
}

// ScrapeResult holds the captured HTTP response and decoded OpenMetrics data from a scrape.
type ScrapeResult struct {
	// Version is the OpenMetrics specification version configured for this test.
	Version Version
	// Request is the HTTP scrape request configuration that was sent.
	Request ScrapeRequest
	// StatusCode is the HTTP status code returned by the target.
	StatusCode int
	// Headers are the HTTP response headers returned by the target.
	Headers http.Header
	// ContentType is the raw Content-Type response header.
	ContentType string
	// MediaType is the parsed media type from Content-Type.
	MediaType string
	// MediaParams are the parsed parameters from Content-Type (e.g. "version", "charset", "escaping").
	MediaParams map[string]string
	// MediaParseErr is any error encountered when parsing Content-Type with mime.ParseMediaType.
	MediaParseErr error
	// RawBody is the raw HTTP response payload before any decompression.
	RawBody []byte
	// Body is the uncompressed OpenMetrics text payload.
	Body []byte
	// DecompressErr is any error encountered when decompressing RawBody.
	DecompressErr error
	// Families maps metric family name to decoded MetricFamily.
	Families map[string]*dto.MetricFamily
	// OrderedFamilies contains the decoded MetricFamilies in wire order.
	OrderedFamilies []*dto.MetricFamily
	// ParseErr is any syntax (ABNF) or semantic error returned by the OpenMetrics decoder.
	ParseErr error
}

// Test defines a compliance test scenario to run against a Target.
type Test struct {
	// Name is a unique name for the test case.
	Name string
	// Description describes the specification requirement being tested.
	Description string
	// RFCLevel is the requirement level for the test to pass.
	RFCLevel compliance.RFCLevel
	// Version is the OpenMetrics specification version to test.
	Version Version
	// Request configures the HTTP scrape request headers.
	Request ScrapeRequest
	// Validate validates the scrape result in the setup stage.
	Validate compliance.ValidateFunc[ScrapeResult]
	// ValidateCases configures sub-cases sharing the same scrape result.
	ValidateCases []compliance.ValidateCase[ScrapeResult]
}

// RunTests starts target once, waits for its metrics endpoint to become ready,
// and runs all provided OpenMetrics compliance test cases against it.
func RunTests(t *testing.T, target Target, tcs []Test) {
	t.Helper()

	require.NotNil(t, target)
	require.NotEmpty(t, tcs)

	hostPort := compliance.FreeLocalHostPort(t)
	const metricsPath = "/metrics"
	endpointURL := fmt.Sprintf("http://%s%s", hostPort, metricsPath)

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
		errCh <- target.Run(ctx, Options{
			MetricsHostPort: hostPort,
			MetricsPath:     metricsPath,
		})
	}()

	// Use DisableCompression so tests can inspect raw Content-Encoding headers and wire bytes directly.
	client := &http.Client{
		Transport: &http.Transport{
			DisableCompression: true,
		},
		Timeout: 10 * time.Second,
	}

	readyOk := compliance.SafeRun(t, fmt.Sprintf("%s/_ready", target.Name()), target.Name(), "Wait for target metrics endpoint to become reachable", compliance.MustLevel, func(t *testing.T) {
		waitCtx, waitCancel := context.WithTimeout(ctx, 30*time.Second)
		defer waitCancel()
		if err := compliance.WaitForHTTPReady(waitCtx, endpointURL, errCh, target.Name()); err != nil {
			t.Fatalf("target %q failed to become ready at %s: %v", target.Name(), endpointURL, err)
		}
	})

	for _, tc := range tcs {
		versionShort := tc.Version.shortName()
		t.Run(fmt.Sprintf("%s/%s/%s", target.Name(), tc.Name, versionShort), func(t *testing.T) {
			t.Parallel()
			t.Attr("version", versionShort)

			var res ScrapeResult
			setupOk := compliance.SafeRun(t, "_setup", target.Name(), tc.Description, tc.RFCLevel, func(t *testing.T) {
				if !readyOk {
					t.Fatal("target readiness failed")
				}
				var err error
				res, err = executeScrape(t.Context(), client, endpointURL, tc)
				require.NoError(t, err, "HTTP scrape request to %s failed", endpointURL)

				if tc.Validate != nil {
					tc.Validate(t, res)
				}
			})

			compliance.RunValidateCases(t, target.Name(), setupOk, res, tc.ValidateCases)
		})
	}
}

func executeScrape(ctx context.Context, client *http.Client, endpointURL string, tc Test) (ScrapeResult, error) {
	reqCfg := tc.Request
	if reqCfg.Accept == "" {
		reqCfg.Accept = tc.Version.AcceptHeader(reqCfg.Escaping)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpointURL, nil)
	if err != nil {
		return ScrapeResult{}, err
	}
	req.Header.Set("Accept", reqCfg.Accept)
	if reqCfg.AcceptEncoding != "" {
		req.Header.Set("Accept-Encoding", reqCfg.AcceptEncoding)
	}
	for k, v := range reqCfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return ScrapeResult{}, err
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ScrapeResult{}, fmt.Errorf("failed to read response body: %w", err)
	}

	res := ScrapeResult{
		Version:     tc.Version,
		Request:     reqCfg,
		StatusCode:  resp.StatusCode,
		Headers:     resp.Header.Clone(),
		ContentType: resp.Header.Get("Content-Type"),
		RawBody:     rawBody,
		Body:        rawBody,
		Families:    make(map[string]*dto.MetricFamily),
	}

	res.MediaType, res.MediaParams, res.MediaParseErr = mime.ParseMediaType(res.ContentType)

	if resp.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(bytes.NewReader(rawBody))
		if err != nil {
			res.DecompressErr = err
			return res, nil
		}
		decompressed, err := io.ReadAll(gz)
		_ = gz.Close()
		if err != nil {
			res.DecompressErr = err
			return res, nil
		}
		res.Body = decompressed
	}

	decodeFormat := buildDecodeFormat(tc.Version, res.MediaParams, reqCfg.Escaping)
	dec := expfmt.NewDecoder(bytes.NewReader(res.Body), decodeFormat)
	for {
		mf := &dto.MetricFamily{}
		if err := dec.Decode(mf); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			res.ParseErr = err
			break
		}
		name := mf.GetName()
		if _, dup := res.Families[name]; dup {
			res.ParseErr = fmt.Errorf("duplicate metric family %q", name)
			break
		}
		res.Families[name] = mf
		res.OrderedFamilies = append(res.OrderedFamilies, mf)
	}

	return res, nil
}

// buildDecodeFormat constructs the expfmt.Format for decoding the scraped payload
// using the ABNF OpenMetrics 1.0 or 2.0 parser for the requested test version
// while honoring any requested or negotiated escaping scheme.
func buildDecodeFormat(v Version, mediaParams map[string]string, reqEscaping string) expfmt.Format {
	versionStr := string(v)
	if versionStr == "" {
		versionStr = expfmt.OpenMetricsVersion_1_0_0
	}
	if v == Version2_0_0 && mediaParams["version"] == expfmt.OpenMetricsVersion_2_0_0_rc {
		versionStr = expfmt.OpenMetricsVersion_2_0_0_rc
	}

	f, err := expfmt.NewOpenMetricsFormat(versionStr)
	if err != nil {
		f = expfmt.FmtOpenMetrics_1_0_0
	}

	escaping := reqEscaping
	if escaping == "" {
		escaping = mediaParams[model.EscapingKey]
	}
	if escaping != "" {
		if scheme, err := model.ToEscapingScheme(escaping); err == nil {
			f = f.WithEscapingScheme(scheme)
		}
	}
	return f
}

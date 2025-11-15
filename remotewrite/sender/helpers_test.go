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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/compliance/remotewrite/sender/targets"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

// CapturedRequest represents a captured HTTP request from a sender.
type CapturedRequest struct {
	Headers http.Header
	Body    []byte
	Request *writev2.Request
}

// MockReceiver is an HTTP server that captures remote write requests.
type MockReceiver struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []CapturedRequest
	response MockReceiverResponse
}

// MockReceiverResponse configures the response behavior of the mock receiver.
type MockReceiverResponse struct {
	StatusCode        int
	Headers           map[string]string
	Body              string
	SamplesWritten    int
	ExemplarsWritten  int
	HistogramsWritten int
}

// NewMockReceiver creates a new mock HTTP receiver for testing senders.
func NewMockReceiver() *MockReceiver {
	mr := &MockReceiver{
		requests: make([]CapturedRequest, 0),
		response: MockReceiverResponse{
			StatusCode: http.StatusNoContent,
		},
	}

	mr.server = httptest.NewServer(http.HandlerFunc(mr.handleRequest))
	return mr
}

// handleRequest handles incoming remote write requests.
func (mr *MockReceiver) handleRequest(w http.ResponseWriter, r *http.Request) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	headers := r.Header.Clone()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req writev2.Request
	var decoded []byte

	contentEncoding := r.Header.Get("Content-Encoding")
	if contentEncoding == "snappy" {
		decoded, err = snappy.Decode(nil, body)
		if err != nil {
			http.Error(w, "Failed to decode snappy", http.StatusBadRequest)
			return
		}
	} else {
		decoded = body
	}

	if err := req.Unmarshal(decoded); err != nil {
		http.Error(w, "Failed to unmarshal protobuf", http.StatusBadRequest)
		return
	}

	mr.requests = append(mr.requests, CapturedRequest{
		Headers: headers,
		Body:    body,
		Request: &req,
	})

	for k, v := range mr.response.Headers {
		w.Header().Set(k, v)
	}

	// Set X-Prometheus-Remote-Write-*-Written headers if response is successful.
	if mr.response.StatusCode >= 200 && mr.response.StatusCode < 300 {
		// If response counts are explicitly configured, use those. Otherwise, automatically count what we actually received.
		samplesWritten := mr.response.SamplesWritten
		exemplarsWritten := mr.response.ExemplarsWritten
		histogramsWritten := mr.response.HistogramsWritten

		// Auto-count if not explicitly set.
		if samplesWritten == 0 && exemplarsWritten == 0 && histogramsWritten == 0 {
			for _, ts := range req.Timeseries {
				samplesWritten += len(ts.Samples)
				exemplarsWritten += len(ts.Exemplars)
				histogramsWritten += len(ts.Histograms)
			}
		}

		w.Header().Set("X-Prometheus-Remote-Write-Samples-Written", fmt.Sprintf("%d", samplesWritten))
		w.Header().Set("X-Prometheus-Remote-Write-Exemplars-Written", fmt.Sprintf("%d", exemplarsWritten))
		w.Header().Set("X-Prometheus-Remote-Write-Histograms-Written", fmt.Sprintf("%d", histogramsWritten))
	}

	w.WriteHeader(mr.response.StatusCode)
	if mr.response.Body != "" {
		w.Write([]byte(mr.response.Body))
	}
}

// URL returns the URL of the mock receiver.
func (mr *MockReceiver) URL() string {
	return mr.server.URL
}

// Close shuts down the mock receiver server.
func (mr *MockReceiver) Close() {
	mr.server.Close()
}

// GetRequests returns all captured requests.
func (mr *MockReceiver) GetRequests() []CapturedRequest {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	return append([]CapturedRequest{}, mr.requests...)
}

// GetLastRequest returns the most recent captured request, or nil if none.
func (mr *MockReceiver) GetLastRequest() *CapturedRequest {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	if len(mr.requests) == 0 {
		return nil
	}
	return &mr.requests[len(mr.requests)-1]
}

// ClearRequests clears all captured requests.
func (mr *MockReceiver) ClearRequests() {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.requests = make([]CapturedRequest, 0)
}

// SetResponse configures the response behavior.
func (mr *MockReceiver) SetResponse(resp MockReceiverResponse) {
	mr.mu.Lock()
	defer mr.mu.Unlock()
	mr.response = resp
}

// WaitForRequests waits for at least n requests to be captured, with configurable timeout.
// Polls every 100ms. Returns all captured requests.
func (mr *MockReceiver) WaitForRequests(n int, timeout time.Duration) []CapturedRequest {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		requests := mr.GetRequests()
		if len(requests) >= n {
			return requests
		}
		<-ticker.C
	}

	// Return whatever we got on timeout
	return mr.GetRequests()
}

// MockScrapeTarget is an HTTP server that serves metrics in Prometheus format.
type MockScrapeTarget struct {
	server  *httptest.Server
	mu      sync.Mutex
	metrics string
}

// NewMockScrapeTarget creates a new mock scrape target.
func NewMockScrapeTarget(initialMetrics string) *MockScrapeTarget {
	mst := &MockScrapeTarget{
		metrics: initialMetrics,
	}
	mst.server = httptest.NewServer(http.HandlerFunc(mst.handleScrape))
	return mst
}

// handleScrape serves metrics in Prometheus exposition format or OpenMetrics format.
// If the metrics contain exemplars (detected by "# {" pattern), use OpenMetrics format.
func (mst *MockScrapeTarget) handleScrape(w http.ResponseWriter, r *http.Request) {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	hasExemplars := containsExemplars(mst.metrics)
	metrics := mst.metrics

	if hasExemplars {
		contentType := "application/openmetrics-text; version=1.0.0; charset=utf-8"
		if !strings.HasSuffix(metrics, "# EOF\n") {
			metrics = metrics + "# EOF\n"
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(metrics))
		return
	}

	// Normal text format for non-exemplar metrics.
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(metrics))
}

// URL returns the URL of the mock scrape target.
func (mst *MockScrapeTarget) URL() string {
	return mst.server.URL
}

// UpdateMetrics updates the metrics served by the scrape target.
func (mst *MockScrapeTarget) UpdateMetrics(metrics string) {
	mst.mu.Lock()
	defer mst.mu.Unlock()
	mst.metrics = metrics
}

// Close shuts down the mock scrape target server.
func (mst *MockScrapeTarget) Close() {
	mst.server.Close()
}

// extractLabels extracts labels from a TimeSeries using the symbol table.
func extractLabels(ts *writev2.TimeSeries, symbols []string) map[string]string {
	labels := make(map[string]string)
	refs := ts.LabelsRefs

	// Labels are stored as pairs: [key_ref, value_ref, key_ref, value_ref, ...].
	for i := 0; i < len(refs); i += 2 {
		if i+1 >= len(refs) {
			break
		}
		keyRef := refs[i]
		valueRef := refs[i+1]

		// Validate symbol indices.
		if int(keyRef) >= len(symbols) || int(valueRef) >= len(symbols) {
			continue
		}

		key := symbols[keyRef]
		value := symbols[valueRef]
		labels[key] = value
	}

	return labels
}

// extractExemplarLabels extracts labels from an Exemplar using the symbol table.
func extractExemplarLabels(ex *writev2.Exemplar, symbols []string) map[string]string {
	labels := make(map[string]string)
	refs := ex.LabelsRefs

	for i := 0; i < len(refs); i += 2 {
		if i+1 >= len(refs) {
			break
		}
		keyRef := refs[i]
		valueRef := refs[i+1]

		if int(keyRef) >= len(symbols) || int(valueRef) >= len(symbols) {
			continue
		}

		key := symbols[keyRef]
		value := symbols[valueRef]
		labels[key] = value
	}

	return labels
}

// must marks a test as having a "MUST" RFC compliance level.
// Tests marked with must() will fail on assertion failures.
func must(t *testing.T) *require.Assertions {
	t.Helper()
	t.Attr("rfcLevel", "MUST")
	return require.New(t)
}

// should marks a test as having a "SHOULD" RFC compliance level.
func should(t *testing.T, condition bool, msg string) {
	t.Helper()
	t.Attr("rfcLevel", "SHOULD")
	if !condition {
		t.Errorf("⚠️  SHOULD level requirement not met (recommended): %s", msg)
	}
}

// may marks a test as having a "MAY" RFC compliance level.
func may(t *testing.T, condition bool, msg string) {
	t.Helper()
	t.Attr("rfcLevel", "MAY")
	if !condition {
		t.Errorf("ℹ️  MAY level feature not present (optional): %s", msg)
	}
}

// recommended marks a test as having an "RECOMMENDED" compliance level.
func recommended(t *testing.T, condition bool, msg string) {
	t.Helper()
	t.Attr("rfcLevel", "RECOMMENDED")
	if !condition {
		t.Errorf("🔧 RECOMMENDED not implemented (performance enhancement): %s", msg)
	}
}

// validateSymbolTable validates that the symbol table follows RW 2.0 requirements.
func validateSymbolTable(t *testing.T, symbols []string) {
	t.Helper()

	must(t).NotEmpty(symbols, "Symbol table must not be empty")
	must(t).Equal("", symbols[0], "First symbol (index 0) must be empty string")

	// Check for duplicates (MUST requirement for deduplication).
	seen := make(map[string]bool)
	for _, sym := range symbols {
		if seen[sym] && sym != "" {
			// Duplicate non-empty strings found - this violates deduplication requirement.
			must(t).Fail(fmt.Sprintf("Duplicate symbol found in symbol table: %q", sym))
		}
		seen[sym] = true
	}
}

// validateLabelRefs validates that label references are valid.
func validateLabelRefs(t *testing.T, refs []uint32, symbols []string) {
	t.Helper()

	must(t).Equal(0, len(refs)%2, "Label refs length must be even (key-value pairs)")

	for i, ref := range refs {
		must(t).Less(int(ref), len(symbols),
			"Label ref at index %d points to invalid symbol index %d (symbol table size: %d)",
			i, ref, len(symbols))
	}
}

// isSorted checks if labels are sorted lexicographically.
func isSorted(labels map[string]string, symbols []string, refs []uint32) bool {
	var prevKey string
	for i := 0; i < len(refs); i += 2 {
		if i+1 >= len(refs) {
			break
		}
		keyRef := refs[i]
		if int(keyRef) >= len(symbols) {
			return false
		}
		key := symbols[keyRef]
		if prevKey != "" && key <= prevKey {
			return false
		}
		prevKey = key
	}
	return true
}

// containsExemplars checks if the metrics string contains exemplar annotations.
// Exemplars in OpenMetrics format are indicated by "# {" after a metric value.
func containsExemplars(metrics string) bool {
	return strings.Contains(metrics, "# {")
}

// TestCase represents a single test case for compliance testing.
type TestCase struct {
	Name        string
	Description string
	RFCLevel    string
	ScrapeData  string
	Validator   func(*testing.T, *CapturedRequest)
}

// runTestCases is a helper that eliminates the common test table runner pattern.
func runTestCases(t *testing.T, tests []TestCase) {
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			t.Attr("rfcLevel", tt.RFCLevel)
			t.Attr("description", tt.Description)

			forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
				runSenderTest(t, targetName, target, SenderTestScenario{
					ScrapeData: tt.ScrapeData,
					Validator:  tt.Validator,
				})
			})
		})
	}
}

// findTimeseriesByMetricName finds a timeseries by metric name from a captured request.
func findTimeseriesByMetricName(req *CapturedRequest, metricName string) (*writev2.TimeSeries, map[string]string) {
	for i := range req.Request.Timeseries {
		ts := &req.Request.Timeseries[i]
		labels := extractLabels(ts, req.Request.Symbols)
		if labels["__name__"] == metricName {
			return ts, labels
		}
	}
	return nil, nil
}

// requireTimeseriesByMetricName finds a timeseries by metric name and fails the test if not found.
func requireTimeseriesByMetricName(t *testing.T, req *CapturedRequest, metricName string) (*writev2.TimeSeries, map[string]string) {
	t.Helper()
	ts, labels := findTimeseriesByMetricName(req, metricName)
	must(t).NotNil(ts, "Timeseries with metric name %q must be present", metricName)
	return ts, labels
}

// findHistogramData attempts to find histogram data in both classic and native formats.
// Returns (classicFound, nativeTS) where:
//   - classicFound: true if classic histogram metrics (_count, _sum, _bucket) are found
//   - nativeTS: pointer to timeseries containing native histogram, or nil if not found
func findHistogramData(req *CapturedRequest, baseName string) (classicFound bool, nativeTS *writev2.TimeSeries) {
	for i := range req.Request.Timeseries {
		ts := &req.Request.Timeseries[i]
		labels := extractLabels(ts, req.Request.Symbols)
		metricName := labels["__name__"]

		// Check for classic histogram components.
		if metricName == baseName+"_count" || metricName == baseName+"_sum" || metricName == baseName+"_bucket" {
			classicFound = true
		}

		// Check for native histogram format.
		if metricName == baseName && len(ts.Histograms) > 0 {
			nativeTS = ts
		}
	}
	return classicFound, nativeTS
}

// extractHistogramCount extracts count from either classic or native histogram format.
// Returns (count, found) where found indicates if count was successfully extracted.
func extractHistogramCount(req *CapturedRequest, baseName string) (float64, bool) {
	// Try classic format first.
	ts, _ := findTimeseriesByMetricName(req, baseName+"_count")
	if ts != nil && len(ts.Samples) > 0 {
		return ts.Samples[0].Value, true
	}

	// Try native format.
	ts, _ = findTimeseriesByMetricName(req, baseName)
	if ts != nil && len(ts.Histograms) > 0 {
		hist := ts.Histograms[0]
		if hist.Count != nil {
			if countInt, ok := hist.Count.(*writev2.Histogram_CountInt); ok {
				return float64(countInt.CountInt), true
			} else if countFloat, ok := hist.Count.(*writev2.Histogram_CountFloat); ok {
				return countFloat.CountFloat, true
			}
		}
	}

	return 0, false
}

// extractHistogramSum extracts sum from either classic or native histogram format.
func extractHistogramSum(req *CapturedRequest, baseName string) (float64, bool) {
	// Try classic format first.
	ts, _ := findTimeseriesByMetricName(req, baseName+"_sum")
	if ts != nil && len(ts.Samples) > 0 {
		return ts.Samples[0].Value, true
	}

	// Try native format.
	ts, _ = findTimeseriesByMetricName(req, baseName)
	if ts != nil && len(ts.Histograms) > 0 {
		return ts.Histograms[0].Sum, true
	}

	return 0, false
}

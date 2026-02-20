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
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/golang/snappy"
	"github.com/prometheus/compliance/remotewrite/sender/targets"
	writev1 "github.com/prometheus/prometheus/prompb"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

// CapturedRequest represents a captured HTTP request from a sender.
// DEPRECATED: To kill, use sendertest.
type CapturedRequest struct {
	Headers http.Header
	Body    []byte
	Request *writev2.Request
}

// MockReceiver is an HTTP server that captures remote write requests.
// DEPRECATED: To kill, use sendertest.
type MockReceiver struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []CapturedRequest
	response MockReceiverResponse
}

// MockReceiverResponse configures the response behavior of the mock receiver.
// DEPRECATED: To kill, use sendertest.
type MockReceiverResponse struct {
	StatusCode        int
	Headers           map[string]string
	Body              string
	SamplesWritten    int
	ExemplarsWritten  int
	HistogramsWritten int
}

// NewMockReceiver creates a new mock HTTP receiver for testing senders.
// DEPRECATED: To kill, use sendertest.
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

// MockScrapeTarget is an HTTP server that serves metrics in Prometheus format.
// DEPRECATED: To kill, use sendertest.
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
// DEPRECATED: To kill, use sendertest.
func must(t *testing.T) *require.Assertions {
	t.Helper()
	t.Attr("rfcLevel", "MUST")
	return require.New(t)
}

// should marks a test as having a "SHOULD" RFC compliance level.
// DEPRECATED: To kill, use sendertest.
func should(t *testing.T, condition bool, msg string) {
	t.Helper()
	t.Attr("rfcLevel", "SHOULD")
	if !condition {
		t.Errorf("⚠️  SHOULD level requirement not met (recommended): %s", msg)
	}
}

// may marks a test as having a "MAY" RFC compliance level.
// DEPRECATED: To kill, use sendertest.
func may(t *testing.T, condition bool, msg string) {
	t.Helper()
	t.Attr("rfcLevel", "MAY")
	if !condition {
		t.Errorf("ℹ️  MAY level feature not present (optional): %s", msg)
	}
}

// recommended marks a test as having an "RECOMMENDED" compliance level.
// DEPRECATED: To kill, use sendertest.
func recommended(t *testing.T, condition bool, msg string) {
	t.Helper()
	t.Attr("rfcLevel", "RECOMMENDED")
	if !condition {
		t.Errorf("🔧 RECOMMENDED not implemented (performance enhancement): %s", msg)
	}
}

// isSorted checks if label names are sorted lexicographically.
func isSorted(symbols []string, refs []uint32) bool {
	var prevKey string
	for i := 0; i < len(refs); i += 2 {
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

// isSortedRW1 checks if label names are sorted lexicographically.
func isSortedRW1(labels []writev1.Label) bool {
	return sort.SliceIsSorted(labels, func(i, j int) bool {
		return strings.Compare(labels[i].Name, labels[j].Name) < 0
	})
}

func TestIsSorted(t *testing.T) {
	symbols := []string{"", "a", "c", "b", "x", "__name__"}
	require.True(t, isSorted(symbols, []uint32{5, 1, 1, 1, 3, 1, 2, 1, 4, 1}))
	require.False(t, isSorted(symbols, []uint32{5, 1, 1, 1, 2, 1, 3, 1, 4, 1}))

	require.True(t, isSortedRW1([]writev1.Label{{Name: "__name__"}, {Name: "a"}, {Name: "b"}, {Name: "c"}}))
	require.False(t, isSortedRW1([]writev1.Label{{Name: "__name__"}, {Name: "a"}, {Name: "x"}, {Name: "c"}}))
	require.False(t, isSortedRW1([]writev1.Label{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "__name__"}}))
}

// containsExemplars checks if the metrics string contains exemplar annotations.
// Exemplars in OpenMetrics format are indicated by "# {" after a metric value.
func containsExemplars(metrics string) bool {
	return strings.Contains(metrics, "# {")
}

// TestCase represents a single test case for compliance testing.
// DEPRECATED: To kill, use sendertest.
type TestCase struct {
	Name        string
	Description string
	RFCLevel    string
	ScrapeData  string
	Validator   func(*testing.T, *CapturedRequest)
}

// runTestCases is a helper that eliminates the common test table runner pattern.
// DEPRECATED: To kill, use sendertest.
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
func findTimeseriesByMetricName(req *writev2.Request, metricName string) (*writev2.TimeSeries, map[string]string) {
	for i := range req.Timeseries {
		ts := &req.Timeseries[i]
		labels := extractLabels(ts, req.Symbols)
		if labels["__name__"] == metricName {
			return ts, labels
		}
	}
	return nil, nil
}

// requireTimeseriesByMetricName finds a timeseries by metric name and fails the test if not found.
func requireTimeseriesByMetricName(t *testing.T, req *writev2.Request, metricName string) (*writev2.TimeSeries, map[string]string) {
	t.Helper()
	ts, labels := findTimeseriesByMetricName(req, metricName)
	require.NotNil(t, ts, "Timeseries with metric name %q must be present", metricName)
	return ts, labels
}

// requireTimeseriesRW1ByMetricName finds a timeseries by metric name and fails the test if not found.
func requireTimeseriesRW1ByMetricName(t *testing.T, req *writev1.WriteRequest, metricName string) *writev1.TimeSeries {
	t.Helper()

	for i := range req.Timeseries {
		for _, l := range req.Timeseries[i].Labels {
			if l.Name == "__name__" && l.Value == metricName {
				return &req.Timeseries[i]
			}
		}
	}
	t.Fatalf("Timeseries with metric name %q must be present", metricName)
	return nil
}

// findHistogramData attempts to find histogram data in both classic and native formats.
// Returns (classicFound, nativeTS) where:
//   - classicFound: true if classic histogram metrics (_count, _sum, _bucket) are found
//   - nativeTS: pointer to timeseries containing native histogram, or nil if not found
func findHistogramData(req *writev2.Request, baseName string) (classicFound bool, nativeTS *writev2.TimeSeries) {
	for i := range req.Timeseries {
		ts := &req.Timeseries[i]
		labels := extractLabels(ts, req.Symbols)
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
func extractHistogramCount(req *writev2.Request, baseName string) (float64, bool) {
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
func extractHistogramSum(req *writev2.Request, baseName string) (float64, bool) {
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

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
	"log"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/compliance/remotewrite/sender/targets"
)

var (
	// testTimeout is the default timeout for tests.
	testTimeout = 2 * time.Minute

	// registeredTargets holds targets that can automatically download binaries.
	registeredTargets = map[string]targets.Target{
		"prometheus": targets.RunPrometheus,
		//"grafana_agent": targets.RunGrafanaAgent,
		//"otelcollector": targets.RunOtelCollector,
		//"vmagent":       targets.RunVMAgent,
		//"telegraf":      targets.RunTelegraf,
		//"vector":        targets.RunVector,
	}
)

// TestMain sets up the test environment.
func TestMain(m *testing.M) {
	log.Printf("Using automatic target downloading and configuration")

	// Set test timeout from environment if specified.
	if timeoutStr := os.Getenv("PROMETHEUS_RW2_COMPLIANCE_TEST_TIMEOUT"); timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			testTimeout = d
		}
	}

	os.Exit(m.Run())
}

// SenderTestScenario defines a test scenario to run against senders.
type SenderTestScenario struct {
	// ScrapeData is the metrics data to serve from the mock scrape target.
	ScrapeData string

	// ReceiverResponse configures how the mock receiver should respond.
	ReceiverResponse MockReceiverResponse

	// Validator is called with captured requests to perform test assertions.
	Validator func(t *testing.T, req *CapturedRequest)

	// WaitTime is how long to wait for requests after sender starts.
	WaitTime time.Duration

	// ExpectedRequestCount is the number of requests expected (0 = at least 1).
	ExpectedRequestCount int
}

// runSenderTest runs a test scenario using an automatic target.
func runSenderTest(t *testing.T, targetName string, target targets.Target, scenario SenderTestScenario) {
	t.Helper()

	receiver := NewMockReceiver()
	defer receiver.Close()

	// Configure receiver response.
	if scenario.ReceiverResponse.StatusCode == 0 {
		scenario.ReceiverResponse.StatusCode = http.StatusNoContent
	}
	receiver.SetResponse(scenario.ReceiverResponse)

	scrapeTarget := NewMockScrapeTarget(scenario.ScrapeData)
	defer scrapeTarget.Close()

	t.Logf("Running %s with scrape target %s and receiver %s", targetName, scrapeTarget.URL(), receiver.URL())

	// Run the target with appropriate timeout.
	// Auto targets need: download time (first run) + startup + scrape interval + send time
	waitTime := scenario.WaitTime
	if waitTime == 0 {
		waitTime = 6 * time.Second // Prometheus needs ~3-4s startup + 1s scrape interval + buffer
	}

	// Run target in a goroutine.
	done := make(chan error, 1)
	go func() {
		done <- target(targets.TargetOptions{
			ScrapeTarget:    scrapeTarget.URL(),
			ReceiveEndpoint: receiver.URL(),
			Timeout:         waitTime,
		})
	}()

	expectedCount := scenario.ExpectedRequestCount
	if expectedCount == 0 {
		expectedCount = 1 // Default: expect at least 1 request
	}

	// Start polling for requests in background
	requestsCh := make(chan []CapturedRequest, 1)
	go func() {
		requests := receiver.WaitForRequests(expectedCount, waitTime)
		requestsCh <- requests
	}()

	// Wait for either requests to arrive or timeout
	var requests []CapturedRequest
	select {
	case requests = <-requestsCh:
		// Got requests! Don't wait for sender to finish, just drain the channel
		select {
		case err := <-done:
			t.Logf("Target finished: %v", err)
		default:
			t.Logf("Target still running (expected)")
		}
	case <-time.After(waitTime + 2*time.Second):
		// Timeout - get whatever requests we have
		requests = receiver.GetRequests()
		t.Logf("Timeout waiting for requests, got %d", len(requests))
		// Drain the done channel
		select {
		case <-done:
		default:
		}
	}

	if len(requests) < expectedCount {
		t.Fatalf("Expected at least %d request(s), got %d", expectedCount, len(requests))
	}

	// Validate the most recent request.
	lastReq := &requests[len(requests)-1]
	if scenario.Validator != nil {
		scenario.Validator(t, lastReq)
	}
}

// runAutoTargetWithCustomReceiver runs an auto-target with a custom receiver (for special test cases).
// Use this when you need custom receiver behavior (e.g., TimestampTrackingReceiver, FallbackTrackingReceiver).
func runAutoTargetWithCustomReceiver(t *testing.T, targetName string, target targets.Target, receiverURL string, scrapeTarget *MockScrapeTarget, waitTime time.Duration) {
	t.Helper()

	if waitTime == 0 {
		waitTime = 15 * time.Second
	}

	t.Logf("Running %s with custom receiver at %s", targetName, receiverURL)

	// Run target in a goroutine.
	done := make(chan error, 1)
	go func() {
		done <- target(targets.TargetOptions{
			ScrapeTarget:    scrapeTarget.URL(),
			ReceiveEndpoint: receiverURL,
			Timeout:         waitTime,
		})
	}()

	// Wait for the target to finish or timeout.
	select {
	case err := <-done:
		if err != nil {
			t.Logf("Target finished with error (may be expected): %v", err)
		}
	case <-time.After(waitTime + 2*time.Second):
		t.Logf("Target timed out (expected for auto targets)")
	}
}

// forEachSender runs the provided test function for each configured sender.
func forEachSender(t *testing.T, f func(*testing.T, string, targets.Target)) {
	// Filter targets if environment variable is set.
	senderNames := os.Getenv("PROMETHEUS_RW2_COMPLIANCE_SENDERS")
	var targetsToTest map[string]targets.Target
	if senderNames != "" {
		targetsToTest = make(map[string]targets.Target)
		nameList := strings.Split(senderNames, ",")
		for _, name := range nameList {
			name = strings.TrimSpace(name)
			if target, ok := registeredTargets[name]; ok {
				targetsToTest[name] = target
			}
		}
		if len(targetsToTest) == 0 {
			t.Skipf("No auto targets found matching %q", senderNames)
			return
		}
	} else {
		targetsToTest = registeredTargets
	}

	// Run test for each target.
	for name, target := range targetsToTest {
		t.Run(name, func(t *testing.T) {
			t.Attr("sender", name)
			f(t, name, target)
		})
	}
}

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
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/compliance/remotewrite/sender/targets"
	"github.com/prometheus/prometheus/config"
)

var (
	// registeredTargets holds pre-defined targets to choose from.
	//
	// Custom targets could be considered for adding here, however the process target likely offers enough flexibility.
	registeredTargets = map[string]targets.Target{
		"prometheus": targets.RunPrometheus, // Default if no PROMETHEUS_RW2_COMPLIANCE_RECEIVERS is specified.
		"process":    targets.RunProcess,
	}
	// targetsToTest is a global variable controlling senders to test.
	// It is adjusted in TestMain via PROMETHEUS_RW2_COMPLIANCE_RECEIVERS variable.
	targetsToTest = map[string]targets.Target{
		"prometheus": registeredTargets["prometheus"],
	}
)

// TestMain sets up the test environment by filtering registeredTargets (senders to tests) using
// PROMETHEUS_RW2_COMPLIANCE_RECEIVERS envvar.
func TestMain(m *testing.M) {
	senderNames := os.Getenv("PROMETHEUS_RW2_COMPLIANCE_SENDERS")
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
			fmt.Println("FAIL: No targets found matching PROMETHEUS_RW2_COMPLIANCE_SENDERS=", senderNames)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

// SenderTestScenario defines a test scenario to run against senders.
type SenderTestScenario struct {
	ScrapeData           string
	ReceiverResponse     MockReceiverResponse
	Validator            func(t *testing.T, req *CapturedRequest)
	WaitTime             time.Duration
	ExpectedRequestCount int
}

// runSenderTest runs a test scenario for each configured target.
func runSenderTest(t *testing.T, scenario SenderTestScenario) {
	t.Helper()

	for name, target := range targetsToTest {
		t.Run(fmt.Sprintf("target=%v", name), func(t *testing.T) {
			t.Attr("rw", name)

			receiver := NewMockReceiver()
			defer receiver.Close()

			scrapeTarget := NewMockScrapeTarget(scenario.ScrapeData)
			defer scrapeTarget.Close()

			if scenario.ReceiverResponse.StatusCode == 0 {
				scenario.ReceiverResponse.StatusCode = http.StatusNoContent
			}
			receiver.SetResponse(scenario.ReceiverResponse)

			err := target(t.Context(), targets.TargetOptions{
				ScrapeTargetURL:    scrapeTarget.URL(),
				ReceiveEndpointURL: receiver.URL(),
				RemoteWriteMessage: "io.prometheus.write.v1","
			})

			// Check for expected error (some might be expected).
			if err != nil {
				t.Fatalf("Target failed: %v", err)
			}

			requests := receiver.GetRequests()
			if len(requests) < scenario.ExpectedRequestCount {
				t.Fatalf("Expected at least %d request(s), got %d", scenario.ExpectedRequestCount, len(requests))
			}

			if scenario.Validator != nil && len(requests) > 0 {
				lastReq := &requests[len(requests)-1]
				scenario.Validator(t, lastReq)
			}
		})
	}

}

// runAutoTargetWithCustomReceiver runs an auto-target with a custom receiver (for special test cases).
// Use this when you need custom receiver behavior (e.g., TimestampTrackingReceiver, FallbackTrackingReceiver).
func runAutoTargetWithCustomReceiver(t *testing.T, targetName string, target targets.Target, receiverURL string, scrapeTarget *MockScrapeTarget, waitTime time.Duration) error {
	t.Helper()

	if waitTime == 0 {
		waitTime = 15 * time.Second
	}

	t.Logf("Running %s with custom receiver at %s", targetName, receiverURL)

	err := target(targets.TargetOptions{
		ScrapeTarget:    scrapeTarget.URL(),
		ReceiveEndpoint: receiverURL,
		Timeout:         waitTime,
	})

	if err != nil {
		t.Logf("Target finished with error (may be expected): %v", err)
	} else {
		t.Logf("Target completed successfully")
	}

	return err
}

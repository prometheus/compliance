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

package sender

import (
	"testing"
	"time"
)

// SenderTestScenario defines a test scenario to run against senders.
// DEPRECATED: To kill, use sendertest.
type SenderTestScenario struct {
	ScrapeData           string
	ReceiverResponse     MockReceiverResponse
	Validator            func(t *testing.T, req *CapturedRequest)
	WaitTime             time.Duration
	ExpectedRequestCount int
}

// runSenderTest runs a test scenario using an automatic target.
// DEPRECATED: To kill, use sendertest.
func runSenderTest(t *testing.T, targetName string, target Sender, scenario SenderTestScenario) {
	t.Helper()
	t.Fatal("TODO: Remove")
}

// runAutoTargetWithCustomReceiver runs an auto-target with a custom receiver (for special test cases).
// Use this when you need custom receiver behavior (e.g., TimestampTrackingReceiver, FallbackTrackingReceiver).
// DEPRECATED: To kill, use sendertest.
func runAutoTargetWithCustomReceiver(t *testing.T, targetName string, target Sender, receiverURL string, scrapeTarget *MockScrapeTarget, waitTime time.Duration) error {
	t.Helper()

	t.Fatal("TODO: Remove")
	return nil
}

// forEachSender runs the provided test function for each configured sender.
func forEachSender(t *testing.T, f func(*testing.T, string, Sender)) {
	t.Fatal("TODO: Remove")
}

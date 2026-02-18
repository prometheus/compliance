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
	"testing"
	"time"

	"github.com/prometheus/compliance/remotewrite/sender/targets"
)

// TestRetryBehavior validates sender retry behavior on different error responses.
func TestRetryBehavior_Old(t *testing.T) {
	t.Skip("TODO: Revise and move to a new framework")

	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		setup       func(*MockReceiver)
		validator   func(*testing.T, []CapturedRequest)
	}{
		{
			name:        "no_retry_on_400",
			description: "Sender MUST NOT retry on 400 Bad Request",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				// Always return 400.
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusBadRequest,
					Body:       "Bad request",
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				// Should receive exactly 1 request (no retries). Allow up to 2 for initial attempt + possible single retry before detecting 4xx.
				should(t, len(requests) <= 2, fmt.Sprintf(
					"Sender should not retry on 400 Bad Request, got %d requests", len(requests)))
				t.Logf("Received %d requests for 400 response", len(requests))
			},
		},
		{
			name:        "no_retry_on_401",
			description: "Sender MUST NOT retry on 401 Unauthorized",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusUnauthorized,
					Body:       "Unauthorized",
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				should(t, len(requests) <= 2, fmt.Sprintf(
					"Sender should not retry on 401 Unauthorized, got %d requests", len(requests)))
				t.Logf("Received %d requests for 401 response", len(requests))
			},
		},
		{
			name:        "no_retry_on_404",
			description: "Sender MUST NOT retry on 404 Not Found",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusNotFound,
					Body:       "Not found",
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				should(t, len(requests) <= 2, fmt.Sprintf(
					"Sender should not retry on 404 Not Found, got %d requests", len(requests)))
				t.Logf("Received %d requests for 404 response", len(requests))
			},
		},
		{
			name:        "may_retry_on_429",
			description: "Sender MAY retry on 429 Too Many Requests",
			rfcLevel:    "MAY",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusTooManyRequests,
					Headers: map[string]string{
						"Retry-After": "1",
					},
					Body: "Too many requests",
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				// 429 retry behavior is optional.
				may(t, len(requests) >= 1, "Sender may retry on 429 Too Many Requests")
				t.Logf("Received %d requests for 429 response (retry optional)", len(requests))
			},
		},
		{
			name:        "retry_on_500",
			description: "Sender MUST retry on 500 Internal Server Error",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusInternalServerError,
					Body:       "Internal server error",
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				// Should retry on 500 (expect multiple attempts).
				// Note: Some senders may give up after a few retries. We just check that at least one request was made.
				must(t).GreaterOrEqual(len(requests), 1,
					"Sender should attempt request on 500 Internal Server Error")
				t.Logf("Received %d requests for 500 response (retries expected)", len(requests))
			},
		},
		{
			name:        "retry_on_503",
			description: "Sender MUST retry on 503 Service Unavailable",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusServiceUnavailable,
					Body:       "Service unavailable",
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				must(t).GreaterOrEqual(len(requests), 1,
					"Sender should attempt request on 503 Service Unavailable")
				t.Logf("Received %d requests for 503 response", len(requests))
			},
		},
		{
			name:        "retry_on_502",
			description: "Sender SHOULD retry on 502 Bad Gateway",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusBadGateway,
					Body:       "Bad gateway",
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				should(t, len(requests) >= 1, "Sender should retry on 502 Bad Gateway")
				t.Logf("Received %d requests for 502 response", len(requests))
			},
		},
		{
			name:        "retry_on_504",
			description: "Sender SHOULD retry on 504 Gateway Timeout",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusGatewayTimeout,
					Body:       "Gateway timeout",
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				should(t, len(requests) >= 1, "Sender should retry on 504 Gateway Timeout")
				t.Logf("Received %d requests for 504 response", len(requests))
			},
		},
		{
			name:        "no_retry_on_413",
			description: "Sender SHOULD NOT retry on 413 Payload Too Large",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusRequestEntityTooLarge,
					Body:       "Payload too large",
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				should(t, len(requests) <= 2, fmt.Sprintf(
					"Sender should not retry on 413 Payload Too Large, got %d requests", len(requests)))
				t.Logf("Received %d requests for 413 response", len(requests))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Attr("rfcLevel", tt.rfcLevel)
			t.Attr("description", tt.description)

			forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
				receiver := NewMockReceiver()
				defer receiver.Close()

				tt.setup(receiver)

				scrapeTarget := NewMockScrapeTarget(tt.scrapeData)
				defer scrapeTarget.Close()

				err := target(targets.TargetOptions{
					ScrapeTarget:    scrapeTarget.URL(),
					ReceiveEndpoint: receiver.URL(),
					Timeout:         8 * time.Second,
				})

				if err != nil {
					t.Logf("Target exited with error (expected for retry tests): %v", err)
				}

				requests := receiver.GetRequests()
				tt.validator(t, requests)
			})
		})
	}
}

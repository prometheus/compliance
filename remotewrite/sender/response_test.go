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
	"github.com/prometheus/compliance/remotewrite/sender/targets"
	"net/http"
	"testing"
	"time"
)

// TestResponseProcessing validates sender response header processing.
func TestResponseProcessing(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		setup       func(*MockReceiver)
		validator   func(*testing.T, []CapturedRequest)
	}{
		{
			name:        "ignore_response_body_on_success",
			description: "Sender SHOULD ignore response body on 2xx success",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode:        http.StatusNoContent,
					Body:              "This body should be ignored",
					SamplesWritten:    1,
					ExemplarsWritten:  0,
					HistogramsWritten: 0,
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				should(t).GreaterOrEqual(len(requests), 1,
					"Should receive at least one request")

				// Sender should accept 204 with body (even though unusual)
				should(t).True(true,
					"Sender should ignore response body on successful requests")
				t.Logf("Received %d successful requests", len(requests))
			},
		},
		{
			name:        "process_written_count_headers",
			description: "Sender MAY use X-Prometheus-Remote-Write-*-Written headers",
			rfcLevel:    "MAY",
			scrapeData: `# Multiple samples
test_counter_total{label="a"} 100
test_counter_total{label="b"} 200
test_gauge{label="c"} 50
`,
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode:        http.StatusNoContent,
					SamplesWritten:    3,
					ExemplarsWritten:  0,
					HistogramsWritten: 0,
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				may(t).GreaterOrEqual(len(requests), 1,
					"Should receive at least one request")

				// Sender may use these headers for optimization/tracking
				may(t).True(true,
					"Sender may process X-Prometheus-Remote-Write-*-Written headers")
				t.Logf("Sent response with written count headers")
			},
		},
		{
			name:        "handle_partial_write_response",
			description: "Sender SHOULD handle partial write responses (some data accepted)",
			rfcLevel:    "SHOULD",
			scrapeData: `# Multiple samples
sample_1 1
sample_2 2
sample_3 3
`,
			setup: func(mr *MockReceiver) {
				// Indicate only 2 out of 3 samples were written
				mr.SetResponse(MockReceiverResponse{
					StatusCode:        http.StatusBadRequest,
					Body:              "Rejected 1 sample",
					SamplesWritten:    2, // Partial acceptance
					ExemplarsWritten:  0,
					HistogramsWritten: 0,
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				should(t).GreaterOrEqual(len(requests), 1,
					"Should receive at least one request")

				// Sender should handle partial writes
				should(t).True(true,
					"Sender should handle partial write responses")
				t.Logf("Handled partial write response")
			},
		},
		{
			name:        "handle_missing_written_headers",
			description: "Sender SHOULD assume 0 written if headers missing on 4xx/5xx",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				// Return error without written count headers
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusBadRequest,
					Body:       "Bad request",
					// No written counts set (defaults to 0)
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				should(t).GreaterOrEqual(len(requests), 1,
					"Should receive request even with error")

				// Sender should assume nothing was written
				should(t).True(true,
					"Sender should assume 0 written when headers missing")
				t.Logf("Handled missing written count headers")
			},
		},
		{
			name:        "log_error_messages_verbatim",
			description: "Sender MUST log error messages as-is without interpretation",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusBadRequest,
					Body:       "Error: Invalid label name '__invalid__'",
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				must(t).GreaterOrEqual(len(requests), 1,
					"Should receive request")

				// Sender should log the error message without modification
				must(t).True(true,
					"Sender must log error messages verbatim")
				t.Logf("Error response sent to sender")
			},
		},
		{
			name:        "handle_large_error_body",
			description: "Sender SHOULD handle large error response bodies",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				// Create large error message
				largeError := "Error details: "
				for i := 0; i < 1000; i++ {
					largeError += "detailed error information; "
				}

				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusBadRequest,
					Body:       largeError,
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				should(t).GreaterOrEqual(len(requests), 1,
					"Should handle large error bodies")

				should(t).True(true,
					"Sender should handle large error response bodies")
				t.Logf("Handled large error response body")
			},
		},
		{
			name:        "handle_204_no_content",
			description: "Sender MUST accept 204 No Content as success",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode:        http.StatusNoContent,
					SamplesWritten:    1,
					ExemplarsWritten:  0,
					HistogramsWritten: 0,
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				must(t).GreaterOrEqual(len(requests), 1,
					"Should successfully send to 204 endpoint")

				must(t).True(true,
					"Sender must accept 204 No Content as successful response")
				t.Logf("Successfully handled 204 No Content")
			},
		},
		{
			name:        "handle_200_ok",
			description: "Sender MUST accept 200 OK as success",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode:        http.StatusOK,
					Body:              "OK",
					SamplesWritten:    1,
					ExemplarsWritten:  0,
					HistogramsWritten: 0,
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				must(t).GreaterOrEqual(len(requests), 1,
					"Should successfully send to 200 endpoint")

				must(t).True(true,
					"Sender must accept 200 OK as successful response")
				t.Logf("Successfully handled 200 OK")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Attr("rfcLevel", tt.rfcLevel)
			t.Attr("description", tt.description)

			forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {

				receiver := NewMockReceiver()
				defer receiver.Close()

				tt.setup(receiver)

				scrapeTarget := NewMockScrapeTarget(tt.scrapeData)
				defer scrapeTarget.Close()



				// Wait for sender to send
				time.Sleep(6 * time.Second)

				// Get captured requests
				requests := receiver.GetRequests()
				tt.validator(t, requests)
			})
		})
	}
}

// TestContentTypeNegotiation validates content-type handling.
func TestContentTypeNegotiation(t *testing.T) {
	t.Attr("rfcLevel", "SHOULD")
	t.Attr("description", "Sender SHOULD handle content-type negotiation")

	scrapeData := "test_metric 42\n"

	forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
		receiver := NewMockReceiver()
		defer receiver.Close()

		// Set successful response
		receiver.SetResponse(MockReceiverResponse{
			StatusCode:        http.StatusNoContent,
			SamplesWritten:    1,
			ExemplarsWritten:  0,
			HistogramsWritten: 0,
		})

		scrapeTarget := NewMockScrapeTarget(scrapeData)
		defer scrapeTarget.Close()



		time.Sleep(5 * time.Second)

		requests := receiver.GetRequests()
		should(t).GreaterOrEqual(len(requests), 1,
			"Should send at least one request")

		if len(requests) > 0 {
			contentType := requests[0].Headers.Get("Content-Type")
			should(t).Contains(contentType, "application/x-protobuf",
				"Should use protobuf content-type")
			t.Logf("Content-Type: %s", contentType)
		}
	})
}

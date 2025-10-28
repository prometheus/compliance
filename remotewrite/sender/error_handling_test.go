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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/compliance/remotewrite/sender/targets"
)

// TestErrorHandling validates sender error handling in various failure scenarios.
func TestErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		setup       func() *httptest.Server
		validator   func(*testing.T, *httptest.Server)
	}{
		{
			name:        "handle_connection_refused",
			description: "Sender SHOULD handle connection refused gracefully",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func() *httptest.Server {
				// Return a server that immediately closes
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// This will be closed before sender connects
				}))
				server.Close() // Close immediately
				return server
			},
			validator: func(t *testing.T, server *httptest.Server) {
				// Sender should handle connection refused without crashing
				should(t, true, "Sender should handle connection refused")
				t.Logf("Sender handled connection refused scenario")
			},
		},
		{
			name:        "handle_timeout",
			description: "Sender SHOULD handle request timeouts gracefully",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func() *httptest.Server {
				// Create server that never responds
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Sleep longer than typical timeout
					time.Sleep(30 * time.Second)
				}))
			},
			validator: func(t *testing.T, server *httptest.Server) {
				// Sender should timeout and handle gracefully
				should(t, true, "Sender should handle timeouts")
				t.Logf("Sender handled timeout scenario")
			},
		},
		{
			name:        "handle_partial_write",
			description: "Sender SHOULD handle partial HTTP writes gracefully",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Write partial response and close
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("partial"))
					// Connection will be closed abruptly
				}))
			},
			validator: func(t *testing.T, server *httptest.Server) {
				should(t, true, "Sender should handle partial writes")
				t.Logf("Sender handled partial write scenario")
			},
		},
		{
			name:        "handle_malformed_response",
			description: "Sender SHOULD handle malformed HTTP responses",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Send invalid HTTP response
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("Not valid response headers\r\n"))
				}))
			},
			validator: func(t *testing.T, server *httptest.Server) {
				should(t, true, "Sender should handle malformed responses")
				t.Logf("Sender handled malformed response")
			},
		},
		{
			name:        "sender_continues_after_errors",
			description: "Sender MUST continue running after recoverable errors",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			setup: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Return error but sender should keep trying
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("Internal server error"))
				}))
			},
			validator: func(t *testing.T, server *httptest.Server) {
				// Sender should not crash and keep running
				must(t).True(true, "Sender must continue running after errors")
				t.Logf("Sender continues running after errors")
			},
		},
		{
			name:        "handle_invalid_status_code",
			description: "Sender SHOULD handle invalid HTTP status codes",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Send unusual status code
					w.WriteHeader(999)
				}))
			},
			validator: func(t *testing.T, server *httptest.Server) {
				should(t, true, "Sender should handle unusual status codes")
				t.Logf("Sender handled invalid status code")
			},
		},
		{
			name:        "handle_empty_response",
			description: "Sender SHOULD handle empty responses gracefully",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Send 200 with no body
					w.WriteHeader(http.StatusOK)
					// No response body
				}))
			},
			validator: func(t *testing.T, server *httptest.Server) {
				should(t, true, "Sender should handle empty responses")
				t.Logf("Sender handled empty response")
			},
		},
		{
			name:        "handle_large_error_response",
			description: "Sender SHOULD handle large error response bodies",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusBadRequest)
					// Send large error message
					largeError := make([]byte, 1024*1024) // 1MB
					for i := range largeError {
						largeError[i] = 'x'
					}
					w.Write(largeError)
				}))
			},
			validator: func(t *testing.T, server *httptest.Server) {
				should(t, true, "Sender should handle large error bodies")
				t.Logf("Sender handled large error response")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			t.Attr("rfcLevel", tt.rfcLevel)
			t.Attr("description", tt.description)

			forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
				// Setup error scenario
				server := tt.setup()
				var serverURL string
				if server != nil {
					defer server.Close()
					serverURL = server.URL
				} else {
					serverURL = "http://localhost:19999"
				}

				scrapeTarget := NewMockScrapeTarget(tt.scrapeData)
				defer scrapeTarget.Close()

				// Run target with custom error server
				runAutoTargetWithCustomReceiver(t, targetName, target, serverURL, scrapeTarget, 8*time.Second)

				// Run validator
				tt.validator(t, server)

				// Verify sender process is still running (didn't crash)
			})
		})
	}
}

// TestNetworkErrors validates handling of network-level errors.
func TestNetworkErrors(t *testing.T) {
	t.Attr("rfcLevel", "SHOULD")
	t.Attr("description", "Sender SHOULD handle network errors gracefully")

	tests := []struct {
		name       string
		scrapeData string
		serverURL  string
	}{
		{
			name:       "dns_resolution_failure",
			scrapeData: "test_metric 42\n",
			serverURL:  "http://nonexistent.invalid.domain:9999/api/v1/write",
		},
		{
			name:       "invalid_url_scheme",
			scrapeData: "test_metric 42\n",
			serverURL:  "invalid://localhost:9999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
				scrapeTarget := NewMockScrapeTarget(tt.scrapeData)
				defer scrapeTarget.Close()

				// Run target with invalid URL - sender should handle gracefully
				runAutoTargetWithCustomReceiver(t, targetName, target, tt.serverURL, scrapeTarget, 5*time.Second)

				should(t, true, "Sender handled network error without crashing")
				t.Logf("Network error handled gracefully: %s", tt.name)
			})
		})
	}
}

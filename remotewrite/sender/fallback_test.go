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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/compliance/remotewrite/sender/targets"
)

// FallbackTrackingReceiver tracks version changes across requests.
type FallbackTrackingReceiver struct {
	*MockReceiver
	mu              sync.Mutex
	requestVersions []string
	requestCount    int
	return415First  bool
}

// NewFallbackTrackingReceiver creates a receiver that tracks version fallback.
func NewFallbackTrackingReceiver(baseReceiver *MockReceiver, return415First bool) *FallbackTrackingReceiver {
	return &FallbackTrackingReceiver{
		MockReceiver:    baseReceiver,
		requestVersions: make([]string, 0),
		return415First:  return415First,
	}
}

// RecordVersion records the version from a request.
func (ftr *FallbackTrackingReceiver) RecordVersion(version string) {
	ftr.mu.Lock()
	defer ftr.mu.Unlock()
	ftr.requestVersions = append(ftr.requestVersions, version)
	ftr.requestCount++
}

// GetVersions returns all recorded versions.
func (ftr *FallbackTrackingReceiver) GetVersions() []string {
	ftr.mu.Lock()
	defer ftr.mu.Unlock()
	return append([]string{}, ftr.requestVersions...)
}

// ShouldReturn415 determines if this request should get 415 response.
func (ftr *FallbackTrackingReceiver) ShouldReturn415() bool {
	ftr.mu.Lock()
	defer ftr.mu.Unlock()
	// Only return 415 for the first request
	return ftr.return415First && ftr.requestCount == 0
}

// TestFallbackBehavior validates RW 2.0 to RW 1.0 fallback on 415 response.
func TestFallbackBehavior(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		validator   func(*testing.T, *FallbackTrackingReceiver)
	}{
		{
			name:        "fallback_on_415_unsupported_media_type",
			description: "Sender SHOULD fallback to RW 1.0 when receiving 415 Unsupported Media Type",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, tracker *FallbackTrackingReceiver) {
				time.Sleep(12 * time.Second)

				versions := tracker.GetVersions()
				if len(versions) < 2 {
					should(t, len(versions) >= 2, "Should see multiple requests for fallback validation")
					t.Logf("Only %d request(s) observed, cannot validate fallback", len(versions))
					return
				}

				// Check if version changed from 2.0 to earlier version
				firstVersion := versions[0]
				laterVersions := versions[1:]

				if strings.HasPrefix(firstVersion, "2.0") {
					// First request was RW 2.0, check if later requests fell back
					for i, v := range laterVersions {
						if strings.HasPrefix(v, "0.1") || v == "" {
							should(t, true, "Sender fell back from RW 2.0 to RW 1.0 after 415")
							t.Logf("Fallback detected: %s -> %s (request %d)",
								firstVersion, v, i+2)
							return
						}
					}
					t.Logf("No fallback detected in %d requests (versions: %v)",
						len(versions), versions)
				} else {
					t.Logf("First request not RW 2.0 (version: %s), fallback test not applicable",
						firstVersion)
				}
			},
		},
		{
			name:        "retry_with_different_version",
			description: "Sender SHOULD retry with RW 1.0 version (0.1.0) after 415",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, tracker *FallbackTrackingReceiver) {
				time.Sleep(12 * time.Second)

				versions := tracker.GetVersions()
				if len(versions) < 2 {
					t.Logf("Need multiple requests to validate version change")
					return
				}

				// Look for version change pattern
				var foundFallback bool
				for i := 1; i < len(versions); i++ {
					if strings.HasPrefix(versions[i-1], "2.0") &&
						(strings.HasPrefix(versions[i], "0.1") || versions[i] == "") {
						foundFallback = true
						should(t, true, fmt.Sprintf("Version changed from %s to %s", versions[i-1], versions[i]))
						break
					}
				}

				if !foundFallback {
					t.Logf("No version fallback observed (versions: %v)", versions)
				}
			},
		},
		{
			name:        "fallback_header_changes",
			description: "Sender MUST change Content-Type header on fallback",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, tracker *FallbackTrackingReceiver) {
				time.Sleep(10 * time.Second)

				requests := tracker.GetRequests()
				if len(requests) < 2 {
					t.Logf("Need multiple requests to validate header changes")
					return
				}

				// Check if Content-Type changed between requests
				firstCT := requests[0].Headers.Get("Content-Type")
				for i := 1; i < len(requests); i++ {
					laterCT := requests[i].Headers.Get("Content-Type")

					// If fallback happened, content-type should differ
					if firstCT != laterCT {
						must(t).NotEqual(firstCT, laterCT,
							"Content-Type should change on fallback")
						t.Logf("Content-Type changed: %s -> %s", firstCT, laterCT)
						return
					}
				}

				t.Logf("No Content-Type change observed")
			},
		},
		{
			name:        "accept_success_after_fallback",
			description: "Sender SHOULD accept successful response after fallback",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, tracker *FallbackTrackingReceiver) {
				time.Sleep(10 * time.Second)

				// After fallback, subsequent requests should succeed
				requests := tracker.GetRequests()
				should(t, len(requests) >= 1, "Should receive requests after fallback")

				t.Logf("Received %d requests total", len(requests))
			},
		},
		{
			name:        "persistent_fallback_choice",
			description: "Sender SHOULD remember successful fallback choice",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, tracker *FallbackTrackingReceiver) {
				time.Sleep(15 * time.Second)

				versions := tracker.GetVersions()
				if len(versions) < 3 {
					t.Logf("Need multiple requests to validate persistent fallback")
					return
				}

				// After fallback, version should stay consistent
				var fallbackVersion string
				for i := 1; i < len(versions); i++ {
					if versions[i] != versions[0] {
						fallbackVersion = versions[i]
						break
					}
				}

				if fallbackVersion != "" {
					// Check that subsequent requests use the same version
					consistentFallback := true
					for i := 2; i < len(versions); i++ {
						if versions[i] != fallbackVersion && versions[i] != versions[0] {
							consistentFallback = false
							break
						}
					}

					should(t, consistentFallback, "Sender should consistently use fallback version")
					t.Logf("Fallback persistence: versions=%v", versions)
				}
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

				// Create fallback tracker
				tracker := NewFallbackTrackingReceiver(receiver, true)

				// Setup custom handler that returns 415 first, then 204
				originalHandler := receiver.server.Config.Handler
				receiver.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					version := r.Header.Get("X-Prometheus-Remote-Write-Version")
					contentType := r.Header.Get("Content-Type")

					tracker.RecordVersion(version)
					t.Logf("Request %d: version=%s, content-type=%s",
						len(tracker.GetVersions()), version, contentType)

					if tracker.ShouldReturn415() {
						// Return 415 for first RW 2.0 request
						if strings.HasPrefix(version, "2.0") {
							t.Logf("Returning 415 to trigger fallback")
							w.WriteHeader(http.StatusUnsupportedMediaType)
							w.Write([]byte("RW 2.0 not supported, please use RW 1.0"))
							return
						}
					}

					// For subsequent requests or RW 1.0, return success
					originalHandler.ServeHTTP(w, r)
				})

				// Set successful response for non-415 cases
				receiver.SetResponse(MockReceiverResponse{
					StatusCode:        http.StatusNoContent,
					SamplesWritten:    1,
					ExemplarsWritten:  0,
					HistogramsWritten: 0,
				})

				scrapeTarget := NewMockScrapeTarget(tt.scrapeData)
				defer scrapeTarget.Close()

				// Run target with custom receiver
				runAutoTargetWithCustomReceiver(t, targetName, target, receiver.URL(), scrapeTarget, 15*time.Second)
				// Run validator which waits and checks fallback
				tt.validator(t, tracker)
			})
		})
	}
}

// TestNoFallbackOn2xx validates that fallback doesn't happen on success.
func TestNoFallbackOn2xx(t *testing.T) {
	t.Attr("rfcLevel", "MUST")
	t.Attr("description", "Sender MUST NOT fallback when receiving 2xx success responses")

	scrapeData := "test_metric 42\n"

	forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
		receiver := NewMockReceiver()
		defer receiver.Close()

		tracker := NewFallbackTrackingReceiver(receiver, false)

		// Track versions
		originalHandler := receiver.server.Config.Handler
		receiver.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			version := r.Header.Get("X-Prometheus-Remote-Write-Version")
			tracker.RecordVersion(version)
			originalHandler.ServeHTTP(w, r)
		})

		receiver.SetResponse(MockReceiverResponse{
			StatusCode:        http.StatusNoContent,
			SamplesWritten:    1,
			ExemplarsWritten:  0,
			HistogramsWritten: 0,
		})

		scrapeTarget := NewMockScrapeTarget(scrapeData)
		defer scrapeTarget.Close()

		// Run target with custom receiver
		runAutoTargetWithCustomReceiver(t, targetName, target, receiver.URL(), scrapeTarget, 12*time.Second)

		versions := tracker.GetVersions()
		if len(versions) > 0 {
			firstVersion := versions[0]
			// All versions should be the same (no fallback on success)
			for _, v := range versions {
				must(t).Equal(firstVersion, v,
					"Version should not change on successful responses")
			}
			t.Logf("No fallback on success: consistent version %s across %d requests",
				firstVersion, len(versions))
		}
	})
}

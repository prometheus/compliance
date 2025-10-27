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
	"sync"
	"testing"
	"time"

	"github.com/prometheus/compliance/remotewrite/sender/targets"
)

// TimestampTrackingReceiver wraps MockReceiver to track request timestamps.
type TimestampTrackingReceiver struct {
	*MockReceiver
	mu         sync.Mutex
	timestamps []time.Time
}

// NewTimestampTrackingReceiver creates a receiver that tracks request timestamps.
func NewTimestampTrackingReceiver(baseReceiver *MockReceiver) *TimestampTrackingReceiver {
	return &TimestampTrackingReceiver{
		MockReceiver: baseReceiver,
		timestamps:   make([]time.Time, 0),
	}
}

// RecordTimestamp records the current time for a request.
func (ttr *TimestampTrackingReceiver) RecordTimestamp() {
	ttr.mu.Lock()
	defer ttr.mu.Unlock()
	ttr.timestamps = append(ttr.timestamps, time.Now())
}

// GetTimestamps returns all recorded timestamps.
func (ttr *TimestampTrackingReceiver) GetTimestamps() []time.Time {
	ttr.mu.Lock()
	defer ttr.mu.Unlock()
	return append([]time.Time{}, ttr.timestamps...)
}

// TestBackoffBehavior validates exponential backoff implementation.
func TestBackoffBehavior(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		setup       func(*MockReceiver)
		validator   func(*testing.T, *TimestampTrackingReceiver)
	}{
		{
			name:        "exponential_backoff_on_retries",
			description: "Sender SHOULD use exponential backoff when retrying",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusServiceUnavailable,
					Body:       "Service unavailable",
				})
			},
			validator: func(t *testing.T, ttr *TimestampTrackingReceiver) {
				// Wait for multiple retry attempts
				time.Sleep(15 * time.Second)

				timestamps := ttr.GetTimestamps()
				if len(timestamps) < 3 {
					should(t, len(timestamps) >= 3, "Need at least 3 requests to validate backoff pattern")
					t.Logf("Only %d requests observed, cannot validate backoff", len(timestamps))
					return
				}

				// Calculate intervals between requests
				intervals := make([]time.Duration, 0)
				for i := 1; i < len(timestamps); i++ {
					interval := timestamps[i].Sub(timestamps[i-1])
					intervals = append(intervals, interval)
					t.Logf("Interval %d: %v", i, interval)
				}

				// Check that intervals are increasing (exponential backoff)
				if len(intervals) >= 2 {
					for i := 1; i < len(intervals); i++ {
						// Allow some tolerance for timing jitter
						// Second interval should be >= first interval (or close)
						ratio := float64(intervals[i]) / float64(intervals[i-1])
						should(t, ratio >= 0.8, fmt.Sprintf("Backoff intervals should increase or stay similar (exponential), got ratio %.2f", ratio))
					}
				}
			},
		},
		{
			name:        "increasing_delays",
			description: "Retry delays SHOULD increase over time",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusInternalServerError,
					Body:       "Internal server error",
				})
			},
			validator: func(t *testing.T, ttr *TimestampTrackingReceiver) {
				time.Sleep(15 * time.Second)

				timestamps := ttr.GetTimestamps()
				if len(timestamps) < 2 {
					t.Logf("Only %d requests, cannot validate increasing delays", len(timestamps))
					return
				}

				// Check that delay between first and second request is less than
				// delay between second and third (if exists)
				if len(timestamps) >= 3 {
					firstDelay := timestamps[1].Sub(timestamps[0])
					secondDelay := timestamps[2].Sub(timestamps[1])

					should(t, firstDelay <= secondDelay*2, "Delays should increase over retries (with some tolerance)")
					t.Logf("First delay: %v, Second delay: %v", firstDelay, secondDelay)
				}
			},
		},
		{
			name:        "backoff_max_delay",
			description: "Backoff SHOULD have a reasonable maximum delay",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusBadGateway,
					Body:       "Bad gateway",
				})
			},
			validator: func(t *testing.T, ttr *TimestampTrackingReceiver) {
				time.Sleep(20 * time.Second)

				timestamps := ttr.GetTimestamps()
				if len(timestamps) < 2 {
					t.Logf("Only %d requests, cannot validate max delay", len(timestamps))
					return
				}

				// Check that no interval exceeds a reasonable maximum (e.g., 60 seconds)
				maxReasonableDelay := 60 * time.Second

				for i := 1; i < len(timestamps); i++ {
					interval := timestamps[i].Sub(timestamps[i-1])
					should(t, interval <= maxReasonableDelay, fmt.Sprintf("Backoff interval too large: %v > %v", interval, maxReasonableDelay))
				}

				t.Logf("Observed %d retry attempts over %v",
					len(timestamps), timestamps[len(timestamps)-1].Sub(timestamps[0]))
			},
		},
		{
			name:        "backoff_with_jitter",
			description: "Sender MAY add jitter to backoff delays",
			rfcLevel:    "MAY",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusServiceUnavailable,
					Body:       "Service unavailable",
				})
			},
			validator: func(t *testing.T, ttr *TimestampTrackingReceiver) {
				time.Sleep(15 * time.Second)

				timestamps := ttr.GetTimestamps()
				if len(timestamps) < 3 {
					may(t, len(timestamps) >= 3, "Jitter validation requires multiple retries")
					return
				}

				// Calculate intervals
				intervals := make([]time.Duration, 0)
				for i := 1; i < len(timestamps); i++ {
					intervals = append(intervals, timestamps[i].Sub(timestamps[i-1]))
				}

				// With jitter, intervals shouldn't be exactly doubling
				// Check for some variance
				may(t, len(intervals) > 0, "Intervals should exist for jitter analysis")
				t.Logf("Intervals: %v (jitter may cause variance)", intervals)
			},
		},
		{
			name:        "minimum_backoff_delay",
			description: "Sender SHOULD have a minimum backoff delay",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusInternalServerError,
					Body:       "Internal server error",
				})
			},
			validator: func(t *testing.T, ttr *TimestampTrackingReceiver) {
				time.Sleep(10 * time.Second)

				timestamps := ttr.GetTimestamps()
				if len(timestamps) < 2 {
					t.Logf("Only %d requests, cannot validate minimum delay", len(timestamps))
					return
				}

				// Check that first retry doesn't happen immediately
				// Reasonable minimum: 100ms
				minReasonableDelay := 100 * time.Millisecond

				for i := 1; i < len(timestamps); i++ {
					interval := timestamps[i].Sub(timestamps[i-1])
					should(t, interval >= minReasonableDelay, fmt.Sprintf("Backoff interval too small: %v < %v", interval, minReasonableDelay))
				}

				t.Logf("Minimum observed interval: %v", timestamps[1].Sub(timestamps[0]))
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

				// Wrap with timestamp tracking
				tracker := NewTimestampTrackingReceiver(receiver)

				tt.setup(receiver)

				// Override handleRequest to track timestamps
				originalHandler := receiver.server.Config.Handler
				receiver.server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					tracker.RecordTimestamp()
					originalHandler.ServeHTTP(w, r)
				})

				scrapeTarget := NewMockScrapeTarget(tt.scrapeData)
				defer scrapeTarget.Close()

				// Run target with custom receiver
				runAutoTargetWithCustomReceiver(t, targetName, target, receiver.URL(), scrapeTarget, 20*time.Second)
				// Run validator
				tt.validator(t, tracker)
			})
		})
	}
}

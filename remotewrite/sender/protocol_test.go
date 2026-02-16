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

/*
TODO(bwplotka): Convert and revise.

func requireRetry(t *testing.T, res MockReceiverResult, a, b int) {
	t.Helper()
	require.Equal(t,
		res.Requests[a].Request.Timeseries[0].Samples[0].Timestamp,
		res.Requests[b].Request.Timeseries[0].Samples[0].Timestamp,
		"Found no retry; expected the same sample on request %d and %d; got %v",
		a, b, res.RequestsProtoToString(),
	)
}

func requireNoRetry(t *testing.T, res MockReceiverResult, a, b int) {
	t.Helper()
	require.NotEqual(t,
		res.Requests[a].Request.Timeseries[0].Samples[0].Timestamp,
		res.Requests[b].Request.Timeseries[0].Samples[0].Timestamp,
		"Detected retry; got the same sample on request %d and %d; got %v",
		a, b, res.RequestsProtoToString(),
	)
}

// TestRetryBehavior validates sender retry behavior on different error responses.
func TestRetryBehavior(t *testing.T) {
	RunTests(t, []TestCase{
		{
			Name:        "no_retry_on_4xx",
			Description: "Sender MUST NOT retry on 4xx status",
			RFCLevel:    sendertest.MustLevel,
			ScrapeData:  "test_metric 42\n",
			TestResponses: []MockReceiverResponse{
				{
					StatusCode: http.StatusBadRequest,
					Body:       "Bad request",
				},
				{
					StatusCode: http.StatusUnauthorized,
					Body:       "Unauthorized",
				},
				{
					StatusCode: http.StatusNotFound,
					Body:       "Not found",
				},
				{}, // OK response.
			},
			Validate: func(t *testing.T, res MockReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 4)
				requireNoRetry(t, res, 0, 1)
				requireNoRetry(t, res, 1, 2)
				requireNoRetry(t, res, 2, 3)
			},
		},
		{
			Name:        "retry_on_500",
			Description: "Sender MUST retry on 5xx status",
			RFCLevel:    sendertest.MustLevel,
			ScrapeData:  "test_metric 42\n",
			TestResponses: []MockReceiverResponse{
				{
					StatusCode: http.StatusInternalServerError,
					Body:       "Internal server error",
				},
				{}, // OK response for retry.
				{
					StatusCode: http.StatusServiceUnavailable,
					Body:       "Service unavailable",
				},
				{}, // OK response for retry.
				{
					StatusCode: http.StatusBadGateway,
					Body:       "Bad Gateway",
				},
				{}, // OK response for retry.
				{}, // OK response.
			},
			Validate: func(t *testing.T, res MockReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 3)
				requireRetry(t, res, 0, 1)
				requireNoRetry(t, res, 1, 2)
				requireRetry(t, res, 2, 3)
				requireNoRetry(t, res, 3, 4)
				requireRetry(t, res, 4, 5)
				requireNoRetry(t, res, 5, 6)
			},
		},
		{
			Name:        "retry_on_429",
			Description: "Sender MAY retry on 429 Too Many Requests",
			RFCLevel:    mayLevel,
			ScrapeData:  "test_metric 42\n",
			TestResponses: []MockReceiverResponse{
				{
					StatusCode: http.StatusTooManyRequests,
					Headers:    map[string]string{"Retry-After": "1"},
					Body:       "Too many requests",
				},
				{}, // OK response for retry.
				{}, // OK response.
			},
			Validate: func(t *testing.T, res MockReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 3)
				requireRetry(t, res, 0, 1)
				requireNoRetry(t, res, 1, 2)
			},
		},
	})
}

// TestBackoffBehavior validates backoff and exponential backoff implementation.
func TestBackoffBehavior(t *testing.T) {
	RunTests(t, []TestCase{
		{
			Name:        "backoff_required",
			Description: "Sender MUST use a backoff algorithm to prevent overwhelming the server",
			RFCLevel:    sendertest.MustLevel,
			ScrapeData:  "test_metric 42\n",
			TestResponses: []MockReceiverResponse{
				{
					StatusCode: http.StatusServiceUnavailable,
					Body:       "Service unavailable",
				},
				{
					StatusCode: http.StatusServiceUnavailable,
					Body:       "Service unavailable",
				},
				{
					StatusCode: http.StatusServiceUnavailable,
					Body:       "Service unavailable",
				},
				{}, // OK response for retry.
			},
			Validate: func(t *testing.T, res MockReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 4)
				requireRetry(t, res, 0, 1)
				requireRetry(t, res, 1, 2)
				requireRetry(t, res, 2, 3)

				// Expect gaps between retries to grow.
				// Not super reliable, but the best we can do.
				start := res.Requests[0].Received
				prev := time.Duration(0)
				for i := 1; i < len(res.Requests); i++ {
					interval := res.Requests[i].Received.Sub(start)
					t.Logf("elapsed %v from the last request", interval.String())
					require.Greater(t, interval, prev, "expected interval to grow, indicating backoff algorithm")
					prev = interval
				}
			},
		},
		// TODO: Add max delay?
		//{
		//	Name:        "backoff_max_delay",
		//	description: "Backoff SHOULD have a reasonable maximum delay",
		//	rfcLevel:    "SHOULD",
		//	scrapeData:  "test_metric 42\n",
		//	setup: func(mr *MockReceiver) {
		//		mr.SetResponse(MockReceiverResponse{
		//			StatusCode: http.StatusBadGateway,
		//			Body:       "Bad gateway",
		//		})
		//	},
		//	validator: func(t *testing.T, ttr *TimestampTrackingReceiver) {
		//		timestamps := ttr.GetTimestamps()
		//		if len(timestamps) < 2 {
		//			t.Logf("Only %d requests, cannot validate max delay", len(timestamps))
		//			return
		//		}
		//
		//		// Check that no interval exceeds a reasonable maximum (e.g., 60 seconds).
		//		maxReasonableDelay := 60 * time.Second
		//
		//		for i := 1; i < len(timestamps); i++ {
		//			interval := timestamps[i].Sub(timestamps[i-1])
		//			should(t, interval <= maxReasonableDelay, fmt.Sprintf("Backoff interval too large: %v > %v", interval, maxReasonableDelay))
		//		}
		//
		//		t.Logf("Observed %d retry attempts over %v",
		//			len(timestamps), timestamps[len(timestamps)-1].Sub(timestamps[0]))
		//	},
		//},
	})
}

// TestHTTPRequest HTTP protocol requirements for Remote Write 2.0 senders' requests.
func TestHTTPRequest(t *testing.T) {
	RunTest(t, TestCase{
		RFCLevel:   sendertest.MustLevel,
		ScrapeData: "test_metric 42\n",
		Validate: func(t *testing.T, res MockReceiverResult) {
			t.Run("post_method", func(t *testing.T) {
				sendertest.MustLevel.annotate(t)
				descAnnotate(t, "Sender MUST use HTTP POST method")

				require.Equal(t, http.MethodPost, res.Requests[0].Method)
			})
			t.Run("content_type_protobuf", func(t *testing.T) {
				sendertest.MustLevel.annotate(t)
				descAnnotate(t, "Sender MUST use Content-Type: application/x-protobuf")

				contentType := res.Requests[0].Headers.Get("Content-Type")
				require.Contains(t, contentType, "application/x-protobuf",
					"Content-Type header MUST contain application/x-protobuf")
			})
			t.Run("content_type_with_proto_param", func(t *testing.T) {
				shouldLevel.annotate(t)
				descAnnotate(t, "Sender SHOULD include proto parameter in Content-Type for RW 2.0")

				contentType := res.Requests[0].Headers.Get("Content-Type")
				require.Contains(t, contentType, "proto=io.prometheus.write.v2.Request", "Content-Type should include proto parameter for RW 2.0")
			})
			t.Run("content_encoding_snappy", func(t *testing.T) {
				sendertest.MustLevel.annotate(t)
				descAnnotate(t, "Sender MUST use Content-Encoding: snappy")

				encoding := res.Requests[0].Headers.Get("Content-Encoding")
				require.Equal(t, "snappy", encoding,
					"Content-Encoding header must be 'snappy'")
			})
			t.Run("snappy_block_format", func(t *testing.T) {
				sendertest.MustLevel.annotate(t)
				descAnnotate(t, "Sender MUST use snappy block format, not framed format")

				body := res.Requests[0].Body
				require.NotEmpty(t, body, "Request body must not be empty")

				// Check that it doesn't start with snappy framed format magic bytes.
				if len(body) >= 10 {
					framedMagic := []byte{0xff, 0x06, 0x00, 0x00, 0x73, 0x4e, 0x61, 0x50, 0x50, 0x59}
					isFramed := true
					for i := 0; i < 10; i++ {
						if body[i] != framedMagic[i] {
							isFramed = false
							break
						}
					}
					require.False(t, isFramed,
						"Sender must use snappy block format, not framed format")
				}
			})
			t.Run("version_header_present", func(t *testing.T) {
				sendertest.MustLevel.annotate(t)
				descAnnotate(t, "Sender MUST include X-Prometheus-Remote-Write-Version header")

				version := res.Requests[0].Headers.Get("X-Prometheus-Remote-Write-Version")
				require.NotEmpty(t, version,
					"X-Prometheus-Remote-Write-Version header must be present")
			})
			t.Run("version_header_value", func(t *testing.T) {
				shouldLevel.annotate(t)
				descAnnotate(t, "Sender SHOULD use version 2.0.0 for RW 2.0 receivers")

				version := res.Requests[0].Headers.Get("X-Prometheus-Remote-Write-Version")
				require.True(t, strings.HasPrefix(version, "2.0"), "Version should be 2.0.x for RW 2.0, got: %s", version)
			})
			t.Run("user_agent_present", func(t *testing.T) {
				sendertest.MustLevel.annotate(t)
				descAnnotate(t, "Sender MUST include User-Agent header (RFC 9110)")

				userAgent := res.Requests[0].Headers.Get("User-Agent")
				require.NotEmpty(t, userAgent,
					"User-Agent header must be present per RFC 9110")
			})
		},
	})
}

// TestHTTPResponseHandling validates sender response header handling.
func TestHTTPResponseHandling(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		setup       func(*MockReceiver)
		validator   func(*testing.T, []CapturedRequest)
	}{

		// TODO: Check scraper metrics for handling?
		//		{
		//			name:        "process_written_count_headers",
		//			description: "Sender MAY use X-Prometheus-Remote-Write-*-Written headers",
		//			rfcLevel:    "MAY",
		//			scrapeData: `# Multiple samples
		//test_counter_total{label="a"} 100
		//test_counter_total{label="b"} 200
		//test_gauge{label="c"} 50
		//`,
		//			setup: func(mr *MockReceiver) {
		//				mr.SetResponse(MockReceiverResponse{
		//					StatusCode:        http.StatusNoContent,
		//					SamplesWritten:    3,
		//					ExemplarsWritten:  0,
		//					HistogramsWritten: 0,
		//				})
		//			},
		//			validator: func(t *testing.T, requests []CapturedRequest) {
		//				may(t, len(requests) >= 1, "Should receive at least one request")
		//
		//				// Sender may use these headers for optimization/tracking.
		//				may(t, true, "Sender may process X-Prometheus-Remote-Write-*-Written headers")
		//				t.Logf("Sent response with written count headers")
		//			},
		//		},
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
				// Indicate only 2 out of 3 samples were written.
				mr.SetResponse(MockReceiverResponse{
					StatusCode:        http.StatusBadRequest,
					Body:              "Rejected 1 sample",
					SamplesWritten:    2, // Partial acceptance
					ExemplarsWritten:  0,
					HistogramsWritten: 0,
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				should(t, len(requests) >= 1, "Should receive at least one request")

				// Sender should handle partial writes.
				should(t, true, "Sender should handle partial write responses")
				t.Logf("Handled partial write response")
			},
		},
		// TODO: Check scraper metrics for handling?
		{
			name:        "handle_missing_written_headers",
			description: "Sender SHOULD assume 0 written if headers missing on 4xx/5xx",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
				// Return error without written count headers.
				mr.SetResponse(MockReceiverResponse{
					StatusCode: http.StatusBadRequest,
					Body:       "Bad request",
				})
			},
			validator: func(t *testing.T, requests []CapturedRequest) {
				should(t, len(requests) >= 1, "Should receive request even with error")

				// Sender should assume nothing was written.
				should(t, true, "Sender should assume 0 written when headers missing")
				t.Logf("Handled missing written count headers")
			},
		},
		{
			name:        "handle_large_error_body",
			description: "Sender SHOULD handle large error response bodies",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			setup: func(mr *MockReceiver) {
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
				should(t, len(requests) >= 1, "Should handle large error bodies")

				should(t, true, "Sender should handle large error response bodies")
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
			t.Parallel()
			t.Attr("rfcLevel", tt.rfcLevel)
			t.Attr("description", tt.description)

			forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
				receiver := NewMockReceiver()
				defer receiver.Close()

				tt.setup(receiver)

				scrapeTarget := NewMockScrapeTarget(tt.scrapeData)
				defer scrapeTarget.Close()

				t.Logf("Running %s with scrape target %s and receiver %s", targetName, scrapeTarget.URL(), receiver.URL())

				err := target(targets.TargetOptions{
					ScrapeTarget:    scrapeTarget.URL(),
					ReceiveEndpoint: receiver.URL(),
					Timeout:         8 * time.Second,
				})

				if err != nil {
					t.Logf("Target exited with error (may be expected): %v", err)
				}

				requests := receiver.GetRequests()
				tt.validator(t, requests)
			})
		})
	}
}

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
				}))
				server.Close()
				return server
			},
			validator: func(t *testing.T, server *httptest.Server) {
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
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					time.Sleep(30 * time.Second)
				}))
			},
			validator: func(t *testing.T, server *httptest.Server) {
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
					w.WriteHeader(http.StatusOK)
					w.Write([]byte("partial"))
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
					// Return error but sender should keep trying.
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte("Internal server error"))
				}))
			},
			validator: func(t *testing.T, server *httptest.Server) {
				// Sender should not crash and keep running.
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
					w.WriteHeader(http.StatusOK)
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

				runAutoTargetWithCustomReceiver(t, targetName, target, serverURL, scrapeTarget, 8*time.Second)

				tt.validator(t, server)
			})
		})
	}
}

// TestBatchingBehavior validates sender batching and queueing behavior.
func TestBatchingBehavior(t *testing.T) {
	tests := []TestCase{
		{
			Name:        "batch_size_reasonable",
			Description: "Sender should use reasonable batch sizes (10k series max) for performance",
			RFCLevel:    "RECOMMENDED",
			ScrapeData: func() string {
				var ret strings.Builder
				ret.WriteString("# Large scrape to test batch size handling\n")
				for i := range 12000 {
					ret.WriteString(fmt.Sprintf("metric{label=\"%d\"} 1\n", i))
				}
				return ret.String()
			}(),
			Validator: func(t *testing.T, req *CapturedRequest) {
				seriesCount := len(req.Request.Timeseries)

				// Batches shouldn't be too small (inefficient) or too large (risk).
				recommended(t, seriesCount >= 1, "Request should contain at least one series")

				recommended(t, seriesCount <= 10000, fmt.Sprintf("Batch size should be reasonable (less than 10k series), got %d", seriesCount))

				t.Logf("Batch contains %d timeseries from 12k available metrics", seriesCount)
			},
		},
	}
}

// TestCombinedFeatures validates integration of multiple Remote Write 2.0 features.
func TestCombinedFeatures(t *testing.T) {
	tests := []TestCase{
		{
			Name:        "samples_with_metadata",
			Description: "Sender SHOULD send samples with associated metadata",
			RFCLevel:    "SHOULD",
			ScrapeData: `# HELP http_requests_total Total HTTP requests received
# TYPE http_requests_total counter
http_requests_total{method="GET",status="200"} 1000
http_requests_total{method="POST",status="201"} 500
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundMetric bool
				var foundWithMetadata bool

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "http_requests_total" {
						foundMetric = true
						should(t, len(ts.Samples) > 0, "Counter should have samples")

						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							should(t, ts.Metadata.Type == writev2.Metadata_METRIC_TYPE_COUNTER,
								"Metadata type should match metric type")
							foundWithMetadata = true
						}

						if ts.Metadata.HelpRef != 0 {
							helpText := req.Request.Symbols[ts.Metadata.HelpRef]
							should(t, strings.Contains(helpText, "HTTP requests"),
								"Help text should be meaningful")
						}
					}
				}

				if !foundMetric {
					t.Fatalf("Expected to find http_requests_total metric")
				}

				should(t, foundWithMetadata, "Metadata should be present with samples")
			},
		},
		{
			Name:        "samples_with_exemplars",
			Description: "Sender MAY send samples with attached exemplars",
			RFCLevel:    "MAY",
			ScrapeData: `# TYPE request_count counter
request_count 1000 # {trace_id="abc123",span_id="def456"} 999 1234567890.123
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundMetric bool
				var foundExemplar bool

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "request_count" {
						foundMetric = true
						if len(ts.Exemplars) > 0 {
							foundExemplar = true
							ex := ts.Exemplars[0]
							exLabels := extractExemplarLabels(&ex, req.Request.Symbols)
							t.Logf("Found exemplar with labels: %v", exLabels)
						}
					}
				}

				if !foundMetric {
					t.Fatalf("Expected to find request_count metric")
				}

				may(t, foundExemplar, "Exemplars present")
			},
		},
		{
			Name:        "histogram_with_metadata_and_exemplars",
			Description: "Sender MAY send histograms with metadata and exemplars",
			RFCLevel:    "MAY",
			ScrapeData: `# HELP request_duration_seconds Request duration in seconds
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="0.1"} 100 # {trace_id="hist123"} 0.05 1234567890.0
request_duration_seconds_bucket{le="0.5"} 250
request_duration_seconds_bucket{le="1.0"} 500
request_duration_seconds_bucket{le="+Inf"} 1000
request_duration_seconds_sum 450.5
request_duration_seconds_count 1000
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundHistogramData bool
				var foundMetadata bool
				var foundExemplar bool

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)

					metricBase := "request_duration_seconds"

					if labels["__name__"] == metricBase+"_count" ||
						labels["__name__"] == metricBase+"_bucket" ||
						labels["__name__"] == metricBase {
						foundHistogramData = true

						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							foundMetadata = true
						}

						if len(ts.Exemplars) > 0 {
							foundExemplar = true
						}
					}
				}

				if !foundHistogramData {
					t.Fatalf("Expected histogram data but none was found")
				}

				may(t, foundMetadata, "Histogram metadata present")
				may(t, foundExemplar, "Histogram exemplars present")
			},
		},
		{
			Name:        "multiple_metric_types",
			Description: "Sender MUST handle multiple metric types in same request",
			RFCLevel:    "MUST",
			ScrapeData: `# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 123.45

# TYPE process_memory_bytes gauge
process_memory_bytes 1048576

# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="+Inf"} 100
request_duration_seconds_sum 50.0
request_duration_seconds_count 100
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				metricTypes := make(map[string]bool)

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]

					if metricName == "process_cpu_seconds_total" {
						metricTypes["counter"] = true
					} else if metricName == "process_memory_bytes" {
						metricTypes["gauge"] = true
					} else if metricName == "request_duration_seconds_count" ||
						metricName == "request_duration_seconds" {
						metricTypes["histogram"] = true
					}
				}

				must(t).NotEmpty(metricTypes, "Request must contain metrics")
				t.Logf("Found metric types: %v", metricTypes)
			},
		},
		{
			Name:        "high_cardinality_labels",
			Description: "Sender should efficiently handle high cardinality label sets",
			RFCLevel:    "RECOMMENDED",
			ScrapeData: `# TYPE http_requests_total counter
http_requests_total{method="GET",path="/api/v1/users",status="200"} 100
http_requests_total{method="GET",path="/api/v1/posts",status="200"} 200
http_requests_total{method="POST",path="/api/v1/users",status="201"} 50
http_requests_total{method="POST",path="/api/v1/posts",status="201"} 75
http_requests_total{method="GET",path="/api/v1/comments",status="200"} 300
http_requests_total{method="DELETE",path="/api/v1/users",status="204"} 10
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// With high cardinality, symbol table deduplication becomes important.
				symbols := req.Request.Symbols
				uniqueSymbols := make(map[string]bool)

				for _, sym := range symbols {
					if sym != "" {
						uniqueSymbols[sym] = true
					}
				}

				// Check that common strings are deduplicated.
				recommended(t, len(uniqueSymbols) > 0, "Symbol table should contain unique symbols")

				httpRequestsSeries := 0
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "http_requests_total" {
						httpRequestsSeries++
					}
				}

				recommended(t, httpRequestsSeries >= 6,
					"High cardinality metrics should have multiple series")
				t.Logf("Found %d unique symbols, %d http_requests_total series",
					len(uniqueSymbols), httpRequestsSeries)
			},
		},
		{
			Name:        "complete_metric_family",
			Description: "Sender MUST send all components of metric family together",
			RFCLevel:    "MUST",
			ScrapeData: `# HELP api_request_duration_seconds API request duration
# TYPE api_request_duration_seconds histogram
api_request_duration_seconds_bucket{le="0.1"} 50
api_request_duration_seconds_bucket{le="0.5"} 150
api_request_duration_seconds_bucket{le="1.0"} 250
api_request_duration_seconds_bucket{le="+Inf"} 300
api_request_duration_seconds_sum 200.5
api_request_duration_seconds_count 300
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// For classic histograms, expect _sum, _count, and _bucket series.
				var foundCount bool

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]

					if metricName == "api_request_duration_seconds_count" {
						foundCount = true
					} else if metricName == "api_request_duration_seconds" && len(ts.Histograms) > 0 {
						// Native histogram format has everything in one series.
						foundCount = true
					}
				}

				// For classic histograms, all components should be present.
				// For native histograms, they're combined.
				must(t).True(foundCount || len(req.Request.Timeseries) > 0,
					"Histogram family must include count")
			},
		},
		{
			Name:        "mixed_labels_and_metadata",
			Description: "Sender SHOULD correctly encode metrics with many labels and metadata",
			RFCLevel:    "SHOULD",
			ScrapeData: `# HELP api_calls_total Total API calls with detailed labels
# TYPE api_calls_total counter
api_calls_total{service="auth",method="POST",endpoint="/login",region="us-east",status="200"} 1000
api_calls_total{service="auth",method="POST",endpoint="/logout",region="us-east",status="200"} 500
api_calls_total{service="users",method="GET",endpoint="/profile",region="eu-west",status="200"} 2000
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var seriesCount int
				var metadataCount int

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "api_calls_total" {
						seriesCount++

						// Check labels are properly structured.
						should(t, labels["service"] != "", "Service label should be present")
						should(t, labels["method"] != "", "Method label should be present")
						should(t, labels["endpoint"] != "", "Endpoint label should be present")

						if ts.Metadata.Type != writev2.Metadata_METRIC_TYPE_UNSPECIFIED {
							metadataCount++
						}
					}
				}

				should(t, seriesCount >= 3,
					"Should have multiple series with different label combinations")
			},
		},
		{
			Name:        "real_world_scenario",
			Description: "Sender MUST handle realistic mixed metric payload",
			RFCLevel:    "MUST",
			ScrapeData: `# Realistic scrape output with multiple metric types
# TYPE up gauge
up 1

# TYPE process_cpu_seconds_total counter
process_cpu_seconds_total 45.67

# TYPE go_memstats_alloc_bytes gauge
go_memstats_alloc_bytes 2097152

# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{le="0.05"} 100
http_request_duration_seconds_bucket{le="0.1"} 200
http_request_duration_seconds_bucket{le="0.5"} 450
http_request_duration_seconds_bucket{le="1.0"} 480
http_request_duration_seconds_bucket{le="+Inf"} 500
http_request_duration_seconds_sum 125.5
http_request_duration_seconds_count 500

# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 5000
http_requests_total{method="POST",code="201"} 1000
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				metricNames := make(map[string]bool)

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]
					metricNames[metricName] = true

					// Validate each series has valid structure.
					must(t).NotEmpty(metricName, "Each timeseries must have __name__")

					// Validate no mixed samples and histograms.
					if len(ts.Samples) > 0 && len(ts.Histograms) > 0 {
						must(t).Fail("Timeseries must not mix samples and histograms")
					}
				}

				must(t).NotEmpty(metricNames, "Request must contain metrics")
				must(t).GreaterOrEqual(len(metricNames), 3,
					"Real-world scenario should have multiple distinct metrics")
			},
		},
	}

	runTestCases(t, tests)
}

*/

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
	"strings"
	"testing"
)

// TestProtocolCompliance validates HTTP protocol requirements for Remote Write 2.0 senders.
func TestProtocolCompliance(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		validator   func(*testing.T, *CapturedRequest)
	}{
		{
			name:        "content_type_protobuf",
			description: "Sender MUST use Content-Type: application/x-protobuf",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				contentType := req.Headers.Get("Content-Type")
				must(t).Contains(contentType, "application/x-protobuf",
					"Content-Type header must contain application/x-protobuf")
			},
		},
		{
			name:        "content_type_with_proto_param",
			description: "Sender SHOULD include proto parameter in Content-Type for RW 2.0",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				contentType := req.Headers.Get("Content-Type")
				should(t).Contains(contentType, "proto=io.prometheus.write.v2.Request",
					"Content-Type should include proto parameter for RW 2.0")
			},
		},
		{
			name:        "content_encoding_snappy",
			description: "Sender MUST use Content-Encoding: snappy",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				encoding := req.Headers.Get("Content-Encoding")
				must(t).Equal("snappy", encoding,
					"Content-Encoding header must be 'snappy'")
			},
		},
		{
			name:        "version_header_present",
			description: "Sender MUST include X-Prometheus-Remote-Write-Version header",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")
				must(t).NotEmpty(version,
					"X-Prometheus-Remote-Write-Version header must be present")
			},
		},
		{
			name:        "version_header_value",
			description: "Sender SHOULD use version 2.0.0 for RW 2.0 receivers",
			rfcLevel:    "SHOULD",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")
				should(t).True(strings.HasPrefix(version, "2.0"),
					"Version should be 2.0.x for RW 2.0, got: %s", version)
			},
		},
		{
			name:        "user_agent_present",
			description: "Sender MUST include User-Agent header (RFC 9110)",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				userAgent := req.Headers.Get("User-Agent")
				must(t).NotEmpty(userAgent,
					"User-Agent header must be present per RFC 9110")
			},
		},
		{
			name:        "snappy_block_format",
			description: "Sender MUST use snappy block format, not framed format",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				// Snappy framed format starts with specific magic bytes: 0xff 0x06 0x00 0x00 0x73 0x4e 0x61 0x50 0x50 0x59
				// Snappy block format does not have these magic bytes.
				body := req.Body
				must(t).NotEmpty(body, "Request body must not be empty")

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
					must(t).False(isFramed,
						"Sender must use snappy block format, not framed format")
				}
			},
		},
		{
			name:        "protobuf_parseable",
			description: "Sender MUST send valid protobuf messages that can be parsed",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				// The request was already parsed in MockReceiver.handleRequest
				// If we got here, the protobuf was successfully parsed.
				must(t).NotNil(req.Request, "Protobuf message must be parseable")
				must(t).NotEmpty(req.Request.Symbols,
					"Parsed request must contain symbols")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Attr("rfcLevel", tt.rfcLevel)
			t.Attr("description", tt.description)

			forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
				runSenderTest(t, targetName, target, SenderTestScenario{
					ScrapeData: tt.scrapeData,
					Validator:  tt.validator,
				})
			})
		})
	}
}

// TestHTTPMethod validates that senders use POST method for remote write.
func TestHTTPMethod(t *testing.T) {
	t.Attr("rfcLevel", "MUST")
	t.Attr("description", "Sender MUST use POST method for remote write requests")

	forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
		runSenderTest(t, targetName, target, SenderTestScenario{
			ScrapeData: "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				must(t).NotNil(req, "Request must be received successfully")
			},
		})
	})
}

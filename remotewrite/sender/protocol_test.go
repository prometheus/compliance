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
	"strings"
	"testing"

	"github.com/prometheus/compliance/remotewrite/sender/targets"
)

// TestProtocolCompliance validates HTTP protocol requirements for Remote Write 2.0 senders.
func TestProtocolCompliance_Old(t *testing.T) {
	t.Skip("TODO: Revise and move to a new framework")

	tests := []TestCase{
		{
			Name:        "content_type_protobuf",
			Description: "Sender MUST use Content-Type: application/x-protobuf",
			RFCLevel:    "MUST",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				contentType := req.Headers.Get("Content-Type")
				must(t).Contains(contentType, "application/x-protobuf",
					"Content-Type header must contain application/x-protobuf")
			},
		},
		{
			Name:        "content_type_with_proto_param",
			Description: "Sender SHOULD include proto parameter in Content-Type for RW 2.0",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				contentType := req.Headers.Get("Content-Type")
				should(t, strings.Contains(contentType, "proto=io.prometheus.write.v2.Request"), "Content-Type should include proto parameter for RW 2.0")
			},
		},
		{
			Name:        "content_encoding_snappy",
			Description: "Sender MUST use Content-Encoding: snappy",
			RFCLevel:    "MUST",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				encoding := req.Headers.Get("Content-Encoding")
				must(t).Equal("snappy", encoding,
					"Content-Encoding header must be 'snappy'")
			},
		},
		{
			Name:        "version_header_present",
			Description: "Sender MUST include X-Prometheus-Remote-Write-Version header",
			RFCLevel:    "MUST",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")
				must(t).NotEmpty(version,
					"X-Prometheus-Remote-Write-Version header must be present")
			},
		},
		{
			Name:        "version_header_value",
			Description: "Sender SHOULD use version 2.0.0 for RW 2.0 receivers",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				version := req.Headers.Get("X-Prometheus-Remote-Write-Version")
				should(t, strings.HasPrefix(version, "2.0"), fmt.Sprintf("Version should be 2.0.x for RW 2.0, got: %s", version))
			},
		},
		{
			Name:        "user_agent_present",
			Description: "Sender MUST include User-Agent header (RFC 9110)",
			RFCLevel:    "MUST",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				userAgent := req.Headers.Get("User-Agent")
				must(t).NotEmpty(userAgent,
					"User-Agent header must be present per RFC 9110")
			},
		},
		{
			Name:        "snappy_block_format",
			Description: "Sender MUST use snappy block format, not framed format",
			RFCLevel:    "MUST",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
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
			Name:        "protobuf_parseable",
			Description: "Sender MUST send valid protobuf messages that can be parsed",
			RFCLevel:    "MUST",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				// The request was already parsed in MockReceiver.handleRequest. If we got here, the protobuf was successfully parsed.
				must(t).NotNil(req.Request, "Protobuf message must be parseable")
				must(t).NotEmpty(req.Request.Symbols,
					"Parsed request must contain symbols")
			},
		},
	}

	runTestCases(t, tests)
}

// TestHTTPMethod validates that senders use POST method for remote write.
func TestHTTPMethod_Old(t *testing.T) {
	t.Skip("TODO: Revise and move to a new framework")

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

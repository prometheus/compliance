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
	"bytes"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/stretchr/testify/require"
)

// snappyFramedHeader is the 10-byte stream identifier chunk magic used by Snappy framed format.
var snappyFramedHeader = []byte{0xff, 0x06, 0x00, 0x00, 0x73, 0x4e, 0x61, 0x50, 0x50, 0x59}

func protocolTests(mode Mode) []Test {
	protoParamLevel := ShouldLevel
	if mode != ModeNone {
		protoParamLevel = MustLevel
	}

	validateCases := []ValidateCase{
		{
			Name:        "http_method_post",
			Description: "Senders MUST send Protobuf message in the body of an HTTP POST request",
			RFCLevel:    MustLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				for _, req := range res.Requests {
					require.Equal(t, http.MethodPost, req.Method, "HTTP method MUST be POST")
				}
			},
		},
		{
			Name:        "content_encoding_snappy",
			Description: "Senders MUST use the snappy value for Content-Encoding header",
			RFCLevel:    MustLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				for _, req := range res.Requests {
					require.Equal(t, "snappy", req.Headers.Get("Content-Encoding"), "Content-Encoding header MUST be snappy")
				}
			},
		},
		{
			Name:        "snappy_block_format",
			Description: "Snappy block format MUST be used and framed format MUST NOT be used",
			RFCLevel:    MustLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				for _, req := range res.Requests {
					require.NotEmpty(t, req.Body, "Request body must not be empty")
					require.False(t, bytes.HasPrefix(req.Body, snappyFramedHeader), "Sender MUST use Snappy block format, not framed format")
					require.NotNil(t, req.RW2, "Request body MUST decode into io.prometheus.write.v2.Request")
					require.NotEmpty(t, req.RW2.Timeseries, "Decoded io.prometheus.write.v2.Request MUST contain timeseries")
				}
			},
		},
		{
			Name:        "content_type_protobuf",
			Description: "Senders MUST use application/x-protobuf as the media type with valid parameters",
			RFCLevel:    MustLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				for _, req := range res.Requests {
					ct := req.Headers.Get("Content-Type")
					require.NotEmpty(t, ct, "Content-Type header MUST be present")

					mediaType, params, err := mime.ParseMediaType(ct)
					require.NoError(t, err, "Content-Type header MUST follow RFC 9110")
					require.Equal(t, "application/x-protobuf", mediaType, "Media type MUST be application/x-protobuf")

					if protoParam, ok := params["proto"]; ok {
						require.Equal(t, string(remote.WriteV2MessageType), protoParam, "proto parameter MUST be io.prometheus.write.v2.Request")
					}
					if modeParam, ok := params["mode"]; ok {
						expectedMode := "none"
						if mode != ModeNone {
							expectedMode = string(mode)
						}
						require.Equal(t, expectedMode, modeParam, "mode parameter MUST be %q", expectedMode)
					}
				}
			},
		},
		{
			Name:        "content_type_proto_param",
			Description: "Senders SHOULD include proto=io.prometheus.write.v2.Request in Content-Type",
			RFCLevel:    protoParamLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				for _, req := range res.Requests {
					_, params, err := mime.ParseMediaType(req.Headers.Get("Content-Type"))
					require.NoError(t, err)
					require.Equal(t, string(remote.WriteV2MessageType), params["proto"],
						"Content-Type SHOULD include proto=io.prometheus.write.v2.Request")
				}
			},
		},
	}

	if mode != ModeNone {
		validateCases = append(validateCases, ValidateCase{
			Name:        "content_type_mode_param",
			Description: fmt.Sprintf("Senders MUST use Content-Type with mode=%s in %s mode", mode, mode),
			RFCLevel:    MustLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				for _, req := range res.Requests {
					_, params, err := mime.ParseMediaType(req.Headers.Get("Content-Type"))
					require.NoError(t, err)
					require.Equal(t, string(mode), params["mode"],
						"Content-Type MUST include mode=%s in %s mode", mode, mode)
				}
			},
		})
	}

	validateCases = append(validateCases,
		ValidateCase{
			Name:        "version_header_present",
			Description: "Senders MUST send X-Prometheus-Remote-Write-Version header",
			RFCLevel:    MustLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				for _, req := range res.Requests {
					require.NotEmpty(t, req.Headers.Get("X-Prometheus-Remote-Write-Version"),
						"X-Prometheus-Remote-Write-Version header MUST be present")
				}
			},
		},
		ValidateCase{
			Name:        "version_header_value",
			Description: "Senders SHOULD use X-Prometheus-Remote-Write-Version: 2.0.0 when sending RW 2.0",
			RFCLevel:    ShouldLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				for _, req := range res.Requests {
					version := req.Headers.Get("X-Prometheus-Remote-Write-Version")
					require.True(t, strings.HasPrefix(version, "2.0"),
						"X-Prometheus-Remote-Write-Version SHOULD be 2.0.x, got %q", version)
				}
			},
		},
		ValidateCase{
			Name:        "user_agent_present",
			Description: "Senders MUST include a User-Agent header",
			RFCLevel:    MustLevel,
			Validate: func(t *testing.T, res ReceiverResult) {
				for _, req := range res.Requests {
					require.NotEmpty(t, req.Headers.Get("User-Agent"), "User-Agent header MUST be present")
				}
			},
		},
	)

	tests := []Test{
		{
			Name:        "protocol",
			Description: "Senders MUST send a Snappy block-compressed Protobuf message via HTTP POST with required headers",
			RFCLevel:    MustLevel,
			ScrapeData:  "# TYPE test_metric gauge\ntest_metric 42\n",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.NotEmpty(t, res.Requests, "Should receive at least 1 request")
			},
			ValidateCases: validateCases,
		},
	}

	if mode == ModeNone {
		tests = append(tests, Test{
			Name:        "rw1_compatibility",
			Description: "Senders MAY support 1.x Receivers using prometheus.WriteRequest",
			RFCLevel:    MayLevel,
			ScrapeData:  "# TYPE test_metric gauge\ntest_metric{z=\"1\",a=\"2\"} 42\n",
			Version:     remote.WriteV1MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.NotEmpty(t, res.Requests, "Should receive at least 1 request")
			},
			ValidateCases: []ValidateCase{
				{
					Name:        "content_type",
					Description: "When talking to 1.x Receivers, Senders SHOULD use Content-Type: application/x-protobuf",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							mediaType, params, err := mime.ParseMediaType(req.Headers.Get("Content-Type"))
							require.NoError(t, err)
							require.Equal(t, "application/x-protobuf", mediaType)
							if protoParam, ok := params["proto"]; ok {
								require.Equal(t, string(remote.WriteV1MessageType), protoParam)
							}
						}
					},
				},
				{
					Name:        "version_header",
					Description: "When talking to 1.x Receivers, Senders MUST use X-Prometheus-Remote-Write-Version: 0.1.0",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							require.Equal(t, "0.1.0", req.Headers.Get("X-Prometheus-Remote-Write-Version"),
								"X-Prometheus-Remote-Write-Version MUST be 0.1.0 for RW 1.0")
						}
					},
				},
				{
					Name:        "payload_valid",
					Description: "When talking to 1.x Receivers, Senders MUST encode valid prometheus.WriteRequest with sorted labels",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW1
						require.NotNil(t, req, "RW 1.0 request must be decoded")
						require.NotEmpty(t, req.Timeseries, "RW 1.0 request must contain timeseries")
						for _, ts := range req.Timeseries {
							require.True(t, isSortedRW1(ts.Labels), "RW 1.0 labels must be sorted lexicographically")
						}
					},
				},
			},
		})
	}

	return tests
}

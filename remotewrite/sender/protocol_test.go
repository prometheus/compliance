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

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/stretchr/testify/require"
)

func protocolTests() []Test {
	return []Test{
		{
			Name:        "protocol_headers_and_method",
			Description: "Sender MUST use POST, Snappy, and set required headers",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData:  "test_metric 42\n",
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)
				for _, req := range res.Requests {
					require.Equal(t, "POST", req.Method, "HTTP method MUST be POST")
					require.Equal(t, "snappy", req.Headers.Get("Content-Encoding"), "Content-Encoding MUST be snappy")

					contentType := req.Headers.Get("Content-Type")
					require.Contains(t, contentType, "application/x-protobuf", "Content-Type MUST be protobuf")

					// SHOULD headers
					require.NotEmpty(t, req.Headers.Get("User-Agent"), "User-Agent SHOULD be set")

					// The X-Prometheus-Remote-Write-Version header is required for 2.0
					require.Equal(t, "2.0.0", req.Headers.Get("X-Prometheus-Remote-Write-Version"), "X-Prometheus-Remote-Write-Version MUST be 2.0.0 for V2")
				}
			},
		},
	}
}

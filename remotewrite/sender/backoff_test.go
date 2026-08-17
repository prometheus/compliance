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
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/stretchr/testify/require"
)

func backoffTests() []Test {
	return []Test{
		{
			Name:        "backoff_required",
			Description: "Sender MUST use a backoff algorithm to prevent overwhelming the server",
			RFCLevel:    MustLevel,
			ScrapeData:  "test_metric 42\n",
			Version:     remote.WriteV2MessageType,
			TestResponses: []ReceiverResponse{
				{
					StatusCode: http.StatusServiceUnavailable,
					Body:       "Service unavailable",
				},
				{
					StatusCode: http.StatusServiceUnavailable,
					Body:       "Service unavailable",
				},
				{
					StatusCode: http.StatusNoContent,
				},
			},
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 3, "Expected at least 3 requests (initial + 2 retries)")

				// MUST: Verify that backoff exists (delay between all requests).
				minBackoffDelay := 1 * time.Millisecond
				for i := 1; i < len(res.Requests); i++ {
					interval := res.Requests[i].Received.Sub(res.Requests[i-1].Received)
					require.GreaterOrEqual(t, interval, minBackoffDelay,
						"Sender MUST implement backoff between all retry attempts, interval %d was: %v", i, interval)
				}
			},
		},
	}
}

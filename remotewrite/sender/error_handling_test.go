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

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/stretchr/testify/require"
)

func errorHandlingTests() []Test {
	return []Test{
		{
			Name:        "sender_continues_after_errors",
			Description: "Sender MUST continue running after recoverable errors",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData:  "test_metric 42\n",
			TestResponses: []ReceiverResponse{
				{StatusCode: http.StatusInternalServerError, Body: "Internal server error"},
				{StatusCode: http.StatusNoContent},
			},
			Validate: func(t *testing.T, res ReceiverResult) {
				// Sender should have successfully retried and continued.
				require.GreaterOrEqual(t, len(res.Requests), 2, "Sender must continue running after errors and retry")
			},
		},
	}
}

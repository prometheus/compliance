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

func retryTests() []Test {
	return []Test{
		{
			Name:        "no_retry_on_400",
			Description: "Sender MUST NOT retry on 400 Bad Request",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData:  "test_metric 42\n",
			TestResponses: []ReceiverResponse{
				{StatusCode: http.StatusBadRequest},
				{StatusCode: http.StatusBadRequest},
			},
			Validate: func(t *testing.T, res ReceiverResult) {
				require.LessOrEqual(t, len(res.Requests), 2, "Sender should not retry on 400 Bad Request")
			},
		},
		{
			Name:        "no_retry_on_401",
			Description: "Sender MUST NOT retry on 401 Unauthorized",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData:  "test_metric 42\n",
			TestResponses: []ReceiverResponse{
				{StatusCode: http.StatusUnauthorized},
				{StatusCode: http.StatusUnauthorized},
			},
			Validate: func(t *testing.T, res ReceiverResult) {
				require.LessOrEqual(t, len(res.Requests), 2, "Sender should not retry on 401 Unauthorized")
			},
		},
		{
			Name:        "retry_on_500",
			Description: "Sender MUST retry on 500 Internal Server Error",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData:  "test_metric 42\n",
			TestResponses: []ReceiverResponse{
				{StatusCode: http.StatusInternalServerError},
				{StatusCode: http.StatusInternalServerError},
				{StatusCode: http.StatusNoContent},
			},
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 3, "Sender should retry on 500 Internal Server Error")
			},
		},
		{
			Name:        "retry_on_503",
			Description: "Sender MUST retry on 503 Service Unavailable",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData:  "test_metric 42\n",
			TestResponses: []ReceiverResponse{
				{StatusCode: http.StatusServiceUnavailable},
				{StatusCode: http.StatusServiceUnavailable},
				{StatusCode: http.StatusNoContent},
			},
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 3, "Sender should retry on 503 Service Unavailable")
			},
		},
	}
}

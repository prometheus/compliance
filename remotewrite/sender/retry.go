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
			Name:        "retry_on_5xx",
			Description: "Senders MUST retry requests on HTTP 5xx responses using a backoff algorithm.",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `# TYPE retry_5xx_metric gauge
retry_5xx_metric 100
`,
			TestResponses: []ReceiverResponse{
				{
					StatusCode: http.StatusServiceUnavailable,
					Body:       "temporary server overload",
				},
				{
					StatusCode: http.StatusInternalServerError,
					Body:       "internal server error",
				},
				{
					StatusCode: http.StatusNoContent,
				},
			},
			Validate: func(t *testing.T, res ReceiverResult) {
				require.Len(t, res.Requests, 3)
				for i := range res.Requests {
					require.NotNil(t, res.Requests[i].RW2)
				}
			},
			ValidateCases: []ValidateCase{
				{
					Name:        "retries_preserve_samples",
					Description: "Sender MUST retry the failed batch on 503 and 500 without losing samples.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						res0 := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "retry_5xx_metric")
						ts0 := flattenTimeseriesResult(t, res0).TimeSeries
						require.NotEmpty(t, ts0.Samples)

						res1 := requireTimeseriesByMetricName(t, res.Requests[1].RW2, "retry_5xx_metric")
						ts1 := flattenTimeseriesResult(t, res1).TimeSeries
						require.Equal(t, ts0.Samples, ts1.Samples, "First retry must resend the exact same samples")

						res2 := requireTimeseriesByMetricName(t, res.Requests[2].RW2, "retry_5xx_metric")
						ts2 := flattenTimeseriesResult(t, res2).TimeSeries
						require.Equal(t, ts0.Samples, ts2.Samples, "Second retry must resend the exact same samples")
					},
				},
				{
					Name:        "backoff_between_retries",
					Description: "Sender MUST delay between retry attempts using a backoff algorithm.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						delay1 := res.Requests[1].Received.Sub(res.Requests[0].Received)
						delay2 := res.Requests[2].Received.Sub(res.Requests[1].Received)
						require.Positive(t, delay1, "First retry must occur after a positive backoff delay")
						require.Positive(t, delay2, "Second retry must occur after a positive backoff delay")
					},
				},
			},
		},
		{
			Name:        "no_retry_on_4xx",
			Description: "Senders MUST NOT retry requests on HTTP 4xx responses (other than 429).",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `# TYPE no_retry_4xx_metric gauge
no_retry_4xx_metric 50
`,
			TestResponses: []ReceiverResponse{
				{
					StatusCode: http.StatusBadRequest,
					Body:       "invalid request payload",
				},
				{
					StatusCode: http.StatusNoContent,
				},
			},
			Validate: func(t *testing.T, res ReceiverResult) {
				require.Len(t, res.Requests, 2)
				require.NotNil(t, res.Requests[0].RW2)
				require.NotNil(t, res.Requests[1].RW2)
			},
			ValidateCases: []ValidateCase{
				{
					Name:        "drops_batch_on_400",
					Description: "Sender MUST NOT retry a batch rejected with 400 Bad Request and MUST proceed to subsequent batches.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						res0 := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "no_retry_4xx_metric")
						ts0 := flattenTimeseriesResult(t, res0).TimeSeries
						require.NotEmpty(t, ts0.Samples)
						maxTS0 := ts0.Samples[len(ts0.Samples)-1].Timestamp

						res1 := requireTimeseriesByMetricName(t, res.Requests[1].RW2, "no_retry_4xx_metric")
						ts1 := flattenTimeseriesResult(t, res1).TimeSeries
						require.NotEmpty(t, ts1.Samples)
						minTS1 := ts1.Samples[0].Timestamp

						require.Greater(t, minTS1, maxTS0, "Rejected 400 batch must be dropped and not resent in the subsequent request")
					},
				},
			},
		},
		{
			Name:        "retry_on_429",
			Description: "Senders MAY retry requests on HTTP 429 Too Many Requests responses when configured.",
			RFCLevel:    MayLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `# TYPE retry_429_metric gauge
retry_429_metric 75
`,
			TestResponses: []ReceiverResponse{
				{
					StatusCode: http.StatusTooManyRequests,
					Body:       "rate limit exceeded",
				},
				{
					StatusCode: http.StatusNoContent,
				},
			},
			Validate: func(t *testing.T, res ReceiverResult) {
				require.Len(t, res.Requests, 2)
				require.NotNil(t, res.Requests[0].RW2)
				require.NotNil(t, res.Requests[1].RW2)
			},
			ValidateCases: []ValidateCase{
				{
					Name:        "retries_429_when_configured",
					Description: "Sender MAY retry the batch on 429 Too Many Requests.",
					RFCLevel:    MayLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						res0 := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "retry_429_metric")
						ts0 := flattenTimeseriesResult(t, res0).TimeSeries
						require.NotEmpty(t, ts0.Samples)

						res1 := requireTimeseriesByMetricName(t, res.Requests[1].RW2, "retry_429_metric")
						ts1 := flattenTimeseriesResult(t, res1).TimeSeries
						require.Equal(t, ts0.Samples, ts1.Samples, "Retry on 429 should resend the same batch")
					},
				},
			},
		},
	}
}

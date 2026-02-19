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
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/stretchr/testify/require"
)

func formatTimeAsOpenMetricsTimestamp(t time.Time) string {
	v := float64(timestamp.FromTime(t)) / 1000
	return labels.FormatOpenMetricsFloat(v)
}

func samplesTests() []Test {
	timeNow := time.Now()
	st := timeNow.Add(-2 * time.Hour)
	explicitTS := timeNow

	return []Test{
		{
			// TODO(bwplotka): Fix 2.0 spec - MUST value and timestamp are not mentioned (only in proto).
			Name:        "samples",
			Description: "Senders MUST send valid samples",
			RFCLevel:    MustLevel,
			ScrapeData: fmt.Sprintf(`# TYPE test_counter counter
test_counter_total 101.13
test_counter_created %v
# TYPE test_histogram histogram
test_histogram_count 100
test_histogram_sum 250.0
test_histogram_bucket{le="+Inf"} 100
test_histogram_created %v
# TYPE test_gauge_with_ts gauge
test_gauge_with_ts 2 %v
`,
				timestamp.FromTime(st),
				timestamp.FromTime(st),
				formatTimeAsOpenMetricsTimestamp(explicitTS),
			),
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1, "Should receive at least 6 timeseries")
				require.Greater(t, len(res.Requests[0].RW2.Timeseries), 6, "Request must contain at least 6 timeseries")
			},
			ValidateCases: []ValidateCase{
				{
					Name:        "value",
					Description: "Sample MUST have value",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_counter_total")
						require.NotEmpty(t, ts.Samples, "Timeseries test_counter_total must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_counter_total must contain a single sample")
						require.Equal(t, 101.13, ts.Samples[0].Value,
							"Sample value for test_counter_total must be correctly encoded")
					},
				},
				{
					Name:        "timestamp",
					Description: "Sample MUST have timestamp",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, ts := range res.Requests[0].RW2.Timeseries {
							require.Len(t, ts.Samples, 1, "Timeseries must contain a single sample")
							require.GreaterOrEqual(t, ts.Samples[0].Timestamp, timestamp.FromTime(timeNow), "Timeseries must contain a fresh timestamp")
						}
					},
				},
				// TODO(bwplotka): Make it work, somehow OM parser kills test_gauge_with_ts metric with no log.
				//{
				//	Name:        "explicit_timestamp",
				//	Description: "Sample with the explicit timestamp work",
				//	RFCLevel:    sendertest.RecommendedLevel, // Prometheus spec, not Remote Write.
				//	Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				//		ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_gauge_with_ts")
				//		require.NotEmpty(t, ts.Samples, "Timeseries test_gauge_with_ts must contain samples")
				//		require.Len(t, ts.Samples, 1, "Timeseries test_gauge_with_ts must contain a single sample")
				//		require.Equal(t, timestamp.FromTime(explicitTS), ts.Samples[0].Timestamp)
				//	},
				//},
				{
					Name:        "start_timestamp for counters",
					Description: "Sample SHOULD have start timestamp for a counter",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_counter_total")
						require.NotEmpty(t, ts.Samples, "Timeseries test_counter_total must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_counter_total must contain a single sample")
						require.Equal(t, timestamp.FromTime(st), ts.Samples[0].StartTimestamp,
							"Sample for test_counter_total does not have ST")
					},
				},
				{
					Name:        "start_timestamp for histograms",
					Description: "Sample SHOULD have start timestamp for a histogram",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						ts, _ := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_histogram_count")
						require.NotEmpty(t, ts.Samples, "Timeseries test_histogram_count must contain samples")
						require.Len(t, ts.Samples, 1, "Timeseries test_histogram_count must contain a single sample")
						require.Equal(t, timestamp.FromTime(st), ts.Samples[0].StartTimestamp,
							"Sample for test_histogram_count does not have ST")
					},
				},
			},
		},
	}
}

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
	"github.com/prometheus/prometheus/model/timestamp"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

func exemplarsTests() []Test {
	timeNow := time.Now().Truncate(time.Millisecond)
	explicitExemplarTime := timeNow.Add(-5 * time.Second)
	explicitExemplarTS := timestamp.FromTime(explicitExemplarTime)

	return []Test{
		{
			Name:        "exemplars",
			Description: "Senders MUST send valid exemplars with sorted labels, values, and timestamps.",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: fmt.Sprintf(`# HELP test_counter Counter with exemplar.
# TYPE test_counter counter
test_counter_total 100 # {span_id="span789",trace_id="abc123def456",user_id="42"} 1.25 %s
# HELP request_duration_seconds Histogram with exemplars on buckets.
# TYPE request_duration_seconds histogram
request_duration_seconds_bucket{le="0.1"} 10 # {trace_id="trace_fast"} 0.05 %s
request_duration_seconds_bucket{le="0.5"} 25 # {span_id="span_med",trace_id="trace_med"} 0.32
request_duration_seconds_bucket{le="1.0"} 30
request_duration_seconds_bucket{le="+Inf"} 32
request_duration_seconds_sum 14.5
request_duration_seconds_count 32
`,
				ToOpenMetricsTimestampString(explicitExemplarTime),
				ToOpenMetricsTimestampString(explicitExemplarTime),
			),
			Validate: func(t *testing.T, res ReceiverResult) {
				require.Len(t, res.Requests, 1)
				require.NotNil(t, res.Requests[0].RW2)
				require.NotEmpty(t, res.Requests[0].RW2.Timeseries)
			},
			ValidateCases: []ValidateCase{
				{
					Name:        "counter_exemplar_value_and_timestamp",
					Description: "Exemplars on counters MUST preserve value and explicit timestamp.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						series := flattenTimeseriesResult(t, requireTimeseriesByMetricName(t, req, "test_counter_total"))
						require.NotEmpty(t, series.TimeSeries.Exemplars, "Counter series must include exemplar")
						ex := series.TimeSeries.Exemplars[0]
						require.InDelta(t, 1.25, ex.Value, 1e-6)
						require.Equal(t, explicitExemplarTS, ex.Timestamp)
					},
				},
				{
					Name:        "histogram_bucket_exemplars",
					Description: "Exemplars on classic histogram buckets MUST preserve value and timestamp.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						bucketResults := requireTimeseriesByMetricName(t, req, "request_duration_seconds_bucket")
						var fastEx, medEx *writev2.Exemplar
						for _, r := range bucketResults {
							if len(r.TimeSeries.Exemplars) == 0 {
								continue
							}
							switch r.Labels["le"] {
							case "0.1":
								ex := r.TimeSeries.Exemplars[0]
								fastEx = &ex
							case "0.5":
								ex := r.TimeSeries.Exemplars[0]
								medEx = &ex
							}
						}
						require.NotNil(t, fastEx, "Exemplar on le=0.1 bucket must be present")
						require.InDelta(t, 0.05, fastEx.Value, 1e-6)
						require.Equal(t, explicitExemplarTS, fastEx.Timestamp)

						require.NotNil(t, medEx, "Exemplar on le=0.5 bucket must be present")
						require.InDelta(t, 0.32, medEx.Value, 1e-6)
						require.GreaterOrEqual(t, medEx.Timestamp, timestamp.FromTime(timeNow), "Exemplar without explicit timestamp must receive scrape timestamp")
					},
				},
				{
					Name:        "exemplar_labels_valid_and_sorted",
					Description: "Exemplar labels_refs MUST have even length, valid symbol indices, non-empty keys/values, and lexicographically sorted label names.",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						foundExemplar := false
						for _, ts := range req.Timeseries {
							for _, ex := range ts.Exemplars {
								foundExemplar = true
								require.NotEmpty(t, ex.LabelsRefs, "Exemplar labels_refs must not be empty")
								require.Zero(t, len(ex.LabelsRefs)%2, "Exemplar labels_refs length must be a multiple of 2")
								for _, ref := range ex.LabelsRefs {
									require.Less(t, int(ref), len(req.Symbols), "Exemplar label symbol reference out of bounds")
									require.NotEmpty(t, req.Symbols[ref], "Exemplar label name and value must not be empty")
								}
								require.True(t, isSorted(req.Symbols, ex.LabelsRefs), "Exemplar label names must be sorted lexicographically")
							}
						}
						require.True(t, foundExemplar, "At least one exemplar must be present in the request")
					},
				},
				{
					Name:        "exemplar_trace_id_present",
					Description: "Exemplars SHOULD contain trace_id label at minimum.",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						series := flattenTimeseriesResult(t, requireTimeseriesByMetricName(t, req, "test_counter_total"))
						require.NotEmpty(t, series.TimeSeries.Exemplars)
						lbls := extractExemplarLabels(&series.TimeSeries.Exemplars[0], req.Symbols)
						require.Equal(t, "abc123def456", lbls["trace_id"])
					},
				},
				{
					Name:        "exemplar_additional_labels",
					Description: "Exemplars MAY contain span_id and additional custom labels.",
					RFCLevel:    MayLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						req := res.Requests[0].RW2
						series := flattenTimeseriesResult(t, requireTimeseriesByMetricName(t, req, "test_counter_total"))
						require.NotEmpty(t, series.TimeSeries.Exemplars)
						lbls := extractExemplarLabels(&series.TimeSeries.Exemplars[0], req.Symbols)
						require.Equal(t, "span789", lbls["span_id"])
						require.Equal(t, "42", lbls["user_id"])
					},
				},
			},
		},
	}
}

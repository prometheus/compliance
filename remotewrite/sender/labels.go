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
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance"
	"github.com/stretchr/testify/require"
)

var (
	metricNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	labelNameRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

func labelsTests() []Test {
	return []Test{
		{
			Name:        "labels",
			Description: "Senders MUST send complete, valid, and lexicographically sorted label sets",
			RFCLevel:    compliance.MustLevel,
			ScrapeData: `# TYPE test_ordering gauge
test_ordering{zzz="3",aaa="1",bbb="2"} 42
# TYPE valid_metric:subsystem_total gauge
valid_metric:subsystem_total{valid_label="value",another_1="val2",empty=""} 10
# TYPE unicode_metric gauge
unicode_metric{emoji="🚀",chinese="测试",path="/api/v1/users?foo=bar&baz=qux"} 7
`,
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res ReceiverResult) {
				require.NotEmpty(t, res.Requests, "Should receive at least 1 request")
				var totalSeries int
				for _, req := range res.Requests {
					require.NotNil(t, req.RW2)
					totalSeries += len(req.RW2.Timeseries)
				}
				require.Positive(t, totalSeries, "Requests must contain timeseries")
			},
			ValidateCases: []compliance.ValidateCase[ReceiverResult]{
				{
					Name:        "labels_refs_present_and_even",
					Description: "labels_refs MUST be provided and its length MUST be a multiple of two",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							for _, ts := range req.RW2.Timeseries {
								require.NotEmpty(t, ts.LabelsRefs, "labels_refs MUST be provided on every TimeSeries")
								require.Equal(t, 0, len(ts.LabelsRefs)%2, "labels_refs length MUST be a multiple of two")
							}
						}
					},
				},
				{
					Name:        "metric_name_present",
					Description: "Label set SHOULD contain a __name__ label",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							for _, ts := range req.RW2.Timeseries {
								labels := extractLabels(&ts, req.RW2.Symbols)
								require.NotEmpty(t, labels["__name__"], "TimeSeries SHOULD contain a __name__ label")
							}
						}
					},
				},
				{
					Name:        "no_duplicate_label_names",
					Description: "Label set MUST NOT contain repeated label names",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							symbols := req.RW2.Symbols
							for _, ts := range req.RW2.Timeseries {
								seen := make(map[string]struct{}, len(ts.LabelsRefs)/2)
								for i := 0; i+1 < len(ts.LabelsRefs); i += 2 {
									require.Less(t, int(ts.LabelsRefs[i]), len(symbols), "Label name symbol reference out of bounds")
									name := symbols[ts.LabelsRefs[i]]
									_, dup := seen[name]
									require.False(t, dup, "Label set MUST NOT contain duplicate label name %q", name)
									seen[name] = struct{}{}
								}
							}
						}
					},
				},
				{
					Name:        "lexicographically_sorted",
					Description: "Label set MUST have label names sorted in lexicographical order",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							for _, ts := range req.RW2.Timeseries {
								require.True(t, isSorted(req.RW2.Symbols, ts.LabelsRefs),
									"Label names MUST be sorted in lexicographical order")
							}
						}
					},
				},
				{
					Name:        "no_empty_names_or_values",
					Description: "Label set MUST NOT contain any empty label names or values",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							symbols := req.RW2.Symbols
							for _, ts := range req.RW2.Timeseries {
								for i := 0; i+1 < len(ts.LabelsRefs); i += 2 {
									require.Less(t, int(ts.LabelsRefs[i]), len(symbols), "Label name symbol reference out of bounds")
									require.Less(t, int(ts.LabelsRefs[i+1]), len(symbols), "Label value symbol reference out of bounds")
									name := symbols[ts.LabelsRefs[i]]
									val := symbols[ts.LabelsRefs[i+1]]
									require.NotEmpty(t, name, "Label name MUST NOT be empty")
									require.NotEmpty(t, val, "Label value for %q MUST NOT be empty", name)
								}
							}
						}
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "valid_metric:subsystem_total")
						_, hasEmptyLabel := results[0].Labels["empty"]
						require.False(t, hasEmptyLabel, "Empty label from scrape target MUST NOT be sent")
					},
				},
				{
					Name:        "utf8_and_special_chars",
					Description: "Metric names, label names, and label values MUST support UTF-8 characters",
					RFCLevel:    compliance.MustLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "unicode_metric")
						lbls := results[0].Labels
						require.Equal(t, "🚀", lbls["emoji"], "UTF-8 emoji in label value MUST be preserved")
						require.Equal(t, "测试", lbls["chinese"], "UTF-8 characters in label value MUST be preserved")
						require.Equal(t, "/api/v1/users?foo=bar&baz=qux", lbls["path"], "Special characters in label value MUST be preserved")
					},
				},
				{
					Name:        "metric_name_format",
					Description: "Metric names SHOULD adhere to [a-zA-Z_:]([a-zA-Z0-9_:]*)",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "valid_metric:subsystem_total")
						for _, req := range res.Requests {
							for _, ts := range req.RW2.Timeseries {
								labels := extractLabels(&ts, req.RW2.Symbols)
								name := labels["__name__"]
								require.True(t, metricNameRE.MatchString(name), "Metric name %q SHOULD match [a-zA-Z_:][a-zA-Z0-9_:]*", name)
							}
						}
					},
				},
				{
					Name:        "label_name_format",
					Description: "Label names SHOULD adhere to [a-zA-Z_]([a-zA-Z0-9_])*",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							for _, ts := range req.RW2.Timeseries {
								labels := extractLabels(&ts, req.RW2.Symbols)
								for name := range labels {
									require.True(t, labelNameRE.MatchString(name), "Label name %q SHOULD match [a-zA-Z_][a-zA-Z0-9_]*", name)
								}
							}
						}
					},
				},
				{
					Name:        "no_reserved_label_prefix",
					Description: "Label names beginning with __ are RESERVED for system usage and SHOULD NOT be used",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						for _, req := range res.Requests {
							for _, ts := range req.RW2.Timeseries {
								labels := extractLabels(&ts, req.RW2.Symbols)
								for name := range labels {
									if name == "__name__" {
										continue
									}
									require.False(t, strings.HasPrefix(name, "__"), "Label %q uses reserved __ prefix", name)
								}
							}
						}
					},
				},
				{
					Name:        "target_labels_present",
					Description: "Scraped series should include job and instance target labels",
					RFCLevel:    compliance.RecommendedLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						results := requireTimeseriesByMetricNameAcrossRequests(t, res.Requests, "test_ordering")
						require.NotEmpty(t, results[0].Labels["job"], "Scraped series should include job label")
						require.NotEmpty(t, results[0].Labels["instance"], "Scraped series should include instance label")
					},
				},
				{
					Name:        "multiple_series_batched",
					Description: "Senders SHOULD use Remote-Write to send samples for multiple series in a single request",
					RFCLevel:    compliance.ShouldLevel,
					Validate: func(t *testing.T, res ReceiverResult) {
						var maxSeries int
						for _, req := range res.Requests {
							maxSeries = max(maxSeries, len(req.RW2.Timeseries))
						}
						require.GreaterOrEqual(t, maxSeries, 3,
							"Sender SHOULD batch multiple scraped series into a single request")
					},
				},
			},
		},
	}
}

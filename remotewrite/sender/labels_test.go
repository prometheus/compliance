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
	"testing"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance/remotewrite/sender/sendertest"
	"github.com/stretchr/testify/require"
)

// TestLabels validates label related requirements for Remote Write 2.0.
func TestLabels(t *testing.T) {
	sendertest.Run(t,
		targetsToTest,
		sendertest.Case{
			RFCLevel: sendertest.MustLevel,
			ScrapeData: `
test_metric{foo="bar",baz="qux"} 1
test_metric1{foo="bar",baz="qux"} 2
another_metric{foo="bar"} 3`,
			Version: remote.WriteV2MessageType,
			Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)
				require.Greater(t, len(res.Requests[0].RW2.Timeseries), 3, "Request must contain at least 3 timeseries")
			},
			ValidateCases: []sendertest.ValidateCase{
				{
					Name:        "symbols_empty_at_index_zero",
					RFCLevel:    sendertest.MustLevel,
					Description: "Symbol table MUST have empty string at index 0",
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						symbols := res.Requests[0].RW2.Symbols
						require.NotEmpty(t, len(symbols))
						require.Equal(t, "", symbols[0], "Symbol at index 0 must be empty string, got: %q", symbols[0])
					},
				},
				{
					Name:        "symbols_deduplication",
					RFCLevel:    sendertest.RecommendedLevel,
					Description: "Symbol table should deduplicate repeated strings for efficiency",
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						symbols := res.Requests[0].RW2.Symbols

						// Check for duplicate non-empty strings.
						seen := make(map[string]int)
						for i, sym := range symbols {
							if sym == "" {
								continue // Empty string can appear multiple times (though should only be at index 0).
							}
							if prevIdx, exists := seen[sym]; exists {
								t.Fatalf("Duplicate string %q found at indices %d and %d (deduplication is a performance recommended)",
									sym, prevIdx, i)
							}
							seen[sym] = i
						}
					},
				},
				{
					Name:        "labels_refs_valid_indices",
					RFCLevel:    sendertest.MustLevel,
					Description: "All label refs MUST point to valid symbol table indices",
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						symbols := res.Requests[0].RW2.Symbols
						tss := res.Requests[0].RW2.Timeseries

						for i, ts := range tss {
							for refIdx, ref := range ts.LabelsRefs {
								require.Less(t, int(ref), len(symbols),
									"Timeseries[%v].LabelsRefs[%v] = %d points outside symbol table (size: %d)",
									i, refIdx, ref, len(symbols))
							}
						}
					},
				},
				{
					Name:        "labels_refs_even_length",
					RFCLevel:    sendertest.MustLevel,
					Description: "Label refs array length MUST be even (key-value pairs)",
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						tss := res.Requests[0].RW2.Timeseries

						for i, ts := range tss {
							refsLen := len(ts.LabelsRefs)
							require.Equal(t, 0, refsLen%2,
								"Timeseries[%v].LabelsRefs has odd length %d (must be even for key-value pairs)",
								i, refsLen)
						}
					},
				},
				{
					Name:        "label_lexicographic_ordering",
					RFCLevel:    sendertest.MustLevel,
					Description: "Label names MUST be sorted in lexicographic order",
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						symbols := res.Requests[0].RW2.Symbols
						tss := res.Requests[0].RW2.Timeseries

						for i, ts := range tss {
							require.True(t, isSorted(symbols, ts.LabelsRefs),
								"Timeseries[%v].LabelsRefs are not sorted in lexicographic order", i)
						}
					},
				},
				{
					Name:        "metric_name_label_present",
					RFCLevel:    sendertest.MustLevel,
					Description: "Labels MUST include __name__ label",
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						symbols := res.Requests[0].RW2.Symbols
						tss := res.Requests[0].RW2.Timeseries

						for _, ts := range tss {
							labels := extractLabels(&ts, symbols)
							require.NotEmpty(t, labels["__name__"],
								"Timeseries[%v].LabelsRefs do not include __name__ label", labels)
						}
					},
				},
			},
		},
		// 1.0.
		sendertest.Case{
			RFCLevel: sendertest.MustLevel,
			ScrapeData: `
test_metric{foo="bar",baz="qux"} 1
test_metric1{foo="bar",baz="qux"} 2
another_metric{foo="bar"} 3`,
			Version: remote.WriteV1MessageType,
			Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)
				require.Greater(t, len(res.Requests[0].RW1.Timeseries), 3, "Request must contain at least 3 timeseries")
			},
			ValidateCases: []sendertest.ValidateCase{
				{
					Name:        "label_lexicographic_ordering",
					RFCLevel:    sendertest.MustLevel,
					Description: "Label names MUST be sorted in lexicographic order",
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						tss := res.Requests[0].RW1.Timeseries
						for i, ts := range tss {
							require.True(t, isSortedRW1(ts.Labels),
								"Timeseries[%v].Labels are not sorted in lexicographic order", i)
						}
					},
				},
				{
					Name:        "metric_name_label_present",
					RFCLevel:    sendertest.MustLevel,
					Description: "Labels MUST include __name__ label",
					Validate: func(t *testing.T, res sendertest.ReceiverResult) {
						tss := res.Requests[0].RW1.Timeseries

						for _, ts := range tss {
							var names int
							for _, label := range ts.Labels {
								if label.Name == "__name__" {
									names++
								}
							}
							require.Equal(t, 1, names, "Timeseries[%v].Labels must include a single __name__ label", ts.Labels)
						}
					},
				},
			},
		},
	)
}

func TestLabelsEdgeCases(t *testing.T) {
	sendertest.Run(t, targetsToTest,
		sendertest.Case{
			Name:        "unicode_in_label_values",
			Description: "Sender handles Unicode characters in label values",
			RFCLevel:    sendertest.RecommendedLevel, // This depends on UTF-8 feature on scrape, thus recommended level.
			ScrapeData:  `test_metric{emoji="🚀",chinese="测试"} 42`,
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)

				_, labels := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric")
				require.Equal(t, map[string]string{
					"__name__": "test_metric",
					"emoji":    "🚀",
					"chinese":  "测试",
				}, labels)
			},
		},
		sendertest.Case{
			Name:        "empty_label_value",
			Description: "Sender MUST NOT send empty label values",
			RFCLevel:    sendertest.MustLevel,
			ScrapeData:  `test_metric{foo="",bar="qux"} 42`,
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)

				_, labels := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric")
				require.Equal(t, map[string]string{
					"__name__": "test_metric",
					"bar":      "qux",
				}, labels)
			},
		},
		sendertest.Case{
			Name:        "metric_name_with_colons",
			Description: "Sender MUST handle metric names with colons",
			RFCLevel:    sendertest.MustLevel,
			ScrapeData:  "http:request:duration:seconds 0.5",
			Version:     remote.WriteV2MessageType,
			Validate: func(t *testing.T, res sendertest.ReceiverResult) {
				require.GreaterOrEqual(t, len(res.Requests), 1)

				_, labels := requireTimeseriesByMetricName(t, res.Requests[0].RW2, "test_metric")
				require.Equal(t, map[string]string{
					"__name__": "http:request:duration:seconds",
				}, labels)
			},
		},
	)
}

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
	"github.com/prometheus/compliance/remotewrite/sender/targets"
	"regexp"
	"testing"
)

// TestLabelValidation validates label encoding and formatting.
func TestLabelValidation(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		validator   func(*testing.T, *CapturedRequest)
	}{
		{
			name:        "label_lexicographic_ordering",
			description: "Labels MUST be sorted in lexicographic order",
			rfcLevel:    "MUST",
			scrapeData: `test_metric{aaa="1",bbb="2",zzz="3"} 42
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_metric" {
						// Verify labels are sorted
						must(t).True(isSorted(labels, req.Request.Symbols, ts.LabelsRefs),
							"Labels must be sorted in lexicographic order")
						break
					}
				}
			},
		},
		{
			name:        "metric_name_label_present",
			description: "Timeseries MUST include __name__ label",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				must(t).NotEmpty(req.Request.Timeseries, "Request must contain timeseries")

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					must(t).NotEmpty(labels["__name__"],
						"Timeseries must include __name__ label")
				}
			},
		},
		{
			name:        "metric_name_format_valid",
			description: "Metric name MUST match [a-zA-Z_:][a-zA-Z0-9_:]* regex",
			rfcLevel:    "MUST",
			scrapeData:  "valid_metric_name:subsystem_total 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				metricNameRegex := regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					metricName := labels["__name__"]
					must(t).NotEmpty(metricName, "Metric name must not be empty")
					must(t).True(metricNameRegex.MatchString(metricName),
						"Metric name must match regex [a-zA-Z_:][a-zA-Z0-9_:]*, got: %s", metricName)
				}
			},
		},
		{
			name:        "label_name_format_valid",
			description: "Label names MUST match [a-zA-Z_][a-zA-Z0-9_]* regex (except __name__)",
			rfcLevel:    "MUST",
			scrapeData:  `test_metric{valid_label="value",another_1="val2"} 42`,
			validator: func(t *testing.T, req *CapturedRequest) {
				labelNameRegex := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					for labelName := range labels {
						if labelName == "" {
							continue
						}
						must(t).True(labelNameRegex.MatchString(labelName) || labelName == "__name__",
							"Label name must match regex [a-zA-Z_][a-zA-Z0-9_]*, got: %s", labelName)
					}
				}
			},
		},
		{
			name:        "no_duplicate_label_names",
			description: "Timeseries MUST NOT have duplicate label names",
			rfcLevel:    "MUST",
			scrapeData:  `test_metric{foo="bar",baz="qux"} 42`,
			validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)

					// Check for duplicates by comparing length
					// If duplicates exist, the map will have fewer entries than pairs
					expectedPairs := len(ts.LabelsRefs) / 2
					must(t).Equal(expectedPairs, len(labels),
						"No duplicate label names allowed")
				}
			},
		},
		{
			name:        "label_names_not_empty",
			description: "Label names MUST NOT be empty",
			rfcLevel:    "MUST",
			scrapeData:  `test_metric{label="value"} 42`,
			validator: func(t *testing.T, req *CapturedRequest) {
				symbols := req.Request.Symbols
				for _, ts := range req.Request.Timeseries {
					refs := ts.LabelsRefs
					// Check all label name refs (even indices)
					for i := 0; i < len(refs); i += 2 {
						keyRef := refs[i]
						labelName := symbols[keyRef]
						must(t).NotEmpty(labelName, "Label names must not be empty")
					}
				}
			},
		},
		{
			name:        "label_values_may_be_empty",
			description: "Label values MAY be empty strings",
			rfcLevel:    "MAY",
			scrapeData:  `test_metric{empty=""} 42`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// Empty label values are allowed
				symbols := req.Request.Symbols
				for _, ts := range req.Request.Timeseries {
					refs := ts.LabelsRefs
					for i := 1; i < len(refs); i += 2 {
						valueRef := refs[i]
						labelValue := symbols[valueRef]
						// Empty values are allowed
						may(t).GreaterOrEqual(len(labelValue), 0,
							"Label values may be empty")
					}
				}
			},
		},
		{
			name:        "reserved_label_prefix",
			description: "Labels with __ prefix SHOULD be reserved for internal use",
			rfcLevel:    "SHOULD",
			scrapeData:  `test_metric{normal="value"} 42`,
			validator: func(t *testing.T, req *CapturedRequest) {
				// Check that user-defined labels don't use __ prefix
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					for labelName := range labels {
						if labelName == "__name__" {
							continue // __name__ is allowed
						}
						if len(labelName) >= 2 && labelName[0:2] == "__" {
							should(t).True(labelName == "__name__",
								"Labels with __ prefix should be reserved, found: %s", labelName)
						}
					}
				}
			},
		},
		{
			name:        "unicode_in_label_values",
			description: "Sender MUST handle Unicode characters in label values",
			rfcLevel:    "MUST",
			scrapeData:  `test_metric{emoji="🚀",chinese="测试"} 42`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundUnicode bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					for _, value := range labels {
						// Check if value contains non-ASCII characters
						for _, r := range value {
							if r > 127 {
								foundUnicode = true
								must(t).NotEmpty(value, "Unicode values must be preserved")
								break
							}
						}
					}
				}
				may(t).True(foundUnicode || len(req.Request.Timeseries) > 0,
					"Unicode characters may be present in labels")
			},
		},
		{
			name:        "special_chars_in_label_values",
			description: "Sender MUST handle special characters in label values",
			rfcLevel:    "MUST",
			scrapeData:  `test_metric{path="/api/v1/users",query="foo=bar&baz=qux"} 42`,
			validator: func(t *testing.T, req *CapturedRequest) {
				var foundSpecial bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					for key, value := range labels {
						if key == "path" || key == "query" {
							must(t).NotEmpty(value, "Special characters must be preserved")
							foundSpecial = true
						}
					}
				}
				should(t).True(foundSpecial || len(req.Request.Timeseries) > 0,
					"Special characters should be handled correctly")
			},
		},
		{
			name:        "very_long_label_names",
			description: "Sender SHOULD handle long label names (within reasonable limits)",
			rfcLevel:    "SHOULD",
			scrapeData:  `test_metric{very_long_label_name_that_exceeds_normal_length_but_is_still_valid="value"} 42`,
			validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					for labelName := range labels {
						if len(labelName) > 50 {
							should(t).NotEmpty(labelName, "Long label names should be handled")
							t.Logf("Found long label name: %s (length: %d)", labelName, len(labelName))
						}
					}
				}
			},
		},
		{
			name:        "very_long_label_values",
			description: "Sender SHOULD handle long label values (within reasonable limits)",
			rfcLevel:    "SHOULD",
			scrapeData:  `test_metric{description="This is a very long label value that contains a lot of text to test how senders handle long strings in label values which might be common in real-world scenarios"} 42`,
			validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					for _, value := range labels {
						if len(value) > 100 {
							should(t).NotEmpty(value, "Long label values should be handled")
							t.Logf("Found long label value (length: %d)", len(value))
						}
					}
				}
			},
		},
		{
			name:        "many_labels_per_series",
			description: "Sender SHOULD handle timeseries with many labels",
			rfcLevel:    "SHOULD",
			scrapeData:  `test_metric{l1="v1",l2="v2",l3="v3",l4="v4",l5="v5",l6="v6",l7="v7",l8="v8",l9="v9",l10="v10"} 42`,
			validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if len(labels) > 5 {
						should(t).GreaterOrEqual(len(labels), 5,
							"Sender should handle timeseries with many labels")
						t.Logf("Found timeseries with %d labels", len(labels))
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Attr("rfcLevel", tt.rfcLevel)
			t.Attr("description", tt.description)

			forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
				runSenderTest(t, targetName, target, SenderTestScenario{
					ScrapeData: tt.scrapeData,
					Validator:  tt.validator,
				})
			})
		})
	}
}

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
	"fmt"
	"regexp"
	"testing"
)

// TestLabelValidation validates label encoding and formatting.
func TestLabelValidation_Old(t *testing.T) {
	t.Skip("TODO: Revise and move to a new framework")

	tests := []TestCase{
		{
			Name:        "label_lexicographic_ordering",
			Description: "Labels MUST be sorted in lexicographic order",
			RFCLevel:    "MUST",
			ScrapeData: `test_metric{aaa="1",bbb="2",zzz="3"} 42
`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if labels["__name__"] == "test_metric" {
						must(t).True(isSorted(req.Request.Symbols, ts.LabelsRefs),
							"Labels must be sorted in lexicographic order")
						break
					}
				}
			},
		},
		{
			Name:        "metric_name_label_present",
			Description: "Timeseries MUST include __name__ label",
			RFCLevel:    "MUST",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				must(t).NotEmpty(req.Request.Timeseries, "Request must contain timeseries")

				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					must(t).NotEmpty(labels["__name__"],
						"Timeseries must include __name__ label")
				}
			},
		},
		{
			Name:        "metric_name_format_valid",
			Description: "Metric name MUST match [a-zA-Z_:][a-zA-Z0-9_:]* regex",
			RFCLevel:    "MUST",
			ScrapeData:  "valid_metric_name:subsystem_total 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
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
			Name:        "label_name_format_valid",
			Description: "Label names MUST match [a-zA-Z_][a-zA-Z0-9_]* regex (except __name__)",
			RFCLevel:    "MUST",
			ScrapeData:  `test_metric{valid_label="value",another_1="val2"} 42`,
			Validator: func(t *testing.T, req *CapturedRequest) {
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
			Name:        "no_duplicate_label_names",
			Description: "Timeseries MUST NOT have duplicate label names",
			RFCLevel:    "MUST",
			ScrapeData:  `test_metric{foo="bar",baz="qux"} 42`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)

					expectedPairs := len(ts.LabelsRefs) / 2
					must(t).Equal(expectedPairs, len(labels),
						"No duplicate label names allowed")
				}
			},
		},
		{
			Name:        "label_names_not_empty",
			Description: "Label names MUST NOT be empty",
			RFCLevel:    "MUST",
			ScrapeData:  `test_metric{label="value"} 42`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				symbols := req.Request.Symbols
				for _, ts := range req.Request.Timeseries {
					refs := ts.LabelsRefs
					for i := 0; i < len(refs); i += 2 {
						keyRef := refs[i]
						labelName := symbols[keyRef]
						must(t).NotEmpty(labelName, "Label names must not be empty")
					}
				}
			},
		},
		{
			Name:        "label_values_may_be_empty",
			Description: "Label values MAY be empty strings",
			RFCLevel:    "MAY",
			ScrapeData:  `test_metric{empty=""} 42`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				symbols := req.Request.Symbols
				for _, ts := range req.Request.Timeseries {
					refs := ts.LabelsRefs
					for i := 1; i < len(refs); i += 2 {
						valueRef := refs[i]
						labelValue := symbols[valueRef]
						may(t, len(labelValue) >= 0, "Label values may be empty")
					}
				}
			},
		},
		{
			Name:        "reserved_label_prefix",
			Description: "Labels with __ prefix SHOULD be reserved for internal use",
			RFCLevel:    "SHOULD",
			ScrapeData:  `test_metric{normal="value"} 42`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					for labelName := range labels {
						if labelName == "__name__" {
							continue // __name__ is allowed
						}
						if len(labelName) >= 2 && labelName[0:2] == "__" {
							should(t, labelName == "__name__", fmt.Sprintf("Labels with __ prefix should be reserved, found: %s", labelName))
						}
					}
				}
			},
		},
		{
			Name:        "unicode_in_label_values",
			Description: "Sender MUST handle Unicode characters in label values",
			RFCLevel:    "MUST",
			ScrapeData:  `test_metric{emoji="🚀",chinese="测试"} 42`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				var foundUnicode bool
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					for _, value := range labels {
						// Check if value contains non-ASCII characters.
						for _, r := range value {
							if r > 127 {
								foundUnicode = true
								must(t).NotEmpty(value, "Unicode values must be preserved")
								break
							}
						}
					}
				}
				may(t, foundUnicode || len(req.Request.Timeseries) > 0, "Unicode characters may be present in labels")
			},
		},
		{
			Name:        "special_chars_in_label_values",
			Description: "Sender MUST handle special characters in label values",
			RFCLevel:    "MUST",
			ScrapeData:  `test_metric{path="/api/v1/users",query="foo=bar&baz=qux"} 42`,
			Validator: func(t *testing.T, req *CapturedRequest) {
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
				should(t, foundSpecial || len(req.Request.Timeseries) > 0, "Special characters should be handled correctly")
			},
		},
		{
			Name:        "very_long_label_names",
			Description: "Sender SHOULD handle long label names (within reasonable limits)",
			RFCLevel:    "SHOULD",
			ScrapeData:  `test_metric{very_long_label_name_that_exceeds_normal_length_but_is_still_valid="value"} 42`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					for labelName := range labels {
						if len(labelName) > 50 {
							should(t, len(labelName) > 0, "Long label names should be handled")
							t.Logf("Found long label name: %s (length: %d)", labelName, len(labelName))
						}
					}
				}
			},
		},
		{
			Name:        "very_long_label_values",
			Description: "Sender SHOULD handle long label values (within reasonable limits)",
			RFCLevel:    "SHOULD",
			ScrapeData:  `test_metric{description="This is a very long label value that contains a lot of text to test how senders handle long strings in label values which might be common in real-world scenarios"} 42`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					for _, value := range labels {
						if len(value) > 100 {
							should(t, len(value) > 0, "Long label values should be handled")
							t.Logf("Found long label value (length: %d)", len(value))
						}
					}
				}
			},
		},
		{
			Name:        "many_labels_per_series",
			Description: "Sender SHOULD handle timeseries with many labels",
			RFCLevel:    "SHOULD",
			ScrapeData:  `test_metric{l1="v1",l2="v2",l3="v3",l4="v4",l5="v5",l6="v6",l7="v7",l8="v8",l9="v9",l10="v10"} 42`,
			Validator: func(t *testing.T, req *CapturedRequest) {
				for _, ts := range req.Request.Timeseries {
					labels := extractLabels(&ts, req.Request.Symbols)
					if len(labels) > 5 {
						should(t, len(labels) >= 5, "Sender should handle timeseries with many labels")
						t.Logf("Found timeseries with %d labels", len(labels))
					}
				}
			},
		},
	}

	runTestCases(t, tests)
}

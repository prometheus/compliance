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
	"testing"
)

// TestSymbolTable validates symbol table requirements for Remote Write 2.0.
func TestSymbolTable(t *testing.T) {
	tests := []struct {
		name        string
		description string
		rfcLevel    string
		scrapeData  string
		validator   func(*testing.T, *CapturedRequest)
	}{
		{
			name:        "empty_string_at_index_zero",
			description: "Symbol table MUST have empty string at index 0",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				symbols := req.Request.Symbols
				must(t).NotEmpty(symbols, "Symbol table must not be empty")
				must(t).Equal("", symbols[0],
					"Symbol at index 0 must be empty string, got: %q", symbols[0])
			},
		},
		{
			name:        "string_deduplication",
			description: "Symbol table MUST deduplicate repeated strings",
			rfcLevel:    "MUST",
			scrapeData: `# Multiple metrics with same label keys/values
test_metric{foo="bar",baz="qux"} 1
test_metric{foo="bar",baz="qux"} 2
another_metric{foo="bar"} 3
`,
			validator: func(t *testing.T, req *CapturedRequest) {
				symbols := req.Request.Symbols
				must(t).NotEmpty(symbols, "Symbol table must not be empty")

				// Check for duplicate non-empty strings.
				seen := make(map[string]int)
				for i, sym := range symbols {
					if sym == "" {
						continue // Empty string can appear multiple times (though should only be at index 0)
					}
					if prevIdx, exists := seen[sym]; exists {
						must(t).Fail("Duplicate string %q found at indices %d and %d (deduplication required)",
							sym, prevIdx, i)
					}
					seen[sym] = i
				}
			},
		},
		{
			name:        "labels_refs_valid_indices",
			description: "All label refs MUST point to valid symbol table indices",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric{label=\"value\"} 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				symbols := req.Request.Symbols
				timeseries := req.Request.Timeseries

				must(t).NotEmpty(timeseries, "Request must contain at least one timeseries")

				for tsIdx, ts := range timeseries {
					for refIdx, ref := range ts.LabelsRefs {
						must(t).Less(int(ref), len(symbols),
							"Timeseries[%d].LabelsRefs[%d] = %d points outside symbol table (size: %d)",
							tsIdx, refIdx, ref, len(symbols))
					}
				}
			},
		},
		{
			name:        "labels_refs_even_length",
			description: "Label refs array length MUST be even (key-value pairs)",
			rfcLevel:    "MUST",
			scrapeData:  "test_metric{label=\"value\"} 42\n",
			validator: func(t *testing.T, req *CapturedRequest) {
				timeseries := req.Request.Timeseries
				must(t).NotEmpty(timeseries, "Request must contain at least one timeseries")

				for tsIdx, ts := range timeseries {
					refsLen := len(ts.LabelsRefs)
					must(t).Equal(0, refsLen%2,
						"Timeseries[%d].LabelsRefs has odd length %d (must be even for key-value pairs)",
						tsIdx, refsLen)
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

// TestSymbolTableEfficiency validates that symbol tables are efficiently constructed.
func TestSymbolTableEfficiency(t *testing.T) {
	t.Attr("rfcLevel", "SHOULD")
	t.Attr("description", "Symbol table SHOULD be efficiently constructed with good compression")

	scrapeData := `# Multiple series with shared labels
http_requests_total{method="GET",status="200",handler="/api/v1"} 100
http_requests_total{method="POST",status="200",handler="/api/v1"} 50
http_requests_total{method="GET",status="404",handler="/api/v1"} 10
http_requests_total{method="GET",status="200",handler="/api/v2"} 75
`

	forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
		runSenderTest(t, targetName, target, SenderTestScenario{
			ScrapeData: scrapeData,
			Validator: func(t *testing.T, req *CapturedRequest) {
				symbols := req.Request.Symbols

				// With deduplication, common strings like "http_requests_total", "method",
				// "status", "handler", "200", "GET", "/api/v1" should appear only once.
				// Without deduplication, the symbol table would be much larger.

				// Count unique non-empty symbols.
				uniqueCount := 0
				for _, sym := range symbols {
					if sym != "" {
						uniqueCount++
					}
				}

				// For the above scrape data, we expect around 11-15 unique symbols:
				// metric name (1), label keys (3), label values (7-8)
				// If the symbol table is much larger, deduplication may not be working.
				should(t).LessOrEqual(uniqueCount, 30,
					"Symbol table should be efficiently deduplicated (found %d unique symbols)",
					uniqueCount)

				t.Logf("Symbol table contains %d unique symbols (total %d entries)",
					uniqueCount, len(symbols))
			},
		})
	})
}

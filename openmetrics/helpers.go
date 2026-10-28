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

package openmetrics

import (
	"strings"
	"testing"
	"unicode/utf8"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
)

// findFamily returns the first decoded MetricFamily matching one of the candidate names, or nil.
func findFamily(res ScrapeResult, names ...string) *dto.MetricFamily {
	for _, n := range names {
		if mf, ok := res.Families[n]; ok {
			return mf
		}
	}
	return nil
}

// requireFamily asserts that at least one of the candidate MetricFamily names exists in res.
func requireFamily(t *testing.T, res ScrapeResult, names ...string) *dto.MetricFamily {
	t.Helper()
	mf := findFamily(res, names...)
	require.NotNilf(t, mf, "expected one of metric families %v in scraped families (found %d families)", names, len(res.Families))
	return mf
}

// familiesByType returns all decoded MetricFamilies in wire order that have the given MetricType.
func familiesByType(res ScrapeResult, mt dto.MetricType) []*dto.MetricFamily {
	var out []*dto.MetricFamily
	for _, mf := range res.OrderedFamilies {
		if mf.GetType() == mt {
			out = append(out, mf)
		}
	}
	return out
}

// labelMap converts a slice of dto.LabelPair into a map of label name to value.
func labelMap(pairs []*dto.LabelPair) map[string]string {
	m := make(map[string]string, len(pairs))
	for _, lp := range pairs {
		m[lp.GetName()] = lp.GetValue()
	}
	return m
}

// findMetricWithLabels returns the first Metric in mf whose labels match all entries in want.
func findMetricWithLabels(mf *dto.MetricFamily, want map[string]string) *dto.Metric {
	if mf == nil {
		return nil
	}
	for _, m := range mf.Metric {
		got := labelMap(m.Label)
		matched := true
		for k, v := range want {
			if got[k] != v {
				matched = false
				break
			}
		}
		if matched {
			return m
		}
	}
	return nil
}

// exemplarLabelRunes returns the total number of UTF-8 runes across all label names and values in e.
func exemplarLabelRunes(e *dto.Exemplar) int {
	if e == nil {
		return 0
	}
	total := 0
	for _, lp := range e.Label {
		total += utf8.RuneCountInString(lp.GetName())
		total += utf8.RuneCountInString(lp.GetValue())
	}
	return total
}

// sampleLinesForFamily returns all sample lines in body belonging to familyName.
func sampleLinesForFamily(body []byte, familyName string) []string {
	var out []string
	quotedPrefix := `"` + familyName + `"`
	braceQuotedPrefix := `{"` + familyName + `"`
	for line := range strings.SplitSeq(string(body), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, familyName+" ") ||
			strings.HasPrefix(line, familyName+"{") ||
			strings.HasPrefix(line, familyName+"_") ||
			strings.HasPrefix(line, quotedPrefix) ||
			strings.HasPrefix(line, braceQuotedPrefix) {
			out = append(out, line)
		}
	}
	return out
}

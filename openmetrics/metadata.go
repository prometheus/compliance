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
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func metadataTests(v Version) []Test {
	unitSuffixLevel := MustLevel
	if v == Version2_0_0 || v == Version2_0_0RC {
		unitSuffixLevel = ShouldLevel
	}

	return []Test{
		{
			Name:        "metadata",
			Description: fmt.Sprintf("Exporters MUST emit valid # TYPE, # HELP, and # UNIT descriptors before samples in OpenMetrics %s", v),
			RFCLevel:    MustLevel,
			Version:     v,
			ValidateCases: []ValidateCase{
				{
					Name:        "descriptors_before_samples_and_unique",
					Description: "Descriptor lines (# TYPE, # HELP, # UNIT) MUST appear before any samples for a metric family and at most once per family",
					RFCLevel:    MustLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NoError(t, res.ParseErr, "ABNF and descriptor ordering validation MUST succeed")
						require.NotEmpty(t, res.OrderedFamilies)

						seenType := make(map[string]bool)
						seenHelp := make(map[string]bool)
						seenUnit := make(map[string]bool)
						for line := range strings.SplitSeq(string(res.Body), "\n") {
							switch {
							case strings.HasPrefix(line, "# TYPE "):
								rest := strings.TrimPrefix(line, "# TYPE ")
								name, _, ok := strings.Cut(rest, " ")
								require.True(t, ok, "Malformed # TYPE line: %q", line)
								require.False(t, seenType[name], "Duplicate # TYPE line for %q", name)
								seenType[name] = true
							case strings.HasPrefix(line, "# HELP "):
								rest := strings.TrimPrefix(line, "# HELP ")
								name, _, ok := strings.Cut(rest, " ")
								require.True(t, ok, "Malformed # HELP line: %q", line)
								require.False(t, seenHelp[name], "Duplicate # HELP line for %q", name)
								seenHelp[name] = true
							case strings.HasPrefix(line, "# UNIT "):
								rest := strings.TrimPrefix(line, "# UNIT ")
								name, _, ok := strings.Cut(rest, " ")
								require.True(t, ok, "Malformed # UNIT line: %q", line)
								require.False(t, seenUnit[name], "Duplicate # UNIT line for %q", name)
								seenUnit[name] = true
							}
						}
					},
				},
				{
					Name:        "type_descriptor_present",
					Description: "Metric families SHOULD include a # TYPE descriptor",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NotEmpty(t, res.OrderedFamilies)
						for _, mf := range res.OrderedFamilies {
							require.NotNilf(t, mf.Type, "MetricFamily %q SHOULD include a # TYPE descriptor", mf.GetName())
						}
					},
				},
				{
					Name:        "help_descriptor_present",
					Description: "Metric families SHOULD include a non-empty # HELP descriptor",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						require.NotEmpty(t, res.OrderedFamilies)
						for _, mf := range res.OrderedFamilies {
							require.NotEmptyf(t, mf.GetHelp(), "MetricFamily %q SHOULD include a non-empty # HELP descriptor", mf.GetName())
						}
					},
				},
				{
					Name:        "unit_descriptor_present",
					Description: "Metric families with units SHOULD include a # UNIT descriptor",
					RFCLevel:    ShouldLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						foundUnit := false
						for _, mf := range res.OrderedFamilies {
							if mf.GetUnit() != "" {
								foundUnit = true
								break
							}
						}
						require.True(t, foundUnit, "Exposition SHOULD include at least one metric family with a # UNIT descriptor")
					},
				},
				{
					Name:        "unit_suffix_matches_unit_descriptor",
					Description: "When # UNIT is specified, the metric family name MUST end with _<unit> in OpenMetrics 1.0 (SHOULD in OpenMetrics 2.0)",
					RFCLevel:    unitSuffixLevel,
					Validate: func(t *testing.T, res ScrapeResult) {
						for _, mf := range res.OrderedFamilies {
							unit := mf.GetUnit()
							if unit == "" {
								continue
							}
							baseName := strings.TrimSuffix(mf.GetName(), "_total")
							require.Truef(t, strings.HasSuffix(baseName, "_"+unit),
								"MetricFamily %q with unit %q must have base name ending with _%s", mf.GetName(), unit, unit)
						}
					},
				},
			},
		},
	}
}

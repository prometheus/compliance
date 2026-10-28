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

package config

import (
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

// Config models the PromQL compliance test queries configuration file.
type Config struct {
	QueryTweaks []*QueryTweak `yaml:"query_tweaks"`
	TestCases   []*TestCase   `yaml:"test_cases"`
}

// A QueryTweak restricts or modifies a query in certain ways that avoids certain systematic errors and/or later comparison problems.
type QueryTweak struct {
	Note                   string                `yaml:"note" json:"note"`
	NoBug                  bool                  `yaml:"no_bug,omitempty" json:"noBug,omitempty"`
	TruncateTimestampsToMS int64                 `yaml:"truncate_timestamps_to_ms" json:"truncateTimestampsToMS,omitempty"`
	AlignTimestampsToStep  bool                  `yaml:"align_timestamps_to_step" json:"alignTimestampsToStep,omitempty"`
	OffsetTimestampsByMS   int64                 `yaml:"offset_timestamps_by_ms" json:"offsetTimestampsByMS,omitempty"`
	DropResultLabels       []model.LabelName     `yaml:"drop_result_labels" json:"dropResultLabels,omitempty"`
	IgnoreFirstStep        bool                  `yaml:"ignore_first_step" json:"ignoreFirstStep,omitempty"`
	IgnoreCase             bool                  `yaml:"ignore_case" json:"ignoreCase,omitempty"`
	AdjustValueTolerance   *AdjustValueTolerance `yaml:"adjust_value_tolerance" json:"adjustValueTolerance,omitempty"`
}

// AdjustValueTolerance configures approximate float comparison tolerances.
type AdjustValueTolerance struct {
	Fraction *float64 `yaml:"fraction" json:"fraction,omitempty"`
	Margin   *float64 `yaml:"margin" json:"margin,omitempty"`
}

// TestCase represents a given query (pattern) to be tested.
type TestCase struct {
	Query          string   `yaml:"query"`
	VariantArgs    []string `yaml:"variant_args,omitempty"`
	SkipComparison bool     `yaml:"skip_comparison,omitempty"`
	ShouldFail     bool     `yaml:"should_fail,omitempty"`
}

// Load parses the YAML input into a Config.
func Load(content []byte) (*Config, error) {
	cfg := &Config{}
	if err := yaml.UnmarshalStrict(content, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

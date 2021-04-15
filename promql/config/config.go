package config

import (
	"io/ioutil"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

// Config models the main configuration file.
type Config struct {
	ReferenceTargetConfig TargetConfig        `yaml:"reference_target_config"`
	TestTargetConfig      TargetConfig        `yaml:"test_target_config"`
	QueryTweaks           []*QueryTweak       `yaml:"query_tweaks"`
	TestCases             []*TestCase         `yaml:"test_cases"`
	QueryTimeParameters   QueryTimeParameters `yaml:"query_time_parameters"`
}

type QueryTimeParameters struct {
	EndTime             string  `yaml:"end_time"`
	RangeInSeconds      float64 `yaml:"range_in_seconds"`
	ResolutionInSeconds float64 `yaml:"resolution_in_seconds"`
}

// TargetConfig represents the configuration of a single Prometheus API endpoint.
type TargetConfig struct {
	QueryURL      string            `yaml:"query_url"`
	BasicAuthUser string            `yaml:"basic_auth_user"`
	BasicAuthPass string            `yaml:"basic_auth_pass"`
	Headers       map[string]string `yaml:"headers"`
	TSDBPath      string            `yaml:"tsdb_path"`
}

// A QueryTweak restricts or modifies a query in certain ways that avoids certain systematic errors and/or later comparison problems.
type QueryTweak struct {
	Note                   string                `yaml:"note" json:"note"`
	NoBug                  bool                  `yaml:"no_bug,omitempty" json:"noBug,omitempty"`
	TruncateTimestampsToMS int64                 `yaml:"truncate_timestamps_to_ms" json:"truncateTimestampsToMS,omitempty"`
	AlignTimestampsToStep  bool                  `yaml:"align_timestamps_to_step" json:"alignTimestampsToStep,omitempty"`
	DropResultLabels       []model.LabelName     `yaml:"drop_result_labels" json:"dropResultLabels,omitempty"`
	IgnoreFirstStep        bool                  `yaml:"ignore_first_step" json:"ignoreFirstStep,omitempty"`
	AdjustValueTolerance   *AdjustValueTolerance `yaml:"adjust_value_tolerance" json:"adjustValueTolerance,omitempty"`
}

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

// LoadFromFile parses the given YAML file into a Config.
func LoadFromFile(filename string) (*Config, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	cfg, err := Load(content)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing YAML file %s", filename)
	}
	return cfg, nil
}

// Load parses the YAML input into a Config.
func Load(content []byte) (*Config, error) {
	cfg := &Config{}
	err := yaml.UnmarshalStrict(content, cfg)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

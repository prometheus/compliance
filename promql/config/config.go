package config

import (
	"bytes"
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
	AlignTimestampsToMS    int64                 `yaml:"align_timestamps_to_ms" json:"alignTimestampsToMS,omitempty"`
	DropResultLabels       []model.LabelName     `yaml:"drop_result_labels" json:"dropResultLabels,omitempty"`
	IgnoreFirstStep        bool                  `yaml:"ignore_first_step" json:"ignoreFirstStep,omitempty"`
	IgnoreCase             bool                  `yaml:"ignore_case" json:"ignoreCase,omitempty"`
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

// LoadFromFiles parses the given YAML files into a Config.
func LoadFromFiles(filenames []string) (*Config, error) {
	var buf bytes.Buffer
	for _, f := range filenames {
		content, err := ioutil.ReadFile(f)
		if err != nil {
			return nil, errors.Wrapf(err, "reading config file %s", f)
		}
		if _, err := buf.Write(content); err != nil {
			return nil, errors.Wrapf(err, "appending config file %s to buffer", f)
		}
	}
	cfg, err := Load(buf.Bytes())
	if err != nil {
		return nil, errors.Wrapf(err, "parsing YAML files %s", filenames)
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

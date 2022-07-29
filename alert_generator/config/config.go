package config

import (
	"fmt"
	"os"
	"strconv"

	"github.com/pkg/errors"
	"github.com/prometheus/common/sigv4"
	"gopkg.in/yaml.v2"
)

// Config models the main configuration file.
type Config struct {
	Settings  Settings `yaml:"settings"`
	Auth      Auth     `yaml:"auth"`
	TestCases []string `yaml:"test_cases"`
}

type Settings struct {
	// RemoteWriteURL is URL to remote write samples.
	RemoteWriteURL string `yaml:"remote_write_url"`
	// QueryBaseURL is the URL to query the database via PromQL via GET <QueryBaseURL>/query and <QueryBaseURL>/query_range.
	QueryBaseURL string `yaml:"query_base_url"`
	// RulesAndAlertsAPIBaseURL is the URL to query the GET <RulesAndAlertsAPIBaseURL>/api/v1/rules and <RulesAndAlertsAPIBaseURL>/api/v1/alerts.
	RulesAndAlertsAPIBaseURL string `yaml:"rules_and_alerts_api_base_url"`
	// AlertReceptionServerPort is the port at which the alert receiving server will be run. Default: 8080.
	AlertReceptionServerPort string `yaml:"alert_reception_server_port"`

	DisableRulesAPICheck        bool `yaml:"disable_rules_api_check"`
	DisableAlertsAPICheck       bool `yaml:"disable_alerts_api_check"`
	DisableAlertsMetricsCheck   bool `yaml:"disable_alerts_metrics_check"`
	DisableAlertsReceptionCheck bool `yaml:"disable_alerts_reception_check"`

	AlertMessageParser string `yaml:"alert_message_parser"`

	//APIHeaders         map[string]string `yaml:"api_headers"`
	//QueryHeaders       map[string]string `yaml:"query_headers"`
	//RemoteWriteHeaders map[string]string `yaml:"remote_write_headers"`
}

type Auth struct {
	RemoteWrite       AuthConfig `yaml:"remote_write"`
	RulesAndAlertsAPI AuthConfig `yaml:"rules_and_alerts_api"`
	Query             AuthConfig `yaml:"query"`
}

type AuthConfig struct {
	SigV4Config   *sigv4.SigV4Config `yaml:"sigv4"`
	BasicAuthUser string             `yaml:"basic_auth_user"`
	BasicAuthPass string             `yaml:"basic_auth_pass"`
}

func validateConfig(cfg *Config) (*Config, error) {
	if cfg.Auth.RemoteWrite.BasicAuthUser != "" && cfg.Auth.RemoteWrite.BasicAuthPass == "" {
		return nil, errors.New("basic_auth_pass is missing while basic_auth_user is set for remote_write")
	}
	if cfg.Auth.RulesAndAlertsAPI.BasicAuthUser != "" && cfg.Auth.RulesAndAlertsAPI.BasicAuthPass == "" {
		return nil, errors.New("basic_auth_pass is missing while basic_auth_user is set for rules_and_alerts_api")
	}
	if cfg.Auth.Query.BasicAuthUser != "" && cfg.Auth.Query.BasicAuthPass == "" {
		return nil, errors.New("basic_auth_pass is missing while basic_auth_user is set for query")
	}

	if cfg.Settings.RemoteWriteURL == "" {
		return nil, errors.New("remote_write_url is not set")
	}
	if cfg.Settings.QueryBaseURL == "" {
		return nil, errors.New("query_base_url is not set")
	}
	if cfg.Settings.RulesAndAlertsAPIBaseURL == "" {
		return nil, errors.New("rules_and_alerts_api_base_url is not set")
	}
	if cfg.Settings.AlertReceptionServerPort == "" {
		cfg.Settings.AlertReceptionServerPort = "8080"
	}

	p, err := strconv.Atoi(cfg.Settings.AlertReceptionServerPort)
	if err != nil {
		return nil, fmt.Errorf("provided alert server port %q does not parse as an integer", cfg.Settings.AlertReceptionServerPort)
	}
	if p > 65535 {
		return nil, fmt.Errorf("provided alert server port %q must be less than 65535", cfg.Settings.AlertReceptionServerPort)
	}

	//if cfg.Settings.APIHeaders == nil {
	//	cfg.Settings.APIHeaders = make(map[string]string)
	//}
	//if cfg.Settings.QueryHeaders == nil {
	//	cfg.Settings.QueryHeaders = make(map[string]string)
	//}
	//if cfg.Settings.RemoteWriteHeaders == nil {
	//	cfg.Settings.RemoteWriteHeaders = make(map[string]string)
	//}

	return cfg, nil
}

// LoadFromFile parses the given YAML file into a Config.
func LoadFromFile(fname string) (*Config, error) {
	content, err := os.ReadFile(fname)
	if err != nil {
		return nil, errors.Wrapf(err, "reading config file %s", fname)
	}

	cfg := &Config{}
	err = yaml.UnmarshalStrict(content, cfg)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing YAML file %s", fname)
	}
	return validateConfig(cfg)
}

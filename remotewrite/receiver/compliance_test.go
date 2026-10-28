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

package receiver

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/prometheus/prometheus/config"
)

// TestMain sets up the test environment by loading configuration and filtering receivers.
// It loads the Prometheus configuration file specified by PROMETHEUS_RW2_COMPLIANCE_CONFIG_FILE
// environment variable (or falls back to config.yml/config_example.yml) and extracts remote write
// configurations. If PROMETHEUS_RW2_COMPLIANCE_RECEIVERS is set, only matching receivers are tested.
func TestMain(m *testing.M) {
	configFile := os.Getenv("PROMETHEUS_RW2_COMPLIANCE_CONFIG_FILE")
	if configFile == "" {
		if _, err := os.Stat("config.yml"); err == nil {
			configFile = "config.yml"
		} else {
			configFile = "config_example.yml"
		}
	}

	promConfig, err := config.LoadFile(configFile, false, nil)
	if err != nil {
		log.Printf("failed to load config file: %s", err.Error())
		os.Exit(1)
	}

	// Extract Remote-Write configs.
	remoteWriteConfigs = promConfig.RemoteWriteConfigs

	// Filter receivers based on PROMETHEUS_RW2_COMPLIANCE_RECEIVERS environment variable.
	receiverNames := os.Getenv("PROMETHEUS_RW2_COMPLIANCE_RECEIVERS")
	if receiverNames != "" {
		filteredConfigs := filterReceivers(remoteWriteConfigs, receiverNames)
		if len(filteredConfigs) > 0 {
			remoteWriteConfigs = filteredConfigs
			log.Printf("Using filtered receivers: %s", receiverNames)
		} else {
			log.Printf("No receivers found matching %q", receiverNames)
			os.Exit(1)
		}
	}

	var cleanups []func()
	for _, rw := range remoteWriteConfigs {
		if rw.Name == "local-prometheus" && !isReceiverReachable(rw.URL.URL) {
			log.Printf("Receiver %q is not running at %s; starting local Prometheus receiver...", rw.Name, rw.URL.URL)
			startedCfg, cleanup, err := startReceiver(context.Background(), NewPrometheus(rw.Name, ""))
			if err != nil {
				log.Printf("failed to start local Prometheus receiver: %v", err)
				os.Exit(1)
			}
			rw.URL = startedCfg.URL
			cleanups = append(cleanups, cleanup)
		}
	}

	code := m.Run()
	for _, cleanup := range cleanups {
		cleanup()
	}
	os.Exit(code)
}

// TestCompliance runs all baseline compliance tests in this suite against configured receivers.
func TestCompliance(t *testing.T) {
	RunTestsWithConfig(t, remoteWriteConfigs, ComplianceTests())
}

// TestStatelessCompliance runs all stateless mode compliance tests in this suite against configured receivers.
func TestStatelessCompliance(t *testing.T) {
	RunTestsWithConfig(t, remoteWriteConfigs, StatelessComplianceTests())
}

// TestStatelessSTCompliance runs all stateless-st mode compliance tests in this suite against configured receivers.
func TestStatelessSTCompliance(t *testing.T) {
	RunTestsWithConfig(t, remoteWriteConfigs, StatelessSTComplianceTests())
}

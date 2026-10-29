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

package receiver_test

import (
	"os"
	"testing"

	"github.com/prometheus/compliance"
	"github.com/prometheus/compliance/remotewrite/receiver"
	"github.com/prometheus/prometheus/config"
	"github.com/stretchr/testify/require"
)

// TestCompliance runs all Remote-Write 2.0 receiver compliance tests against configured receiver endpoints.
// When local-prometheus is configured and not already listening at its configured URL, TestCompliance
// automatically starts a local reference Prometheus receiver via prometheus{}.
func TestCompliance(t *testing.T) {
	configFile := os.Getenv("PROMETHEUS_RW2_COMPLIANCE_CONFIG_FILE")
	if configFile == "" {
		if _, err := os.Stat("config.yml"); err == nil {
			configFile = "config.yml"
		} else {
			configFile = "config_example.yml"
		}
	}

	promConfig, err := config.LoadFile(configFile, false, nil)
	require.NoError(t, err, "failed to load config file %q", configFile)

	remoteWriteConfigs := promConfig.RemoteWriteConfigs
	externalLabels := promConfig.GlobalConfig.ExternalLabels.Map()

	if receiverNames := os.Getenv("PROMETHEUS_RW2_COMPLIANCE_RECEIVERS"); receiverNames != "" {
		remoteWriteConfigs = receiver.FilterRemoteWriteConfigs(remoteWriteConfigs, receiverNames)
		require.NotEmpty(t, remoteWriteConfigs, "no receivers found matching %q", receiverNames)
	}

	tests := receiver.ComplianceTests()
	for _, rw := range remoteWriteConfigs {
		if rw.Name == "local-prometheus" && (rw.URL == nil || !isReceiverReachable(rw.URL.URL)) {
			_, err := compliance.GetPrometheusBinary()
			require.NoError(t, err)
			receiver.RunTests(t, prometheus{name: rw.Name}, tests, receiver.WithExternalLabels(externalLabels))
			continue
		}

		rec, err := receiver.NewRemoteWriteConfigReceiver(rw)
		require.NoError(t, err)
		receiver.RunTests(t, rec, tests, receiver.WithExternalLabels(externalLabels))
	}
}

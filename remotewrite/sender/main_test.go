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
	"os"
	"strings"
	"testing"

	"github.com/prometheus/compliance/remotewrite/sender/targets"
)

const senderEnvVar = "PROMETHEUS_COMPLIANCE_RW_SENDERS"

var (
	// registeredTargets holds pre-defined targets to choose from.
	//
	// Custom targets could be considered for adding here, however the process target likely offers enough flexibility.
	registeredTargets = map[string]targets.Target{
		"prometheus": targets.RunPrometheus, // Default if no senderEnvVar is specified.
		"process":    targets.RunProcess,
	}
	// targetsToTest is a global variable controlling senders to test.
	// It is adjusted in TestMain via senderEnvVar variable.
	targetsToTest = map[string]targets.Target{
		"prometheus": registeredTargets["prometheus"],
	}
)

// TestMain sets up the test environment by filtering registeredTargets (senders to tests) using
// PROMETHEUS_COMPLIANCE_RW_SENDERS envvar.
func TestMain(m *testing.M) {
	senderNames := os.Getenv(senderEnvVar)
	if senderNames != "" {
		targetsToTest = make(map[string]targets.Target)
		nameList := strings.Split(senderNames, ",")
		for _, name := range nameList {
			name = strings.TrimSpace(name)
			if target, ok := registeredTargets[name]; ok {
				targetsToTest[name] = target
			}
		}
		if len(targetsToTest) == 0 {
			fmt.Println("FAIL: No targets found matching PROMETHEUS_COMPLIANCE_RW_SENDERS=", senderNames)
			os.Exit(1)
		}
	}

	os.Exit(m.Run())
}

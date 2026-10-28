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

package sender_test

import (
	"testing"

	"github.com/prometheus/compliance/remotewrite/sender"
	"github.com/stretchr/testify/require"
)

// TestCompliance runs all baseline compliance tests in this suite against prometheus{}.
func TestCompliance(t *testing.T) {
	t.Parallel()
	_, err := getPrometheusBinary()
	require.NoError(t, err)
	sender.RunTests(t, prometheus{}, sender.ComplianceTests())
}

// TestStatelessCompliance runs all stateless mode compliance tests in this suite against prometheus{}.
func TestStatelessCompliance(t *testing.T) {
	t.Parallel()
	_, err := getPrometheusBinary()
	require.NoError(t, err)
	sender.RunTests(t, prometheus{}, sender.StatelessComplianceTests())
}

// TestStatelessSTCompliance runs all stateless-st mode compliance tests in this suite against prometheus{}.
func TestStatelessSTCompliance(t *testing.T) {
	t.Parallel()
	_, err := getPrometheusBinary()
	require.NoError(t, err)
	sender.RunTests(t, prometheus{}, sender.StatelessSTComplianceTests())
}

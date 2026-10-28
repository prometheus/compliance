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

package promql_test

import (
	"os"
	"testing"

	"github.com/prometheus/compliance/promql"
)

// TestCompliance runs all PromQL compliance tests against Prometheus.
func TestCompliance(t *testing.T) {
	promql.RunTests(
		t,
		promql.NewPrometheus("reference-prometheus", ""),
		promql.NewPrometheus("prometheus", os.Getenv("PROMETHEUS_COMPLIANCE_PROMQL_BINARY")),
		promql.ComplianceTests(),
	)
}

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

package sender

import (
	"testing"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/stretchr/testify/require"
)

func metadataTests() []Test {
	return []Test{
		{
			Name:        "metadata_fields_valid",
			Description: "Sender SHOULD send valid metadata",
			RFCLevel:    ShouldLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `# HELP test_metric A test metric
# TYPE test_metric counter
# UNIT test_metric bytes
test_metric 42
`,
			Validate: func(t *testing.T, res ReceiverResult) {
				var foundMetadata bool
				for _, req := range res.Requests {
					if req.RW2 != nil {
						for _, ts := range req.RW2.Timeseries {
							if ts.Metadata.Type != 0 {
								foundMetadata = true
								require.Less(t, int(ts.Metadata.HelpRef), len(req.RW2.Symbols), "Help ref out of bounds")
								require.Less(t, int(ts.Metadata.UnitRef), len(req.RW2.Symbols), "Unit ref out of bounds")
							}
						}
					}
				}
				if !foundMetadata {
					t.Log("Sender did not send metadata (optional)")
				}
			},
		},
	}
}

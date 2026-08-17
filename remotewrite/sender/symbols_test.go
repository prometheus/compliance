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

func symbolsTests() []Test {
	return []Test{
		{
			Name:        "symbols_table_valid",
			Description: "Symbol table MUST have empty string at index 0 and valid refs",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData:  "test_metric 42\n",
			Validate: func(t *testing.T, res ReceiverResult) {
				for _, req := range res.Requests {
					if req.RW2 != nil {
						require.NotEmpty(t, req.RW2.Symbols, "Symbol table must not be empty")
						require.Equal(t, "", req.RW2.Symbols[0], "Symbol at index 0 must be empty string")

						for _, ts := range req.RW2.Timeseries {
							require.Equal(t, 0, len(ts.LabelsRefs)%2, "Labels refs must be even length")
							for _, ref := range ts.LabelsRefs {
								require.Less(t, int(ref), len(req.RW2.Symbols), "Label ref out of bounds")
							}
						}
					}
				}
			},
		},
	}
}

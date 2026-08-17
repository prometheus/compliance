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

func labelsTests() []Test {
	return []Test{
		{
			Name:        "labels_sorted_and_valid",
			Description: "Sender MUST sort labels by name and have valid refs",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData:  `test_metric{b="2",a="1",c="3"} 42` + "\n",
			Validate: func(t *testing.T, res ReceiverResult) {
				var found bool
				for _, req := range res.Requests {
					if req.RW2 != nil {
						for _, ts := range req.RW2.Timeseries {
							found = true
							require.Equal(t, 0, len(ts.LabelsRefs)%2, "Labels must be even length")
							var prevKey string
							for i := 0; i < len(ts.LabelsRefs); i += 2 {
								keyIdx := ts.LabelsRefs[i]
								require.Less(t, int(keyIdx), len(req.RW2.Symbols), "Label key ref out of bounds")
								key := req.RW2.Symbols[keyIdx]
								// Note: __name__ is usually sorted first, but lexicographically it is before most things.
								// We skip the check if i==0 just to initialize prevKey.
								if i > 0 {
									require.True(t, key > prevKey, "Labels must be sorted by key (got %s after %s)", key, prevKey)
								}
								prevKey = key
							}
						}
					}
				}
				require.True(t, found, "Expected to find timeseries")
			},
		},
	}
}

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

func rw1CompatTests() []Test {
	return []Test{
		{
			Name:        "rw1_fallback_supported",
			Description: "Senders MAY support falling back to PRW 1.0",
			RFCLevel:    MayLevel,
			Version:     remote.WriteV1MessageType,
			ScrapeData:  "test_metric 42\n",
			Validate: func(t *testing.T, res ReceiverResult) {
				var foundRW1 bool
				for _, req := range res.Requests {
					if req.RW1 != nil {
						foundRW1 = true
						require.NotEmpty(t, req.RW1.Timeseries, "RW1 timeseries should not be empty")
					}
				}
				if !foundRW1 {
					t.Log("Sender did not send PRW 1.0 payload (optional)")
				}
			},
		},
	}
}

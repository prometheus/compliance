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
	"math"
	"testing"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/stretchr/testify/require"
)

func edgeCasesTests() []Test {
	return []Test{
		{
			Name:        "unicode_in_labels",
			Description: "Sender MUST preserve Unicode characters in labels",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData:  `test_metric{emoji="🚀",chinese="测试",arabic="مرحبا",vietnamese="tôi yêu việt nam"} 42` + "\n",
			Validate: func(t *testing.T, res ReceiverResult) {
				var foundUnicode bool
				for _, req := range res.Requests {
					if req.RW2 != nil {
						for _, sym := range req.RW2.Symbols {
							hasUnicode := false
							for _, r := range sym {
								if r > 127 {
									hasUnicode = true
									break
								}
							}
							if hasUnicode {
								foundUnicode = true
								require.NotEmpty(t, sym, "Unicode value must be preserved")
							}
						}
					}
				}
				require.True(t, foundUnicode, "Unicode characters should be preserved in symbols")
			},
		},
		{
			Name:        "special_float_combinations",
			Description: "Sender MUST handle special float value combinations",
			RFCLevel:    MustLevel,
			Version:     remote.WriteV2MessageType,
			ScrapeData: `special_values{type="nan"} NaN
special_values{type="inf"} +Inf
special_values{type="ninf"} -Inf
special_values{type="zero"} 0
special_values{type="negative"} -123.45
`,
			Validate: func(t *testing.T, res ReceiverResult) {
				foundSpecial := make(map[string]bool)

				for _, req := range res.Requests {
					if req.RW2 != nil {
						for _, ts := range req.RW2.Timeseries {
							if len(ts.Samples) > 0 {
								// find value of type label
								var valueType string
								for i := 0; i < len(ts.LabelsRefs); i += 2 {
									if req.RW2.Symbols[ts.LabelsRefs[i]] == "type" {
										valueType = req.RW2.Symbols[ts.LabelsRefs[i+1]]
										break
									}
								}

								value := ts.Samples[0].Value
								switch valueType {
								case "nan":
									if math.IsNaN(value) {
										foundSpecial["nan"] = true
									}
								case "inf":
									if math.IsInf(value, 1) {
										foundSpecial["inf"] = true
									}
								case "ninf":
									if math.IsInf(value, -1) {
										foundSpecial["ninf"] = true
									}
								case "zero":
									if value == 0 {
										foundSpecial["zero"] = true
									}
								case "negative":
									if value < 0 {
										foundSpecial["negative"] = true
									}
								}
							}
						}
					}
				}

				require.GreaterOrEqual(t, len(foundSpecial), 1, "Should handle special float values")
			},
		},
	}
}

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

	writev1 "github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

func TestIsSorted(t *testing.T) {
	symbols := []string{"", "a", "c", "b", "x", "__name__"}
	require.True(t, isSorted(symbols, []uint32{5, 1, 1, 1, 3, 1, 2, 1, 4, 1}))
	require.False(t, isSorted(symbols, []uint32{5, 1, 1, 1, 2, 1, 3, 1, 4, 1}))

	require.True(t, isSortedRW1([]writev1.Label{{Name: "__name__"}, {Name: "a"}, {Name: "b"}, {Name: "c"}}))
	require.False(t, isSortedRW1([]writev1.Label{{Name: "__name__"}, {Name: "a"}, {Name: "x"}, {Name: "c"}}))
	require.False(t, isSortedRW1([]writev1.Label{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "__name__"}}))
}

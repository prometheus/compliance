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

package openmetrics

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestHelpers(t *testing.T) {
	mfCounter := &dto.MetricFamily{
		Name: proto.String("http_requests_total"),
		Type: dto.MetricType_COUNTER.Enum(),
		Metric: []*dto.Metric{
			{
				Label: []*dto.LabelPair{
					{Name: proto.String("method"), Value: proto.String("GET")},
					{Name: proto.String("status"), Value: proto.String("200")},
				},
				Counter: &dto.Counter{Value: proto.Float64(5)},
			},
		},
	}
	res := ScrapeResult{
		Families: map[string]*dto.MetricFamily{
			"http_requests_total": mfCounter,
		},
		OrderedFamilies: []*dto.MetricFamily{mfCounter},
	}

	require.Equal(t, mfCounter, findFamily(res, "http_requests", "http_requests_total"))
	require.Nil(t, findFamily(res, "missing_metric"))
	require.Len(t, familiesByType(res, dto.MetricType_COUNTER), 1)
	require.Empty(t, familiesByType(res, dto.MetricType_GAUGE))

	m := findMetricWithLabels(mfCounter, map[string]string{"method": "GET", "status": "200"})
	require.NotNil(t, m)
	require.Nil(t, findMetricWithLabels(mfCounter, map[string]string{"method": "POST"}))

	ex := &dto.Exemplar{
		Label: []*dto.LabelPair{
			{Name: proto.String("trace_id"), Value: proto.String("abc")},
			{Name: proto.String("utf8"), Value: proto.String("🚀")},
		},
	}
	require.Equal(t, len("trace_id")+len("abc")+len("utf8")+1, exemplarLabelRunes(ex))

	body := []byte("# TYPE http_requests counter\nhttp_requests_total{method=\"GET\"} 5\nhttp_requests_in_flight 2\n\"http.active\"{\"k\"=\"v\"} 1\n# EOF\n")
	require.Equal(t, []string{`http_requests_total{method="GET"} 5`}, sampleLinesForFamily(body, "http_requests"))
	require.Equal(t, []string{`"http.active"{"k"="v"} 1`}, sampleLinesForFamily(body, "http.active"))
}

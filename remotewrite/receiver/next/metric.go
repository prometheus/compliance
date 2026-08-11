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

package receiver

import (
	"fmt"
	"math"
	"time"

	"github.com/prometheus/prometheus/model/value"
)

// specialFloatValues contains test float values including special values
// (NaN, Inf) shared across metric test cases.
var specialFloatValues = map[string]float64{
	"1.0":      1.0,
	"StaleNaN": float64(value.StaleNaN),
	"NaN":      float64(value.NormalNaN),
	"Inf":      math.Inf(1),
	"-Inf":     math.Inf(-1),
}

func basicMetric(name string) map[string]string {
	return map[string]string{"__name__": name}
}

func testJobInstanceLabels() map[string]string {
	return map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9090"}
}

// metricTests returns compliance tests covering single and multiple metric
// samples with a variety of float values (including NaN/Inf) and label shapes.
func metricTests() (ret []Test) {
	for name, v := range specialFloatValues {
		ret = append(ret, singleMetricTests(name, v)...)
		ret = append(ret, multipleMetricsTests(name, v)...)
	}
	ret = append(ret, counterWithCreatedTimestampTest())
	return ret
}

func singleMetricTests(valueName string, v float64) []Test {
	cases := []struct {
		name    string
		labels  map[string]string
		success bool
	}{
		{"simple metric", basicMetric("up"), true},
		{"simple metric with multiple labels", testJobInstanceLabels(), true},
		{"simple metric with newlines", map[string]string{"__name__": "up", "job": "test\njob\n", "instance": "localhost:9090"}, true},
		{"simple metric with dots", map[string]string{"__name__": "resource.cpu.usage", "job.name": "testjob", "instance.name": "localhost:9090"}, true},
		{"simple metric with spaces", map[string]string{"__name__": "resource cpu usage", "job name": "testjob", "instance name": "localhost:9090"}, true},
		{"simple metric without name label", map[string]string{"job": "testjob", "instance": "localhost:9090"}, false},
		{"empty metric", map[string]string{}, false},
	}

	var ret []Test
	for _, tc := range cases {
		expectedSamples := 0
		if tc.success {
			expectedSamples = 1
		}
		ret = append(ret, Test{
			Name:        fmt.Sprintf("SingleMetric/%s/%s", valueName, tc.name),
			Description: "Test single metric samples with different float values including special values (NaN, Inf)",
			Opts: RequestOpts{
				Samples: []SampleWithLabels{{Labels: tc.labels, Value: v}},
			},
			Expect:        ExpectedResponse{Samples: expectedSamples},
			ExpectSuccess: tc.success,
		})
	}
	return ret
}

func multipleMetricsTests(valueName string, v float64) []Test {
	cases := []struct {
		name         string
		metrics      []SampleWithLabels
		success      bool
		validSamples int
	}{
		{
			name: "multiple metrics",
			metrics: []SampleWithLabels{
				{Labels: testJobInstanceLabels(), Value: v},
				{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9091"}, Value: v},
				{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9092"}, Value: v},
				{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9093"}, Value: v},
			},
			success:      true,
			validSamples: 4,
		},
		{
			name: "multiple metrics, without name label",
			metrics: []SampleWithLabels{
				{Labels: map[string]string{"job": "testjob", "instance": "localhost:9090"}, Value: v},
				{Labels: map[string]string{"job": "testjob", "instance": "localhost:9091"}, Value: v},
				{Labels: map[string]string{"job": "testjob", "instance": "localhost:9092"}, Value: v},
				{Labels: map[string]string{"job": "testjob", "instance": "localhost:9093"}, Value: v},
			},
			success:      false,
			validSamples: 0,
		},
		{
			name: "multiple metrics, 1 without name label",
			metrics: []SampleWithLabels{
				{Labels: map[string]string{"job": "testjob", "instance": "localhost:9090"}, Value: v},
				{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9091"}, Value: v},
				{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9092"}, Value: v},
				{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9093"}, Value: v},
			},
			success:      false,
			validSamples: 3,
		},
	}

	var ret []Test
	for _, tc := range cases {
		ret = append(ret, Test{
			Name:          fmt.Sprintf("MultipleMetrics/%s/%s", valueName, tc.name),
			Description:   "Test multiple metric samples in single request with validation of partial success scenarios",
			Opts:          RequestOpts{Samples: tc.metrics},
			Expect:        ExpectedResponse{Samples: tc.validSamples},
			ExpectSuccess: tc.success,
		})
	}
	return ret
}

func counterWithCreatedTimestampTest() Test {
	now := time.Now()
	createdTime := now.Add(-1 * time.Hour)

	return Test{
		Name:        "CounterWithCreatedTimestamp",
		Description: "Test counter with created timestamp set in the past",
		Opts: RequestOpts{
			Samples: []SampleWithLabels{{
				Labels:           map[string]string{"__name__": "http_requests_total", "job": "api"},
				Value:            100.0,
				CreatedTimestamp: &createdTime,
			}},
		},
		Expect:        ExpectedResponse{Samples: 1},
		ExpectSuccess: true,
	}
}

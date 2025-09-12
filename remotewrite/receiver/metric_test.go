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

package main

import (
	"math"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/value"
)

// values contains test float values including special values (NaN, Inf) for metric testing.
var values = map[string]float64{
	"1.0":      1.0,
	"StaleNaN": float64(value.StaleNaN),
	"NaN":      float64(value.NormalNaN),
	"Inf":      math.Inf(1),
	"-Inf":     math.Inf(-1),
}

func TestSingleMetric(t *testing.T) {
	t.Attr("description", "Test single metric samples with different float values including special values (NaN, Inf)")
	for k, v := range values {
		t.Run(k, func(t *testing.T) {
			tc := []struct {
				name    string
				metric  SampleWithLabels
				success bool
			}{
				{
					name:    "simple metric",
					metric:  SampleWithLabels{Labels: basicMetric("up"), Value: v},
					success: true,
				},
				{
					name:    "simple metric with multiple labels",
					metric:  SampleWithLabels{Labels: testJobInstanceLabels(), Value: v},
					success: true,
				},
				{
					name:    "simple metric with newlines",
					metric:  SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "test\njob\n", "instance": "localhost:9090"}, Value: v},
					success: true,
				},
				{
					name:    "simple metric with dots",
					metric:  SampleWithLabels{Labels: map[string]string{"__name__": "resource.cpu.usage", "job.name": "testjob", "instance.name": "localhost:9090"}, Value: v},
					success: true,
				},
				{
					name:    "simple metric with spaces",
					metric:  SampleWithLabels{Labels: map[string]string{"__name__": "resource cpu usage", "job name": "testjob", "instance name": "localhost:9090"}, Value: v},
					success: true,
				},
				{
					name:    "simple metric without name label",
					metric:  SampleWithLabels{Labels: map[string]string{"job": "testjob", "instance": "localhost:9090"}, Value: v},
					success: false,
				},
				{
					name:    "empty metric",
					metric:  SampleWithLabels{Labels: map[string]string{}, Value: v},
					success: false,
				},
			}
			for _, tc := range tc {
				runComplianceTest(t, tc.name, tc.name,
					RequestOpts{Samples: []SampleWithLabels{tc.metric}},
					requestParams{
						samples: 1,
					},
					tc.success)
			}
		})
	}
}

func TestCounterWithCreatedTimestamp(t *testing.T) {
	should(t)
	t.Attr("description", "Test counter with created timestamp set in the past")

	now := time.Now()
	createdTime := now.Add(-1 * time.Hour)

	sample := SampleWithLabels{
		Labels:           map[string]string{"__name__": "http_requests_total", "job": "api"},
		Value:            100.0,
		CreatedTimestamp: &createdTime,
	}

	runComplianceTest(t, "", "Counter with created timestamp",
		RequestOpts{Samples: []SampleWithLabels{sample}},
		requestParams{
			samples: 1,
		},
		true)
}

func TestMultipleMetrics(t *testing.T) {
	should(t)
	t.Attr("description", "Test multiple metric samples in single request with validation of partial success scenarios")
	for k, v := range values {
		t.Run(k, func(t *testing.T) {
			tc := []struct {
				name         string
				metrics      []SampleWithLabels
				success      bool
				validSamples int
			}{
				{
					name: "multiple metrics",
					metrics: []SampleWithLabels{
						SampleWithLabels{Labels: testJobInstanceLabels(), Value: v},
						SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9091"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9092"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9093"}, Value: v},
					},
					success:      true,
					validSamples: 4,
				},
				{
					name: "multiple metrics, without name label",
					metrics: []SampleWithLabels{
						SampleWithLabels{Labels: map[string]string{"job": "testjob", "instance": "localhost:9090"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"job": "testjob", "instance": "localhost:9091"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"job": "testjob", "instance": "localhost:9092"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"job": "testjob", "instance": "localhost:9093"}, Value: v},
					},
					success:      false,
					validSamples: 0,
				},
				{
					name: "multiple metrics, 1 without name label",
					metrics: []SampleWithLabels{
						SampleWithLabels{Labels: map[string]string{"job": "testjob", "instance": "localhost:9090"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9091"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9092"}, Value: v},
						SampleWithLabels{Labels: map[string]string{"__name__": "up", "job": "testjob", "instance": "localhost:9093"}, Value: v},
					},
					success:      false,
					validSamples: 3,
				},
			}
			for _, tc := range tc {
				runComplianceTest(t, tc.name, tc.name,
					RequestOpts{Samples: tc.metrics},
					requestParams{
						samples: tc.validSamples,
					},
					tc.success)
			}
		})
	}
}

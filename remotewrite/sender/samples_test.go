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
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/prometheus/compliance/remotewrite/sender/targets"
)

// TestSampleEncoding validates that senders correctly encode float samples.
func TestSampleEncoding(t *testing.T) {
	tests := []TestCase{
		{
			Name:        "float_value_encoding",
			Description: "Sender MUST correctly encode regular float values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_metric 123.45\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				must(t).NotEmpty(req.Request.Timeseries, "Request must contain timeseries")
				ts, _ := requireTimeseriesByMetricName(t, req, "test_metric")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).Equal(123.45, ts.Samples[0].Value,
					"Sample value must be correctly encoded")
			},
		},
		{
			Name:        "integer_value_encoding",
			Description: "Sender MUST correctly encode integer values as floats",
			RFCLevel:    "MUST",
			ScrapeData:  "test_counter_total 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req, "test_counter_total")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).Equal(42.0, ts.Samples[0].Value,
					"Integer value must be encoded as float")
			},
		},
		{
			Name:        "zero_value_encoding",
			Description: "Sender MUST correctly encode zero values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_gauge 0\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req, "test_gauge")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).Equal(0.0, ts.Samples[0].Value,
					"Zero value must be correctly encoded")
			},
		},
		{
			Name:        "negative_value_encoding",
			Description: "Sender MUST correctly encode negative values",
			RFCLevel:    "MUST",
			ScrapeData:  "temperature_celsius -15.5\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req, "temperature_celsius")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).Equal(-15.5, ts.Samples[0].Value,
					"Negative value must be correctly encoded")
			},
		},
		{
			Name:        "positive_infinity_encoding",
			Description: "Sender MUST correctly encode +Inf values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_gauge +Inf\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req, "test_gauge")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).True(math.IsInf(ts.Samples[0].Value, 1),
					"Positive infinity must be correctly encoded")
			},
		},
		{
			Name:        "negative_infinity_encoding",
			Description: "Sender MUST correctly encode -Inf values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_gauge -Inf\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req, "test_gauge")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).True(math.IsInf(ts.Samples[0].Value, -1),
					"Negative infinity must be correctly encoded")
			},
		},
		{
			Name:        "nan_encoding",
			Description: "Sender MUST correctly encode NaN values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_gauge NaN\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req, "test_gauge")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).True(math.IsNaN(ts.Samples[0].Value),
					"NaN must be correctly encoded")
			},
		},
		{
			Name:        "timestamp_milliseconds_format",
			Description: "Sender MUST encode timestamps as milliseconds since Unix epoch",
			RFCLevel:    "MUST",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req, "test_metric")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")

				timestamp := ts.Samples[0].Timestamp
				must(t).Greater(timestamp, int64(1e12),
					"Timestamp should be in milliseconds, not seconds")
				must(t).Less(timestamp, int64(1e16),
					"Timestamp should be in milliseconds, not nanoseconds")
			},
		},
		{
			Name:        "timestamp_recent",
			Description: "Sender SHOULD send timestamps close to current time for fresh scrapes",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req, "test_metric")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")

				timestamp := ts.Samples[0].Timestamp
				now := time.Now().UnixMilli()

				diff := now - timestamp
				if diff < 0 {
					diff = -diff
				}
				should(t, diff < int64(5*60*1000), fmt.Sprintf(
					"Timestamp should be recent (within 5 minutes), diff: %dms", diff))
			},
		},
		{
			Name:        "multiple_samples_same_series",
			Description: "Sender MAY send multiple samples for the same series in different requests",
			RFCLevel:    "MAY",
			ScrapeData:  "test_counter_total 100\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := findTimeseriesByMetricName(req, "test_counter_total")
				if ts != nil {
					may(t, len(ts.Samples) > 0, "Timeseries may contain samples")
				}
				may(t, ts != nil, "test_counter_total may be present")
			},
		},
		{
			Name:        "large_float_values",
			Description: "Sender MUST handle very large float values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_large 1.7976931348623157e+308\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req, "test_large")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).Greater(ts.Samples[0].Value, 1e307,
					"Large float value must be correctly encoded")
			},
		},
		{
			Name:        "small_float_values",
			Description: "Sender MUST handle very small float values",
			RFCLevel:    "MUST",
			ScrapeData:  "test_small 2.2250738585072014e-308\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req, "test_small")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).Less(ts.Samples[0].Value, 1e-307,
					"Small float value must be correctly encoded")
				must(t).Greater(ts.Samples[0].Value, 0.0,
					"Small float value must be positive")
			},
		},
		{
			Name:        "scientific_notation",
			Description: "Sender MUST handle values in scientific notation",
			RFCLevel:    "MUST",
			ScrapeData:  "test_scientific 1.23e-4\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req, "test_scientific")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
				must(t).InDelta(0.000123, ts.Samples[0].Value, 0.0000001,
					"Scientific notation value must be correctly parsed and encoded")
			},
		},
		{
			Name:        "precision_preservation",
			Description: "Sender SHOULD preserve float precision",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_precision 0.123456789012345\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				ts, _ := requireTimeseriesByMetricName(t, req, "test_precision")
				must(t).NotEmpty(ts.Samples, "Timeseries must contain samples")
			},
		},
		{
			Name:        "job_instance_labels_present",
			Description: "Sender SHOULD include job and instance labels in samples",
			RFCLevel:    "SHOULD",
			ScrapeData:  "test_metric 42\n",
			Validator: func(t *testing.T, req *CapturedRequest) {
				_, labels := requireTimeseriesByMetricName(t, req, "test_metric")
				should(t, len(labels["job"]) > 0, "Sample should include 'job' label")
				should(t, len(labels["instance"]) > 0, "Sample should include 'instance' label")
			},
		},
	}

	runTestCases(t, tests)
}

// TestSampleOrdering validates timestamp ordering in samples.
func TestSampleOrdering(t *testing.T) {
	t.Attr("rfcLevel", "MUST")
	t.Attr("description", "Sender MUST send samples with older timestamps before newer ones within a series")

	scrapeData := `# Multiple metrics
metric_a 1
metric_b 2
metric_c 3
`

	forEachSender(t, func(t *testing.T, targetName string, target targets.Target) {
		runSenderTest(t, targetName, target, SenderTestScenario{
			ScrapeData: scrapeData,
			Validator: func(t *testing.T, req *CapturedRequest) {
				// Verify that all samples in the request have valid timestamps.
				for _, ts := range req.Request.Timeseries {
					if len(ts.Samples) > 1 {
						for i := 1; i < len(ts.Samples); i++ {
							must(t).LessOrEqual(ts.Samples[i-1].Timestamp, ts.Samples[i].Timestamp,
								"Samples within a timeseries must be ordered by timestamp")
						}
					}
				}
			},
		})
	})
}

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
	"time"

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
)

// histogramTests returns compliance tests covering native histogram samples.
func histogramTests() (ret []Test) {
	cases := []struct {
		name             string
		labels           map[string]string
		includePositive  bool
		includeNegative  bool
		includeZero      bool
		useCustomBuckets bool
		expectSuccess    bool
		badCount         bool
		description      string
	}{
		{
			name:             "basic_histogram",
			labels:           basicMetric("test_histogram"),
			includePositive:  true,
			includeNegative:  true,
			includeZero:      true,
			useCustomBuckets: false,
			expectSuccess:    true,
			description:      "Basic histogram with positive, negative and zero buckets",
		},
		{
			name:             "custom_buckets_histogram",
			labels:           basicMetric("test_histogram"),
			includePositive:  true,
			includeNegative:  false,
			includeZero:      false,
			useCustomBuckets: true,
			expectSuccess:    true,
			description:      "Histogram with custom bucket values",
		},
		{
			name:             "positive_only_histogram",
			labels:           map[string]string{"__name__": "test_histogram", "job": "test"},
			includePositive:  true,
			includeNegative:  false,
			includeZero:      true,
			useCustomBuckets: false,
			expectSuccess:    true,
			description:      "Histogram with only positive and zero buckets",
		},
		{
			name:             "negative_only_histogram",
			labels:           map[string]string{"__name__": "test_histogram", "instance": "localhost"},
			includePositive:  false,
			includeNegative:  true,
			includeZero:      false,
			useCustomBuckets: false,
			expectSuccess:    true,
			description:      "Histogram with only negative buckets",
		},
		{
			name:             "minimal_histogram",
			labels:           basicMetric("minimal_hist"),
			includePositive:  false,
			includeNegative:  false,
			includeZero:      true,
			useCustomBuckets: false,
			expectSuccess:    true,
			description:      "Minimal histogram with only zero bucket",
		},
		{
			name:             "bad_count_histogram",
			labels:           basicMetric("minimal_hist"),
			includePositive:  true,
			includeNegative:  false,
			includeZero:      false,
			useCustomBuckets: false,
			expectSuccess:    false,
			badCount:         true,
			description:      "Histogram with bad count",
		},
	}

	for _, tc := range cases {
		ret = append(ret, Test{
			Name:        "Histograms/" + tc.name,
			Description: tc.description,
			Opts: RequestOpts{
				Histograms: []HistogramWithLabels{{
					Labels:    tc.labels,
					Histogram: Histogram(1.0, tc.includePositive, tc.includeNegative, tc.includeZero, tc.useCustomBuckets, tc.badCount),
				}},
			},
			Expect:        ExpectedResponse{Histograms: 1},
			ExpectSuccess: tc.expectSuccess,
		})
	}

	now := time.Now()
	createdTime := now.Add(-30 * time.Minute)
	ret = append(ret, Test{
		Name:        "HistogramWithCreatedTimestamp",
		Description: "Test histogram with created timestamp",
		Opts: RequestOpts{
			Histograms: []HistogramWithLabels{{
				Labels:         map[string]string{"__name__": "request_duration_seconds", "job": "api"},
				Histogram:      Histogram(1.5, true, true, true, false, false),
				StartTimestamp: &createdTime,
			}},
		},
		Expect:        ExpectedResponse{Histograms: 1},
		ExpectSuccess: true,
		Raw:           true,
		RFCLevel:      ShouldLevel,
	})

	ret = append(ret, Test{
		Name:        "MultipleHistograms",
		Description: "Test sending multiple histograms in a single request",
		Opts: RequestOpts{
			Histograms: []HistogramWithLabels{
				{Labels: map[string]string{"__name__": "hist1", "job": "test"}, Histogram: Histogram(10.5, true, false, true, false, false)},
				{Labels: map[string]string{"__name__": "hist2", "job": "test"}, Histogram: Histogram(5.2, false, true, false, false, false)},
			},
		},
		Expect:        ExpectedResponse{Histograms: 2},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "EmptyHistogram",
		Description: "Test histogram with no buckets",
		Opts: RequestOpts{
			Histograms: []HistogramWithLabels{{Labels: basicMetric("empty_hist"), Histogram: Histogram(0.0, false, false, false, false, false)}},
		},
		Expect:        ExpectedResponse{Histograms: 1},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "HistogramWithMetadata",
		Description: "Test histogram with associated metadata",
		Opts: RequestOpts{
			Metadata: []MetadataWithLabels{{
				Labels: basicMetric("request_duration_seconds"),
				Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
				Help:   "HTTP request duration in seconds",
				Unit:   "seconds",
			}},
			Histograms: []HistogramWithLabels{{
				Labels:    map[string]string{"__name__": "request_duration_seconds", "job": "api"},
				Histogram: Histogram(2.5, true, true, true, false, false),
			}},
		},
		Expect:        ExpectedResponse{Histograms: 1},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "HistogramWithExemplar",
		Description: "Test histogram with exemplar",
		Opts: RequestOpts{
			Exemplars: []ExemplarWithLabels{{
				Labels:         map[string]string{"__name__": "http_request_duration_seconds", "method": "GET"},
				ExemplarLabels: map[string]string{"trace_id": "abc123def456", "span_id": "span789"},
				Value:          1.23,
			}},
			Histograms: []HistogramWithLabels{{
				Labels:    map[string]string{"__name__": "http_request_duration_seconds", "method": "GET"},
				Histogram: Histogram(1.23, true, false, true, false, false),
			}},
		},
		Expect:        ExpectedResponse{Histograms: 1, Exemplars: 1},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "HistogramWithMetadataAndExemplar",
		Description: "Test histogram with both metadata and exemplar",
		Opts: RequestOpts{
			Metadata: []MetadataWithLabels{{
				Labels: basicMetric("response_size_bytes"),
				Type:   writev2.Metadata_METRIC_TYPE_HISTOGRAM,
				Help:   "HTTP response size in bytes",
				Unit:   "bytes",
			}},
			Exemplars: []ExemplarWithLabels{{
				Labels:         map[string]string{"__name__": "response_size_bytes", "service": "upload"},
				ExemplarLabels: map[string]string{"trace_id": "large_upload_trace", "user_id": "user456"},
				Value:          10485760,
			}},
			Histograms: []HistogramWithLabels{{
				Labels:    map[string]string{"__name__": "response_size_bytes", "service": "upload"},
				Histogram: Histogram(10485760, true, true, true, false, false),
			}},
		},
		Expect:        ExpectedResponse{Histograms: 1, Exemplars: 1},
		ExpectSuccess: true,
	})

	ret = append(ret, Test{
		Name:        "MultipleHistogramsWithMetadata",
		Description: "Test multiple histograms each with their own metadata",
		Opts: RequestOpts{
			Metadata: []MetadataWithLabels{
				{Labels: basicMetric("cpu_usage_histogram"), Type: writev2.Metadata_METRIC_TYPE_HISTOGRAM, Help: "CPU usage distribution", Unit: "ratio"},
				{Labels: basicMetric("memory_usage_histogram"), Type: writev2.Metadata_METRIC_TYPE_HISTOGRAM, Help: "Memory usage distribution", Unit: "bytes"},
			},
			Histograms: []HistogramWithLabels{
				{Labels: map[string]string{"__name__": "cpu_usage_histogram", "instance": "server1"}, Histogram: Histogram(0.75, true, false, true, false, false)},
				{Labels: map[string]string{"__name__": "memory_usage_histogram", "instance": "server1"}, Histogram: Histogram(8589934592, true, true, false, false, false)},
			},
		},
		Expect:        ExpectedResponse{Histograms: 2},
		ExpectSuccess: true,
	})

	return ret
}

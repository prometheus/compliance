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

package comparer

import (
	"context"
	"math"
	"testing"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/compliance/promql/config"
)

type stubPromAPI struct {
	val model.Value
	err error
}

var _ PromAPI = (*stubPromAPI)(nil)

func (s *stubPromAPI) Query(context.Context, string, time.Time, ...v1.Option) (model.Value, v1.Warnings, error) {
	return s.val, nil, s.err
}

func (s *stubPromAPI) QueryRange(context.Context, string, v1.Range, ...v1.Option) (model.Value, v1.Warnings, error) {
	return s.val, nil, s.err
}

func TestCompare_NativeHistograms(t *testing.T) {
	start := time.Unix(1700000000, 0).UTC()
	end := start.Add(time.Minute)
	ts := model.TimeFromUnixNano(start.UnixNano())

	sampleMatrix := func(count, sum, bucketCount float64) model.Matrix {
		return model.Matrix{
			{
				Metric: model.Metric{"__name__": "demo_api_request_duration_native_seconds"},
				Histograms: []model.SampleHistogramPair{
					{
						Timestamp: ts,
						Histogram: &model.SampleHistogram{
							Count: model.FloatString(count),
							Sum:   model.FloatString(sum),
							Buckets: model.HistogramBuckets{
								{
									Boundaries: 0,
									Lower:      0.25,
									Upper:      0.5,
									Count:      model.FloatString(bucketCount),
								},
							},
						},
					},
				},
			},
		}
	}

	for _, tc := range []struct {
		name        string
		ref         model.Matrix
		test        model.Matrix
		tweaks      []*config.QueryTweak
		wantSuccess bool
	}{
		{
			name:        "approx=within_tolerance",
			ref:         sampleMatrix(10.0, 2.5, 4.0),
			test:        sampleMatrix(10.0+1e-9, 2.5-1e-9, 4.0+1e-9),
			wantSuccess: true,
		},
		{
			name:        "approx=outside_tolerance",
			ref:         sampleMatrix(10.0, 2.5, 4.0),
			test:        sampleMatrix(10.0, 2.6, 4.0),
			wantSuccess: false,
		},
		{
			name:        "nan=equated",
			ref:         sampleMatrix(10.0, math.NaN(), 4.0),
			test:        sampleMatrix(10.0, math.NaN(), 4.0),
			wantSuccess: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp := New(&stubPromAPI{val: tc.ref}, &stubPromAPI{val: tc.test}, tc.tweaks)
			res, err := comp.Compare(&TestCase{
				Query:      "demo_api_request_duration_native_seconds",
				Start:      start,
				End:        end,
				Resolution: 10 * time.Second,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Success() != tc.wantSuccess {
				t.Fatalf("got Success()=%v (diff=%q), want %v", res.Success(), res.Diff, tc.wantSuccess)
			}
		})
	}
}

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
	"bytes"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

func generateRW1Request(samples []SampleWithLabels, externalLabels map[string]string) (*http.Request, error) {
	now := nextRequestTime()

	var timeseries []prompb.TimeSeries
	for _, s := range samples {
		lbls := mergeExternalLabels(s.Labels, externalLabels)
		keys := slices.Sorted(maps.Keys(lbls))
		labels := make([]prompb.Label, 0, len(keys))
		for _, k := range keys {
			labels = append(labels, prompb.Label{
				Name:  k,
				Value: lbls[k],
			})
		}

		timeseries = append(timeseries, prompb.TimeSeries{
			Labels: labels,
			Samples: []prompb.Sample{
				{
					Timestamp: now.Add(s.Offset).UnixMilli(),
					Value:     s.Value,
				},
			},
		})
	}

	req := &prompb.WriteRequest{
		Timeseries: timeseries,
	}

	data, err := req.Marshal()
	if err != nil {
		return nil, fmt.Errorf("marshal writev1 request: %w", err)
	}
	compressed := snappy.Encode(nil, data)

	return &http.Request{
		Method: http.MethodPost,
		Header: http.Header{
			"Content-Encoding": []string{"snappy"},
			"Content-Type":     []string{"application/x-protobuf"},
		},
		Body: io.NopCloser(bytes.NewReader(compressed)),
	}, nil
}

func rw1CompatTests() []Test {
	return []Test{
		singleRequestTest(
			"rw1_compat/basic_compatibility",
			"RW2 receivers may accept RW1 sample requests with basic content-type",
			MayLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				return generateRW1Request([]SampleWithLabels{
					{Labels: testJobInstanceLabels(), Value: 1.0},
					{Labels: basicMetric("http_requests_total"), Value: 100.0},
				}, externalLabels)
			},
			requestParams{samples: 0, success: true},
		),
		singleRequestTest(
			"rw1_compat/error_handling",
			"RW2 receivers may handle malformed RW1 requests appropriately",
			MayLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				req, err := generateRW1Request([]SampleWithLabels{
					{Labels: basicMetric("error_test"), Value: 1.0},
				}, externalLabels)
				if err != nil {
					return nil, err
				}
				body, err := io.ReadAll(req.Body)
				if err != nil {
					return nil, err
				}
				body[0] = 0xFF
				req.Body = io.NopCloser(bytes.NewReader(body))
				return req, nil
			},
			requestParams{success: false},
		),
	}
}

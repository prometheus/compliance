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
	"io"
	"net/http"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
)

// generateRW1Request generates an HTTP request that simulates a Remote-Write
// v1.0 request by using RW1 headers and content-type while sending basic
// sample data. This tests RW2 receivers' backward compatibility with RW1 clients.
func generateRW1Request(samples []SampleWithLabels) *http.Request {
	now := time.Now()

	var timeseries []prompb.TimeSeries
	for _, s := range samples {
		var labels []prompb.Label
		for k, v := range s.Labels {
			labels = append(labels, prompb.Label{Name: k, Value: v})
		}
		timeseries = append(timeseries, prompb.TimeSeries{
			Labels: labels,
			Samples: []prompb.Sample{{
				Timestamp: now.Add(s.Offset).UnixMilli(),
				Value:     s.Value,
			}},
		})
	}

	req := &prompb.WriteRequest{Timeseries: timeseries}
	data, _ := req.Marshal()
	compressed := snappy.Encode(nil, data)

	return &http.Request{
		Method: http.MethodPost,
		Header: http.Header{
			"Content-Encoding": []string{"snappy"},
			"Content-Type":     []string{"application/x-protobuf"},
			// RW1 doesn't send version headers.
		},
		Body: io.NopCloser(bytes.NewReader(compressed)),
	}
}

// rw1CompatTests returns compliance tests covering RW2 receivers' optional
// backward compatibility with RW1 clients.
func rw1CompatTests() []Test {
	return []Test{
		{
			Name:        "RW1BasicCompatibility",
			Description: "RW2 receivers may accept RW1 sample requests with basic content-type",
			BuildRequest: func() *http.Request {
				return generateRW1Request([]SampleWithLabels{
					{Labels: testJobInstanceLabels(), Value: 1.0},
					{Labels: basicMetric("http_requests_total"), Value: 100.0},
				})
			},
			Expect:        ExpectedResponse{},
			ExpectSuccess: true,
			Raw:           true,
			RFCLevel:      MayLevel,
		},
		{
			Name:        "RW1ErrorHandling",
			Description: "RW2 receivers may handle malformed RW1 requests appropriately",
			BuildRequest: func() *http.Request {
				req := generateRW1Request([]SampleWithLabels{{Labels: basicMetric("error_test"), Value: 1.0}})
				corruptRequestBody(req, 0xFF) // Corrupt snappy header.
				return req
			},
			Expect:        ExpectedResponse{},
			ExpectSuccess: false,
			Raw:           true,
			RFCLevel:      MayLevel,
		},
	}
}

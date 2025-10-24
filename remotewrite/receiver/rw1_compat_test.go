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
	"bytes"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/require"
)

// generateRW1Request generates an HTTP request that simulates a Remote-Write v1.0 request
// by using RW1 headers and content-type while sending basic sample data.
// This tests RW2 receivers' backward compatibility with RW1 clients.
func generateRW1Request(samples []SampleWithLabels) *http.Request {
	<-ticker.C
	now := time.Now()

	var timeseries []prompb.TimeSeries

	for _, s := range samples {
		var labels []prompb.Label
		for k, v := range s.Labels {
			labels = append(labels, prompb.Label{
				Name:  k,
				Value: v,
			})
		}

		ts := prompb.TimeSeries{
			Labels: labels,
			Samples: []prompb.Sample{
				{
					Timestamp: now.Add(s.Offset).UnixMilli(),
					Value:     s.Value,
				},
			},
		}
		timeseries = append(timeseries, ts)
	}

	req := &prompb.WriteRequest{
		Timeseries: timeseries,
	}

	data, _ := req.Marshal()
	compressed := snappy.Encode(nil, data)

	httpReq := http.Request{
		Method: "POST",
		Header: http.Header{
			"Content-Encoding": []string{"snappy"},
			"Content-Type":     []string{"application/x-protobuf"},
			// RW1 doesn't send version headers.
		},
		Body: io.NopCloser(bytes.NewReader(compressed)),
	}
	return &httpReq
}

// TestRW1BasicCompatibility tests that RW2 receivers can accept basic RW1 sample requests.
func TestRW1BasicCompatibility(t *testing.T) {
	may(t)
	t.Attr("description", "RW2 receivers may accept RW1 sample requests with basic content-type")

	samples := []SampleWithLabels{
		{Labels: testJobInstanceLabels(), Value: 1.0},
		{Labels: basicMetric("http_requests_total"), Value: 100.0},
	}

	request := generateRW1Request(samples)
	runRequest(t, request, requestParams{
		samples: 0,
		success: true,
	})
}

// TestRW1ErrorHandling tests error scenarios with RW1 requests.
func TestRW1ErrorHandling(t *testing.T) {
	may(t)
	t.Attr("description", "RW2 receivers may handle malformed RW1 requests appropriately")

	samples := []SampleWithLabels{
		{Labels: basicMetric("error_test"), Value: 1.0},
	}

	request := generateRW1Request(samples)

	// Corrupt the request body to test error handling.
	body, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	body[0] = 0xFF // Corrupt snappy header.
	request.Body = io.NopCloser(bytes.NewReader(body))

	runRequest(t, request, requestParams{
		success: false,
	})
}

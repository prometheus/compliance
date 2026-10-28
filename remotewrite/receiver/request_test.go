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
	"strings"
	"testing"

	"github.com/golang/snappy"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

func corruptRequestBody(t *testing.T, request *http.Request) io.ReadCloser {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	body[0] = '0'
	return io.NopCloser(bytes.NewReader(body))
}

func TestBadContentTypes(t *testing.T) {
	must(t)
	t.Attr("description", "Test that unsupported content types and proto parameters are rejected with 415 status")
	testCases := []struct {
		name        string
		contentType string
	}{
		{name: "application_json", contentType: "application/json"},
		{name: "image_png", contentType: "image/png"},
		{name: "text_plain", contentType: "text/plain"},
		{name: "unsupported_proto", contentType: "application/x-protobuf;proto=unsupported.Message"},
		{name: "empty_proto", contentType: "application/x-protobuf;proto="},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: map[string]string{"__name__": "up"}, Value: 1.0}}})
			request.Header.Set("Content-Type", tc.contentType)

			runRequest(t, request, requestParams{
				exactReponseCode: http.StatusUnsupportedMediaType,
				success:          false,
			})
		})
	}
}

func TestBadContentTypeModes(t *testing.T) {
	must(t)
	t.Attr("description", "Test that unsupported or malformed mode parameters in Content-Type are rejected with 415 status")
	testCases := []struct {
		name        string
		contentType string
	}{
		{name: "unknown_mode", contentType: "application/x-protobuf;proto=io.prometheus.write.v2.Request;mode=unknown"},
		{name: "empty_mode", contentType: "application/x-protobuf;proto=io.prometheus.write.v2.Request;mode="},
		{name: "mode_on_rw1", contentType: "application/x-protobuf;proto=prometheus.WriteRequest;mode=stateless"},
		{name: "unknown_param", contentType: "application/x-protobuf;proto=io.prometheus.write.v2.Request;unknown=param"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: map[string]string{"__name__": "up"}, Value: 1.0}}})
			request.Header.Set("Content-Type", tc.contentType)

			runRequest(t, request, requestParams{
				exactReponseCode: http.StatusUnsupportedMediaType,
				success:          false,
			})
		})
	}
}

func TestUnsupportedContentTypeAcceptHeader(t *testing.T) {
	should(t)
	t.Attr("description", "Test that 415 responses for unsupported Content-Type, proto, or mode include the RFC 7694 Accept header")
	testCases := []struct {
		name        string
		contentType string
	}{
		{name: "unsupported_media_type", contentType: "application/json"},
		{name: "unsupported_proto", contentType: "application/x-protobuf;proto=unsupported.Message"},
		{name: "unsupported_mode", contentType: "application/x-protobuf;proto=io.prometheus.write.v2.Request;mode=unknown"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			request := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: map[string]string{"__name__": "up"}, Value: 1.0}}})
			request.Header.Set("Content-Type", tc.contentType)

			runRequest(t, request, requestParams{
				exactReponseCode: http.StatusUnsupportedMediaType,
				success:          false,
				validateHeaders: func(t *testing.T, header http.Header) {
					accept := header.Get("Accept")
					require.NotEmpty(t, accept, "415 response SHOULD include Accept header per RFC 7694")
					require.Contains(t, accept, "application/x-protobuf;proto=io.prometheus.write.v2.Request",
						"Accept header SHOULD list supported RW 2.0 Content-Type")
				},
			})
		})
	}
}

func TestBadContentEncodings(t *testing.T) {
	must(t)
	t.Attr("description", "Test that unsupported content encodings are rejected with 415 status")
	for _, contentEncoding := range []string{"gzip", "deflate", "zstd", "identity"} {
		t.Run(contentEncoding, func(t *testing.T) {
			request := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: map[string]string{"__name__": "up"}, Value: 1.0}}})
			request.Header.Set("Content-Encoding", contentEncoding)
			request.Body = corruptRequestBody(t, request)

			runRequest(t, request, requestParams{
				exactReponseCode: http.StatusUnsupportedMediaType,
				success:          false,
			})
		})
	}
}

func TestUnsupportedContentEncodingAcceptEncodingHeader(t *testing.T) {
	should(t)
	t.Attr("description", "Test that 415 responses for unsupported Content-Encoding include the RFC 7694 Accept-Encoding header")
	for _, contentEncoding := range []string{"gzip", "deflate", "zstd"} {
		t.Run(contentEncoding, func(t *testing.T) {
			request := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: map[string]string{"__name__": "up"}, Value: 1.0}}})
			request.Header.Set("Content-Encoding", contentEncoding)

			runRequest(t, request, requestParams{
				exactReponseCode: http.StatusUnsupportedMediaType,
				success:          false,
				validateHeaders: func(t *testing.T, header http.Header) {
					acceptEncoding := strings.ToLower(header.Get("Accept-Encoding"))
					require.NotEmpty(t, acceptEncoding, "415 response SHOULD include Accept-Encoding header per RFC 7694")
					require.Contains(t, acceptEncoding, "snappy", "Accept-Encoding header SHOULD include snappy")
				},
			})
		})
	}
}

func TestCorruptRequestBody(t *testing.T) {
	must(t)
	t.Attr("description", "Test that malformed Snappy request bodies are rejected with 4xx status")
	request := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: map[string]string{"__name__": "up"}, Value: 1.0}}})
	request.Body = corruptRequestBody(t, request)

	runRequest(t, request, requestParams{
		success: false,
	})
}

func TestSnappyFramedFormatRejected(t *testing.T) {
	must(t)
	t.Attr("description", "Test that Snappy framed format is rejected with 400 status since Snappy block format MUST be used")

	symbols := writev2.NewSymbolTable()
	labelRefs := symbolizeSortedLabels(&symbols, basicMetric("up"))
	req := &writev2.Request{
		Symbols: symbols.Symbols(),
		Timeseries: []writev2.TimeSeries{
			{
				LabelsRefs: labelRefs,
				Samples:    []writev2.Sample{{Timestamp: 1000, Value: 1.0}},
			},
		},
	}
	data, err := req.Marshal()
	require.NoError(t, err)

	var framedBuf bytes.Buffer
	framedWriter := snappy.NewBufferedWriter(&framedBuf)
	_, err = framedWriter.Write(data)
	require.NoError(t, err)
	require.NoError(t, framedWriter.Close())

	request := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("up"), Value: 1.0}}})
	request.Body = io.NopCloser(bytes.NewReader(framedBuf.Bytes()))

	runRequest(t, request, requestParams{
		success: false,
	})
}

func TestUncompressedProtobufRejected(t *testing.T) {
	must(t)
	t.Attr("description", "Test that uncompressed protobuf bytes with Content-Encoding snappy are rejected with 400 status")

	symbols := writev2.NewSymbolTable()
	labelRefs := symbolizeSortedLabels(&symbols, basicMetric("up"))
	req := &writev2.Request{
		Symbols: symbols.Symbols(),
		Timeseries: []writev2.TimeSeries{
			{
				LabelsRefs: labelRefs,
				Samples:    []writev2.Sample{{Timestamp: 1000, Value: 1.0}},
			},
		},
	}
	data, err := req.Marshal()
	require.NoError(t, err)

	request := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("up"), Value: 1.0}}})
	request.Body = io.NopCloser(bytes.NewReader(data))

	runRequest(t, request, requestParams{
		success: false,
	})
}

func TestInvalidProtobufBodyRejected(t *testing.T) {
	must(t)
	t.Attr("description", "Test that Snappy-compressed invalid protobuf wire bytes are rejected with 400 status")

	compressed := snappy.Encode(nil, []byte("invalid protobuf wire payload \xff\xff\xff"))
	request := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("up"), Value: 1.0}}})
	request.Body = io.NopCloser(bytes.NewReader(compressed))

	runRequest(t, request, requestParams{
		success: false,
	})
}

func TestUnsupportedHTTPMethods(t *testing.T) {
	must(t)
	t.Attr("description", "Test that non-POST HTTP methods are rejected with 4xx status")
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			request := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("up"), Value: 1.0}}})
			request.Method = method

			runRequest(t, request, requestParams{
				success:     false,
				allowAny4xx: true,
			})
		})
	}
}

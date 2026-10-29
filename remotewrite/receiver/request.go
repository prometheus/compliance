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
	"strings"
	"testing"
	"time"

	"github.com/golang/snappy"
	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/stretchr/testify/require"
)

func corruptRequestBody(request *http.Request) (io.ReadCloser, error) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	body[0] = '0'
	return io.NopCloser(bytes.NewReader(body)), nil
}

func requestTests() []Test {
	var tests []Test

	// Unsupported Content-Types and proto parameters rejected with 415.
	badContentTypes := []struct {
		name        string
		contentType string
	}{
		{name: "application_json", contentType: "application/json"},
		{name: "image_png", contentType: "image/png"},
		{name: "text_plain", contentType: "text/plain"},
		{name: "unsupported_proto", contentType: "application/x-protobuf;proto=unsupported.Message"},
		{name: "empty_proto", contentType: "application/x-protobuf;proto="},
	}
	for _, tc := range badContentTypes {
		ct := tc.contentType
		tests = append(tests, singleRequestTest(
			"request/bad_content_types/"+tc.name,
			"Test that unsupported content types and proto parameters are rejected with 415 status",
			MustLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				req, err := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("up"), Value: 1.0}}}, externalLabels)
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", ct)
				return req, nil
			},
			requestParams{exactResponseCode: http.StatusUnsupportedMediaType, success: false},
		))
	}

	// Unsupported or malformed mode parameters in Content-Type rejected with 415.
	badContentTypeModes := []struct {
		name        string
		contentType string
	}{
		{name: "unknown_mode", contentType: "application/x-protobuf;proto=io.prometheus.write.v2.Request;mode=unknown"},
		{name: "empty_mode", contentType: "application/x-protobuf;proto=io.prometheus.write.v2.Request;mode="},
		{name: "empty_quoted_mode", contentType: `application/x-protobuf;proto=io.prometheus.write.v2.Request;mode=""`},
		{name: "mode_on_rw1", contentType: "application/x-protobuf;proto=prometheus.WriteRequest;mode=stateless"},
		{name: "mode_without_proto", contentType: "application/x-protobuf;mode=stateless"},
		{name: "unknown_param", contentType: "application/x-protobuf;proto=io.prometheus.write.v2.Request;unknown=param"},
	}
	for _, tc := range badContentTypeModes {
		ct := tc.contentType
		tests = append(tests, singleRequestTest(
			"request/bad_content_type_modes/"+tc.name,
			"Test that unsupported or malformed mode parameters in Content-Type are rejected with 415 status",
			MustLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				req, err := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("up"), Value: 1.0}}}, externalLabels)
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", ct)
				return req, nil
			},
			requestParams{exactResponseCode: http.StatusUnsupportedMediaType, success: false},
		))
	}

	// RFC 7694 Accept header on 415 responses for unsupported Content-Type, proto, or mode.
	acceptHeaderCases := []struct {
		name        string
		contentType string
	}{
		{name: "unsupported_media_type", contentType: "application/json"},
		{name: "unsupported_proto", contentType: "application/x-protobuf;proto=unsupported.Message"},
		{name: "unsupported_mode", contentType: "application/x-protobuf;proto=io.prometheus.write.v2.Request;mode=unknown"},
	}
	for _, tc := range acceptHeaderCases {
		ct := tc.contentType
		tests = append(tests, singleRequestTest(
			"request/unsupported_content_type_accept_header/"+tc.name,
			"Test that 415 responses for unsupported Content-Type, proto, or mode include the RFC 7694 Accept header",
			ShouldLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				req, err := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("up"), Value: 1.0}}}, externalLabels)
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Type", ct)
				return req, nil
			},
			requestParams{
				exactResponseCode: http.StatusUnsupportedMediaType,
				success:           false,
				validateHeaders: func(t *testing.T, header http.Header) {
					accept := header.Get("Accept")
					require.NotEmpty(t, accept, "415 response SHOULD include Accept header per RFC 7694")
					require.Contains(t, accept, "application/x-protobuf;proto=io.prometheus.write.v2.Request",
						"Accept header SHOULD list supported RW 2.0 Content-Type")
				},
			},
		))
	}

	// Unsupported Content-Encoding values rejected with 415.
	for _, contentEncoding := range []string{"gzip", "deflate", "zstd", "identity"} {
		ce := contentEncoding
		tests = append(tests, singleRequestTest(
			"request/bad_content_encodings/"+ce,
			"Test that unsupported content encodings are rejected with 415 status",
			MustLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				req, err := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("up"), Value: 1.0}}}, externalLabels)
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Encoding", ce)
				req.Body, err = corruptRequestBody(req)
				return req, err
			},
			requestParams{exactResponseCode: http.StatusUnsupportedMediaType, success: false},
		))
	}

	// RFC 7694 Accept-Encoding header on 415 responses for unsupported Content-Encoding.
	for _, contentEncoding := range []string{"gzip", "deflate", "zstd"} {
		ce := contentEncoding
		tests = append(tests, singleRequestTest(
			"request/unsupported_content_encoding_accept_encoding_header/"+ce,
			"Test that 415 responses for unsupported Content-Encoding include the RFC 7694 Accept-Encoding header",
			ShouldLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				req, err := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("up"), Value: 1.0}}}, externalLabels)
				if err != nil {
					return nil, err
				}
				req.Header.Set("Content-Encoding", ce)
				return req, nil
			},
			requestParams{
				exactResponseCode: http.StatusUnsupportedMediaType,
				success:           false,
				validateHeaders: func(t *testing.T, header http.Header) {
					acceptEncoding := strings.ToLower(header.Get("Accept-Encoding"))
					require.NotEmpty(t, acceptEncoding, "415 response SHOULD include Accept-Encoding header per RFC 7694")
					require.Contains(t, acceptEncoding, "snappy", "Accept-Encoding header SHOULD include snappy")
				},
			},
		))
	}

	// Malformed Snappy request body rejected with 4xx.
	tests = append(tests, singleRequestTest(
		"request/corrupt_body",
		"Test that malformed Snappy request bodies are rejected with 4xx status",
		MustLevel,
		func(externalLabels map[string]string) (*http.Request, error) {
			req, err := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("up"), Value: 1.0}}}, externalLabels)
			if err != nil {
				return nil, err
			}
			req.Body, err = corruptRequestBody(req)
			return req, err
		},
		requestParams{success: false},
	))

	// Snappy framed format rejected because Snappy block format MUST be used.
	tests = append(tests, singleRequestTest(
		"request/snappy_framed_rejected",
		"Test that Snappy framed format is rejected with 400 status since Snappy block format MUST be used",
		MustLevel,
		func(externalLabels map[string]string) (*http.Request, error) {
			symbols := writev2.NewSymbolTable()
			labelRefs := symbolizeSortedLabels(&symbols, basicMetric("snappy_framed_rejected"), externalLabels)
			protoReq := &writev2.Request{
				Symbols: symbols.Symbols(),
				Timeseries: []writev2.TimeSeries{
					{
						LabelsRefs: labelRefs,
						Samples:    []writev2.Sample{{Timestamp: time.Now().UnixMilli(), Value: 1.0}},
					},
				},
			}
			data, err := protoReq.Marshal()
			if err != nil {
				return nil, err
			}
			var framedBuf bytes.Buffer
			framedWriter := snappy.NewBufferedWriter(&framedBuf)
			if _, err := framedWriter.Write(data); err != nil {
				return nil, err
			}
			if err := framedWriter.Close(); err != nil {
				return nil, err
			}
			req, err := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("snappy_framed_rejected"), Value: 1.0}}}, externalLabels)
			if err != nil {
				return nil, err
			}
			req.Body = io.NopCloser(bytes.NewReader(framedBuf.Bytes()))
			return req, nil
		},
		requestParams{success: false},
	))

	// Uncompressed protobuf bytes with Content-Encoding: snappy rejected with 400.
	tests = append(tests, singleRequestTest(
		"request/uncompressed_protobuf_rejected",
		"Test that uncompressed protobuf bytes with Content-Encoding snappy are rejected with 400 status",
		MustLevel,
		func(externalLabels map[string]string) (*http.Request, error) {
			symbols := writev2.NewSymbolTable()
			labelRefs := symbolizeSortedLabels(&symbols, basicMetric("uncompressed_protobuf_rejected"), externalLabels)
			protoReq := &writev2.Request{
				Symbols: symbols.Symbols(),
				Timeseries: []writev2.TimeSeries{
					{
						LabelsRefs: labelRefs,
						Samples:    []writev2.Sample{{Timestamp: time.Now().UnixMilli(), Value: 1.0}},
					},
				},
			}
			data, err := protoReq.Marshal()
			if err != nil {
				return nil, err
			}
			req, err := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("uncompressed_protobuf_rejected"), Value: 1.0}}}, externalLabels)
			if err != nil {
				return nil, err
			}
			req.Body = io.NopCloser(bytes.NewReader(data))
			return req, nil
		},
		requestParams{success: false},
	))

	// Snappy-compressed invalid protobuf wire payload rejected with 400.
	tests = append(tests, singleRequestTest(
		"request/invalid_protobuf_rejected",
		"Test that Snappy-compressed invalid protobuf wire bytes are rejected with 400 status",
		MustLevel,
		func(externalLabels map[string]string) (*http.Request, error) {
			compressed := snappy.Encode(nil, []byte("invalid protobuf wire payload \xff\xff\xff"))
			req, err := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("up"), Value: 1.0}}}, externalLabels)
			if err != nil {
				return nil, err
			}
			req.Body = io.NopCloser(bytes.NewReader(compressed))
			return req, nil
		},
		requestParams{success: false},
	))

	// Non-POST HTTP methods rejected with 4xx.
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		m := method
		tests = append(tests, singleRequestTest(
			"request/unsupported_http_methods/"+m,
			"Test that non-POST HTTP methods are rejected with 4xx status",
			MustLevel,
			func(externalLabels map[string]string) (*http.Request, error) {
				req, err := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: basicMetric("up"), Value: 1.0}}}, externalLabels)
				if err != nil {
					return nil, err
				}
				req.Method = m
				return req, nil
			},
			requestParams{success: false, allowAny4xx: true},
		))
	}

	return tests
}

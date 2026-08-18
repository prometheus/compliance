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
)

// corruptRequestBody replaces req's first body byte with corruptByte, so the
// request stops being valid snappy/protobuf while everything else (headers,
// method) stays intact.
func corruptRequestBody(req *http.Request, corruptByte byte) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		panic(err)
	}
	if len(body) > 0 {
		body[0] = corruptByte
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
}

// requestValidationTests returns compliance tests covering rejection of
// malformed requests (unsupported content type/encoding, corrupt bodies).
func requestValidationTests() (ret []Test) {
	basicOpts := RequestOpts{Samples: []SampleWithLabels{{Labels: map[string]string{"__name__": "up"}, Value: 1.0}}}

	for _, contentType := range []string{"application/json", "image/png"} {
		contentType := contentType
		ret = append(ret, Test{
			Name:        "BadContentTypes/" + strings.ReplaceAll(contentType, "/", "_"),
			Description: "Test that unsupported content types are rejected with 415 status",
			BuildRequest: func() *http.Request {
				req := generateRequest(basicOpts)
				req.Header.Set("Content-Type", contentType)
				corruptRequestBody(req, '0')
				return req
			},
			Expect:        ExpectedResponse{ExactStatusCode: http.StatusUnsupportedMediaType},
			ExpectSuccess: false,
			Raw:           true,
			RFCLevel:      MustLevel,
		})
	}

	for _, contentEncoding := range []string{"gzip", "deflate"} {
		contentEncoding := contentEncoding
		ret = append(ret, Test{
			Name:        "BadContentEncodings/" + contentEncoding,
			Description: "Test that unsupported content encodings are rejected with 415 status",
			BuildRequest: func() *http.Request {
				req := generateRequest(basicOpts)
				req.Header.Set("Content-Encoding", contentEncoding)
				corruptRequestBody(req, '0')
				return req
			},
			Expect:        ExpectedResponse{ExactStatusCode: http.StatusUnsupportedMediaType},
			ExpectSuccess: false,
			Raw:           true,
			RFCLevel:      MustLevel,
		})
	}

	ret = append(ret, Test{
		Name:        "CorruptRequestBody",
		Description: "Test that malformed protobuf request bodies are rejected with 4xx status",
		BuildRequest: func() *http.Request {
			req := generateRequest(basicOpts)
			corruptRequestBody(req, '0')
			return req
		},
		Expect:        ExpectedResponse{},
		ExpectSuccess: false,
		Raw:           true,
		RFCLevel:      MustLevel,
	})

	return ret
}

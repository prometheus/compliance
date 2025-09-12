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
	t.Attr("description", "Test that unsupported content types are rejected with 415 status")
	for _, contentType := range []string{"application/json", "image/png"} {
		t.Run(strings.ReplaceAll(contentType, "/", "_"), func(t *testing.T) {
			request := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: map[string]string{"__name__": "up"}, Value: 1.0}}})
			request.Header.Set("Content-Type", contentType)
			request.Body = corruptRequestBody(t, request)

			runRequest(t, request, requestParams{
				exactReponseCode: http.StatusUnsupportedMediaType,
				success:          false,
			})
		})
	}
}
func TestBadContentEncodings(t *testing.T) {
	must(t)
	t.Attr("description", "Test that unsupported content encodings are rejected with 415 status")
	for _, contentEncoding := range []string{"gzip", "deflate"} {
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

func TestCorruptRequestBody(t *testing.T) {
	must(t)
	t.Attr("description", "Test that malformed protobuf request bodies are rejected with 4xx status")
	request := generateRequest(RequestOpts{Samples: []SampleWithLabels{{Labels: map[string]string{"__name__": "up"}, Value: 1.0}}})
	request.Body = corruptRequestBody(t, request)

	runRequest(t, request, requestParams{
		success: false,
	})
}

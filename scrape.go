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

package compliance

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

const (
	// OM1ContentType is the OpenMetrics 1.0 text exposition Content-Type header value.
	OM1ContentType = "application/openmetrics-text; version=1.0.0; charset=utf-8"
	// ProtoMediaType is the protobuf exposition media type matched in HTTP Accept headers.
	ProtoMediaType = "application/vnd.google.protobuf"
	// ProtoContentType is the delimited Prometheus protobuf exposition Content-Type header value.
	ProtoContentType = "application/vnd.google.protobuf; proto=io.prometheus.client.MetricFamily; encoding=delimited"
)

// ScrapePayload represents a single metric exposition response served by ScrapeServer.
type ScrapePayload struct {
	// Text holds the OpenMetrics/text exposition payload (or raw bytes when ContentType is explicitly set).
	Text string
	// Proto holds an optional delimited MetricFamily protobuf payload served when the scraper's
	// Accept header negotiates ProtoMediaType and ContentType is empty.
	Proto []byte
	// ContentType overrides the response Content-Type header when non-empty.
	// When empty, ScrapeServer negotiates ProtoContentType (if Proto is non-empty and requested via Accept)
	// or defaults to OM1ContentType with a trailing "# EOF\n" ensured on Text.
	ContentType string
}

// ScrapeServerOpts configures a ScrapeServer.
type ScrapeServerOpts struct {
	// Payloads is the ordered sequence of scrape responses to serve.
	Payloads []ScrapePayload
	// RepeatLast controls post-sequence scrape behaviour:
	// when true, scrapes after the final payload repeat the last payload;
	// when false, scrapes after the final payload return an empty valid exposition ("# EOF\n" or empty protobuf).
	RepeatLast bool
}

// ScrapeServer is an in-process HTTP server that serves a sequence of Prometheus/OpenMetrics scrape payloads.
type ScrapeServer struct {
	server     *httptest.Server
	repeatLast bool

	mu       sync.Mutex
	payloads []ScrapePayload
	idx      int
	doneOnce sync.Once
	doneCh   chan struct{}
}

// NewScrapeServer starts a new HTTP ScrapeServer with the given options.
func NewScrapeServer(opts ScrapeServerOpts) *ScrapeServer {
	s := &ScrapeServer{
		repeatLast: opts.RepeatLast,
		payloads:   opts.Payloads,
		doneCh:     make(chan struct{}),
	}
	if len(s.payloads) == 0 {
		s.doneOnce.Do(func() { close(s.doneCh) })
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.handleScrape))
	return s
}

// NewTextScrapeServer starts a ScrapeServer that serves initial on the first scrape,
// additional payloads sequentially on subsequent scrapes, and repeats the final payload thereafter.
func NewTextScrapeServer(initial string, additional []string, contentType string) *ScrapeServer {
	payloads := make([]ScrapePayload, 0, 1+len(additional))
	payloads = append(payloads, ScrapePayload{
		Text:        initial,
		ContentType: contentType,
	})
	for _, m := range additional {
		payloads = append(payloads, ScrapePayload{
			Text:        m,
			ContentType: contentType,
		})
	}
	return NewScrapeServer(ScrapeServerOpts{
		Payloads:   payloads,
		RepeatLast: true,
	})
}

func (s *ScrapeServer) handleScrape(w http.ResponseWriter, req *http.Request) {
	s.mu.Lock()
	var payload ScrapePayload
	hasPayload := false
	switch {
	case s.idx < len(s.payloads):
		payload = s.payloads[s.idx]
		hasPayload = true
		s.idx++
		if s.idx == len(s.payloads) {
			s.doneOnce.Do(func() { close(s.doneCh) })
		}
	case s.repeatLast && len(s.payloads) > 0:
		payload = s.payloads[len(s.payloads)-1]
		hasPayload = true
		s.idx++
	}
	s.mu.Unlock()

	if hasPayload && payload.ContentType != "" {
		w.Header().Set("Content-Type", payload.ContentType)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, payload.Text)
		return
	}

	wantsProto := strings.Contains(req.Header.Get("Accept"), ProtoMediaType)
	if hasPayload {
		if len(payload.Proto) > 0 && wantsProto {
			w.Header().Set("Content-Type", ProtoContentType)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(payload.Proto)
			return
		}
		text := payload.Text
		if !strings.HasSuffix(text, "# EOF\n") {
			text += "# EOF\n"
		}
		w.Header().Set("Content-Type", OM1ContentType)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, text)
		return
	}

	// Replay sequence exhausted and RepeatLast is false: serve an empty valid exposition.
	if wantsProto {
		w.Header().Set("Content-Type", ProtoContentType)
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Header().Set("Content-Type", OM1ContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "# EOF\n")
}

// URL returns the full HTTP URL of the scrape server.
func (s *ScrapeServer) URL() string {
	return s.server.URL
}

// HostPort returns the "host:port" address of the scrape server.
func (s *ScrapeServer) HostPort(tb testing.TB) string {
	tb.Helper()
	u, err := url.Parse(s.server.URL)
	if err != nil {
		tb.Fatal(err)
	}
	return u.Host
}

// Done returns a channel that is closed once all configured payloads have been scraped at least once.
func (s *ScrapeServer) Done() <-chan struct{} {
	return s.doneCh
}

// ScrapedCount returns the number of scrapes served (capped at PayloadCount when RepeatLast is false).
func (s *ScrapeServer) ScrapedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idx
}

// PayloadCount returns the total number of configured scrape payloads.
func (s *ScrapeServer) PayloadCount() int {
	return len(s.payloads)
}

// Run blocks until ctx is cancelled and then closes the underlying HTTP server.
func (s *ScrapeServer) Run(ctx context.Context) {
	<-ctx.Done()
	s.Close()
}

// Close shuts down the underlying HTTP server.
func (s *ScrapeServer) Close() {
	s.server.Close()
}

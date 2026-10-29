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

package compliance_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/compliance"
)

func TestSafeRunAndValidateCases(t *testing.T) {
	t.Setenv("DEBUG", "")

	var ranMust, ranShould bool
	ok := compliance.SafeRun(t, "setup", "mock-target", "Verify SafeRun passes", compliance.MustLevel, func(_ *testing.T) {})
	if !ok {
		t.Fatal("expected SafeRun to return true on passing subtest")
	}

	compliance.RunValidateCases(t, "mock-target", ok, 42, []compliance.ValidateCase[int]{
		{
			Name:        "must_check",
			Description: "Check captured value",
			RFCLevel:    compliance.MustLevel,
			Validate: func(t *testing.T, res int) {
				ranMust = true
				if res != 42 {
					t.Fatalf("got %d, want 42", res)
				}
			},
		},
		{
			Name:        "should_check",
			Description: "Secondary check",
			RFCLevel:    compliance.ShouldLevel,
			Validate: func(_ *testing.T, _ int) {
				ranShould = true
			},
		},
	})

	if !ranMust || !ranShould {
		t.Fatalf("expected all validate cases to run: ranMust=%v ranShould=%v", ranMust, ranShould)
	}
}

func TestScrapeServerTextRepeatLast(t *testing.T) {
	srv := compliance.NewTextScrapeServer(
		"metric_a 1\n",
		[]string{"metric_a 2\n# EOF\n"},
		"",
	)
	t.Cleanup(srv.Close)

	type wantResp struct {
		body        string
		contentType string
	}
	wants := []wantResp{
		{body: "metric_a 1\n# EOF\n", contentType: compliance.OM1ContentType},
		{body: "metric_a 2\n# EOF\n", contentType: compliance.OM1ContentType},
		{body: "metric_a 2\n# EOF\n", contentType: compliance.OM1ContentType},
	}

	for i, w := range wants {
		resp, err := http.Get(srv.URL())
		if err != nil {
			t.Fatalf("scrape %d failed: %v", i, err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("read scrape %d body: %v", i, err)
		}
		if got := resp.Header.Get("Content-Type"); got != w.contentType {
			t.Fatalf("scrape %d Content-Type = %q, want %q", i, got, w.contentType)
		}
		if string(body) != w.body {
			t.Fatalf("scrape %d body = %q, want %q", i, string(body), w.body)
		}
	}

	select {
	case <-srv.Done():
	default:
		t.Fatal("expected Done() channel to be closed after all payloads were scraped")
	}
	if got := srv.ScrapedCount(); got != 3 {
		t.Fatalf("ScrapedCount() = %d, want 3", got)
	}
}

func TestScrapeServerCustomContentType(t *testing.T) {
	rawProto := "\x0a\x04test"
	srv := compliance.NewTextScrapeServer(rawProto, nil, compliance.ProtoContentType)
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL())
	if err != nil {
		t.Fatalf("scrape failed: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if got := resp.Header.Get("Content-Type"); got != compliance.ProtoContentType {
		t.Fatalf("Content-Type = %q, want %q", got, compliance.ProtoContentType)
	}
	if string(body) != rawProto {
		t.Fatalf("body = %q, want raw bytes without # EOF suffix %q", string(body), rawProto)
	}
}

func TestScrapeServerReplayAndProtoNegotiation(t *testing.T) {
	srv := compliance.NewScrapeServer(compliance.ScrapeServerOpts{
		Payloads: []compliance.ScrapePayload{
			{Text: "demo 1\n# EOF\n", Proto: []byte{0x01, 0x02}},
			{Text: "demo 2\n# EOF\n", Proto: []byte{0x03, 0x04}},
		},
		RepeatLast: false,
	})
	t.Cleanup(srv.Close)

	doScrape := func(accept string) (string, []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://"+srv.HostPort(t), nil)
		if err != nil {
			t.Fatal(err)
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.Header.Get("Content-Type"), b
	}

	// Scrape 1 negotiates protobuf.
	ct, b := doScrape(compliance.ProtoMediaType + ";q=0.9")
	if ct != compliance.ProtoContentType || string(b) != "\x01\x02" {
		t.Fatalf("scrape 1 got (%q, %x), want (%q, 0102)", ct, b, compliance.ProtoContentType)
	}
	select {
	case <-srv.Done():
		t.Fatal("Done() closed prematurely after 1 of 2 payloads")
	default:
	}

	// Scrape 2 falls back to OpenMetrics 1.0 text.
	ct, b = doScrape("text/plain")
	if ct != compliance.OM1ContentType || string(b) != "demo 2\n# EOF\n" {
		t.Fatalf("scrape 2 got (%q, %q), want (%q, %q)", ct, string(b), compliance.OM1ContentType, "demo 2\n# EOF\n")
	}
	select {
	case <-srv.Done():
	default:
		t.Fatal("expected Done() to close after 2 of 2 payloads")
	}

	// Post-replay scrapes return empty valid expositions without advancing ScrapedCount.
	ct, b = doScrape(compliance.ProtoMediaType)
	if ct != compliance.ProtoContentType || len(b) != 0 {
		t.Fatalf("post-replay proto scrape got (%q, %x), want empty proto response", ct, b)
	}
	ct, b = doScrape("")
	if ct != compliance.OM1ContentType || string(b) != "# EOF\n" {
		t.Fatalf("post-replay text scrape got (%q, %q), want # EOF", ct, string(b))
	}
	if got := srv.ScrapedCount(); got != 2 {
		t.Fatalf("ScrapedCount() = %d, want 2", got)
	}
}

func TestFreeLocalHostPortAndWaitForHTTPReady(t *testing.T) {
	addr := compliance.FreeLocalHostPort(t)
	if _, _, err := net.SplitHostPort(addr); err != nil {
		t.Fatalf("FreeLocalHostPort returned invalid host:port %q: %v", addr, err)
	}

	t.Run("becomes_ready", func(t *testing.T) {
		var attempts atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if attempts.Add(1) < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		if err := compliance.WaitForHTTPReady(ctx, srv.URL, nil, "mock"); err != nil {
			t.Fatalf("WaitForHTTPReady failed: %v", err)
		}
	})

	t.Run("premature_exit", func(t *testing.T) {
		deadAddr := compliance.FreeLocalHostPort(t)
		errCh := make(chan error, 1)
		errCh <- errors.New("boom")

		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		if err := compliance.WaitForHTTPReady(ctx, "http://"+deadAddr+"/-/ready", errCh, "mock"); err == nil {
			t.Fatal("expected premature exit error, got nil")
		}
	})
}

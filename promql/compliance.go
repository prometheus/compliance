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

package promql

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/compliance/promql/comparer"
	"github.com/prometheus/compliance/promql/config"
	"github.com/prometheus/compliance/promql/testcases"
)

//go:embed promql-test-queries.yml
var rawTestQueries []byte

// ComplianceTests returns the official PromQL compliance test cases.
func ComplianceTests() []*config.TestCase {
	cfg, err := config.Load(rawTestQueries)
	if err != nil {
		panic(fmt.Sprintf("failed to load embedded promql-test-queries.yml: %v", err))
	}
	return cfg.TestCases
}

// RFCLevel represents the compliance requirement level of a test case.
type RFCLevel string

const (
	MustLevel        RFCLevel = "MUST"
	ShouldLevel      RFCLevel = "SHOULD"
	MayLevel         RFCLevel = "MAY"
	RecommendedLevel RFCLevel = "RECOMMENDED"
)

func (r RFCLevel) annotate(t *testing.T) {
	t.Attr("rfcLevel", string(r))
}

func descAnnotate(t *testing.T, desc string) {
	t.Attr("description", desc)
}

// RunOption customizes a RunTests execution.
type RunOption func(*runConfig)

type runConfig struct {
	queryTweaks    []*config.QueryTweak
	scrapeInterval time.Duration
}

// WithQueryTweaks configures query comparison tweaks for the target under test.
func WithQueryTweaks(tweaks ...*config.QueryTweak) RunOption {
	return func(cfg *runConfig) {
		cfg.queryTweaks = append(cfg.queryTweaks, tweaks...)
	}
}

// WithScrapeInterval overrides the scrape interval passed to targets during fast scrape replay.
func WithScrapeInterval(d time.Duration) RunOption {
	return func(cfg *runConfig) {
		cfg.scrapeInterval = d
	}
}

// RunTests ingests synthetic demo metrics into refTarget and testTarget via fast timestamped
// scrape replay and runs all expanded PromQL test cases against both targets.
func RunTests(t *testing.T, refTarget, testTarget Target, tcs []*config.TestCase, opts ...RunOption) {
	t.Helper()

	if refTarget == nil {
		t.Fatal("refTarget must not be nil")
	}
	if testTarget == nil {
		t.Fatal("testTarget must not be nil")
	}
	if len(tcs) == 0 {
		t.Fatal("tcs must not be empty")
	}

	rcfg := &runConfig{
		scrapeInterval: 15 * time.Millisecond,
	}
	for _, o := range opts {
		o(rcfg)
	}

	// Keep the synthetic window 2 minutes in the past so any post-replay scrapes
	// at wall-clock time never overlap the queried [start, end] range.
	end := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Minute)
	start := end.Add(-10 * time.Minute)
	resolution := 10 * time.Second

	snapshots := generateDemoSnapshots(end)
	refReplayer := newScrapeReplayer(snapshots)
	testReplayer := newScrapeReplayer(snapshots)

	refQueryHostPort := freeLocalHostPort(t)
	testQueryHostPort := freeLocalHostPort(t)

	ctx, cancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	t.Cleanup(func() {
		cancel()
		refReplayer.Close()
		testReplayer.Close()
		wg.Wait()
	})

	refErrCh := make(chan error, 1)
	testErrCh := make(chan error, 1)

	wg.Add(2)
	go func() {
		defer wg.Done()
		refErrCh <- refTarget.Run(ctx, Options{
			ScrapeTargetHostPort: refReplayer.HostPort(t),
			ScrapeTargetJobName:  "demo",
			ScrapeInterval:       rcfg.scrapeInterval,
			QueryHostPort:        refQueryHostPort,
		})
	}()
	go func() {
		defer wg.Done()
		testErrCh <- testTarget.Run(ctx, Options{
			ScrapeTargetHostPort: testReplayer.HostPort(t),
			ScrapeTargetJobName:  "demo",
			ScrapeInterval:       rcfg.scrapeInterval,
			QueryHostPort:        testQueryHostPort,
		})
	}()

	refAPI, err := newPromAPI("http://" + refQueryHostPort)
	if err != nil {
		t.Fatalf("failed to create reference API client: %v", err)
	}
	testAPI, err := newPromAPI("http://" + testQueryHostPort)
	if err != nil {
		t.Fatalf("failed to create test API client: %v", err)
	}

	setupOk := safeRun(t, fmt.Sprintf("%s/_setup", testTarget.Name()), testTarget.Name(), "Ingest synthetic scrape data into reference and target", MustLevel, func(t *testing.T) {
		waitCtx, waitCancel := context.WithTimeout(ctx, 2*time.Minute)
		defer waitCancel()

		if err := waitReplayerAndTarget(waitCtx, refReplayer, refAPI, end, refErrCh, refTarget.Name()); err != nil {
			t.Fatalf("reference target %q setup failed: %v", refTarget.Name(), err)
		}
		if err := waitReplayerAndTarget(waitCtx, testReplayer, testAPI, end, testErrCh, testTarget.Name()); err != nil {
			t.Fatalf("test target %q setup failed: %v", testTarget.Name(), err)
		}
	})

	comp := comparer.New(refAPI, testAPI, rcfg.queryTweaks)
	expanded := testcases.ExpandTestCases(tcs, rcfg.queryTweaks, start, end, resolution)

	for _, tc := range expanded {
		safeRun(t, fmt.Sprintf("%s/%s", testTarget.Name(), tc.Query), testTarget.Name(), tc.Query, MustLevel, func(t *testing.T) {
			t.Parallel()
			if !setupOk {
				t.Fatal("setup failed")
			}

			res, err := comp.Compare(tc)
			if err != nil {
				t.Fatalf("comparison failed: %v", err)
			}
			if res.Success() {
				return
			}
			if res.Unsupported {
				t.Skipf("query unsupported: %s", res.UnexpectedFailure)
			}
			if res.UnexpectedFailure != "" {
				t.Fatalf("query failed unexpectedly: %s", res.UnexpectedFailure)
			}
			if res.UnexpectedSuccess {
				t.Fatal("query succeeded, but should have failed")
			}
			if res.Diff != "" {
				t.Fatalf("query returned different results (-ref +test):\n%s", res.Diff)
			}
		})
	}
}

func safeRun(t *testing.T, name, target, desc string, rfc RFCLevel, f func(t *testing.T)) bool {
	return t.Run(name, func(t *testing.T) {
		t.Attr("rw", target)
		rfc.annotate(t)
		descAnnotate(t, desc)

		if os.Getenv("DEBUG") == "" {
			defer func() {
				if val := recover(); val != nil {
					t.Fatalf("test panicked: %v\n%s", val, debug.Stack())
				}
			}()
		}
		f(t)
	})
}

func newPromAPI(queryURL string) (v1.API, error) {
	cl, err := api.NewClient(api.Config{Address: queryURL})
	if err != nil {
		return nil, err
	}
	return v1.NewAPI(cl), nil
}

func freeLocalHostPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate local port: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

func waitReplayerAndTarget(ctx context.Context, replayer *scrapeReplayer, promAPI v1.API, end time.Time, errCh <-chan error, name string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for %s to scrape %d snapshots (scraped %d): %w", name, len(replayer.snapshots), replayer.ScrapedCount(), ctx.Err())
	case err := <-errCh:
		return fmt.Errorf("target %s exited prematurely while scraping: %w", name, err)
	case <-replayer.Done():
	}

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		val, _, _, err := promAPI.Query(ctx, "count(demo_num_cpus)", end)
		if err == nil {
			if vec, ok := val.(model.Vector); ok && len(vec) == 1 && vec[0].Value == 3 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timed out waiting for %s query readiness at %v (last err: %v): %w", name, end, err, ctx.Err())
		case err := <-errCh:
			return fmt.Errorf("target %s exited prematurely before query readiness: %w", name, err)
		case <-ticker.C:
		}
	}
}

const om1ContentType = "application/openmetrics-text; version=1.0.0; charset=utf-8"

// scrapeReplayer serves a sequence of pre-computed OpenMetrics 1.0 snapshots,
// advancing by one snapshot on each HTTP scrape until all snapshots have been served.
type scrapeReplayer struct {
	server    *httptest.Server
	mu        sync.Mutex
	snapshots []string
	idx       int
	doneOnce  sync.Once
	doneCh    chan struct{}
}

func newScrapeReplayer(snapshots []string) *scrapeReplayer {
	r := &scrapeReplayer{
		snapshots: snapshots,
		doneCh:    make(chan struct{}),
	}
	r.server = httptest.NewServer(http.HandlerFunc(r.handleScrape))
	return r
}

func (r *scrapeReplayer) handleScrape(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	var body string
	if r.idx < len(r.snapshots) {
		body = r.snapshots[r.idx]
		r.idx++
		if r.idx == len(r.snapshots) {
			r.doneOnce.Do(func() { close(r.doneCh) })
		}
	} else {
		body = "# EOF\n"
	}
	r.mu.Unlock()

	w.Header().Set("Content-Type", om1ContentType)
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

func (r *scrapeReplayer) HostPort(t *testing.T) string {
	t.Helper()
	u, err := url.Parse(r.server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func (r *scrapeReplayer) Done() <-chan struct{} {
	return r.doneCh
}

func (r *scrapeReplayer) ScrapedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.idx
}

func (r *scrapeReplayer) Close() {
	r.server.Close()
}

// generateDemoSnapshots builds deterministic OpenMetrics 1.0 scrape payloads covering
// [end-70m, end] for the demo_* metrics used by promql-test-queries.yml.
// TODO(bwplotka): Support Native Histograms long term (e.g. via Prometheus Protobuf or OpenMetrics 2.0 scrape payloads and NH test queries in promql-test-queries.yml).
func generateDemoSnapshots(end time.Time) []string {
	var timestamps []time.Time
	// Coarse 2m steps over [end-70m, end-12m) to populate 1h range lookups and 10m offsets.
	for ts := end.Add(-70 * time.Minute); ts.Before(end.Add(-12 * time.Minute)); ts = ts.Add(2 * time.Minute) {
		timestamps = append(timestamps, ts)
	}
	// Fine 5s steps over [end-12m, end] matching prometheus-test-data.yml scrape_interval.
	for ts := end.Add(-12 * time.Minute); !ts.After(end); ts = ts.Add(5 * time.Second) {
		timestamps = append(timestamps, ts)
	}

	instances := []string{
		"demo.promlabs.com:10000",
		"demo.promlabs.com:10001",
		"demo.promlabs.com:10002",
	}
	memTypes := []string{"free", "buffers", "cached"}
	cpuModes := []string{"idle", "user"}
	methods := []string{"GET", "POST"}
	bucketBounds := []string{"0.05", "0.1", "0.2", "0.4", "0.8", "+Inf"}

	resetTime := end.Add(-4 * time.Minute)
	baseTime := timestamps[0]

	snapshots := make([]string, 0, len(timestamps))
	var sb strings.Builder

	for stepIdx, ts := range timestamps {
		sb.Reset()
		tsSec := ts.Unix()
		elapsedSec := ts.Sub(baseTime).Seconds()

		// 1. demo_num_cpus (gauge).
		sb.WriteString("# TYPE demo_num_cpus gauge\n")
		for _, inst := range instances {
			fmt.Fprintf(&sb, "demo_num_cpus{instance=%q} 4 %d\n", inst, tsSec)
		}

		// 2. demo_memory_usage_bytes (gauge).
		sb.WriteString("# TYPE demo_memory_usage_bytes gauge\n")
		for instIdx, inst := range instances {
			for typeIdx, mType := range memTypes {
				val := 1e8*float64(instIdx+1) + 2e7*float64(typeIdx+1) + 1e6*math.Sin(float64(stepIdx)/5.0+float64(instIdx+typeIdx))
				fmt.Fprintf(&sb, "demo_memory_usage_bytes{instance=%q,type=%q} %.4f %d\n", inst, mType, val, tsSec)
			}
		}

		// 3. demo_cpu_usage_seconds_total (counter with a reset at end-4m).
		sb.WriteString("# TYPE demo_cpu_usage_seconds counter\n")
		counterElapsed := elapsedSec
		if !ts.Before(resetTime) {
			counterElapsed = ts.Sub(resetTime).Seconds()
		}
		for instIdx, inst := range instances {
			for modeIdx, mode := range cpuModes {
				ratePerSec := 0.25*float64(instIdx+1) + 0.15*float64(modeIdx+1)
				val := 10.0 + counterElapsed*ratePerSec
				fmt.Fprintf(&sb, "demo_cpu_usage_seconds_total{instance=%q,mode=%q} %.4f %d\n", inst, mode, val, tsSec)
			}
		}

		// 4. demo_disk_usage_bytes (gauge with linear trend for deriv/predict_linear).
		sb.WriteString("# TYPE demo_disk_usage_bytes gauge\n")
		for instIdx, inst := range instances {
			val := 5e9*float64(instIdx+1) + elapsedSec*1024.0*float64(instIdx+1)
			fmt.Fprintf(&sb, "demo_disk_usage_bytes{instance=%q} %.2f %d\n", inst, val, tsSec)
		}

		// 5. demo_batch_last_success_timestamp_seconds (gauge stepping every minute).
		sb.WriteString("# TYPE demo_batch_last_success_timestamp_seconds gauge\n")
		batchTS := ts.Truncate(time.Minute).Unix()
		for instIdx, inst := range instances {
			fmt.Fprintf(&sb, "demo_batch_last_success_timestamp_seconds{instance=%q} %d %d\n", inst, batchTS-int64(instIdx*10), tsSec)
		}

		// 6. demo_intermittent_metric (gauge with a >5m gap so lookback staleness triggers).
		if ts.Before(end.Add(-8*time.Minute)) || !ts.Before(end.Add(-2*time.Minute)) {
			sb.WriteString("# TYPE demo_intermittent_metric gauge\n")
			for instIdx, inst := range instances {
				fmt.Fprintf(&sb, "demo_intermittent_metric{instance=%q} %d %d\n", inst, instIdx+1, tsSec)
			}
		}

		// 7. demo_api_request_duration_seconds (classic histogram).
		sb.WriteString("# TYPE demo_api_request_duration_seconds histogram\n")
		for instIdx, inst := range instances {
			for methodIdx, method := range methods {
				baseCount := uint64(stepIdx+1) * uint64(instIdx+1) * uint64(methodIdx+1)
				var totalCount uint64
				for bIdx, le := range bucketBounds {
					totalCount = baseCount * uint64(bIdx+1)
					fmt.Fprintf(&sb, "demo_api_request_duration_seconds_bucket{instance=%q,method=%q,le=%q} %d %d\n", inst, method, le, totalCount, tsSec)
				}
				sum := float64(totalCount) * 0.125
				fmt.Fprintf(&sb, "demo_api_request_duration_seconds_sum{instance=%q,method=%q} %.4f %d\n", inst, method, sum, tsSec)
				fmt.Fprintf(&sb, "demo_api_request_duration_seconds_count{instance=%q,method=%q} %d %d\n", inst, method, totalCount, tsSec)
			}
		}

		sb.WriteString("# EOF\n")
		snapshots = append(snapshots, sb.String())
	}

	return snapshots
}

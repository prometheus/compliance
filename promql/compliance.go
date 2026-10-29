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
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/compliance"
	"github.com/prometheus/compliance/promql/comparer"
	"github.com/prometheus/compliance/promql/config"
	"github.com/prometheus/compliance/promql/testcases"
	"google.golang.org/protobuf/proto"
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
type RFCLevel = compliance.RFCLevel

const (
	MustLevel        = compliance.MustLevel
	ShouldLevel      = compliance.ShouldLevel
	MayLevel         = compliance.MayLevel
	RecommendedLevel = compliance.RecommendedLevel
)

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
	refReplayer := compliance.NewScrapeServer(compliance.ScrapeServerOpts{Payloads: snapshots})
	testReplayer := compliance.NewScrapeServer(compliance.ScrapeServerOpts{Payloads: snapshots})

	refQueryHostPort := compliance.FreeLocalHostPort(t)
	testQueryHostPort := compliance.FreeLocalHostPort(t)

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

	setupOk := compliance.SafeRun(t, fmt.Sprintf("%s/_setup", testTarget.Name()), testTarget.Name(), "Ingest synthetic scrape data into reference and target", MustLevel, func(t *testing.T) {
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
		compliance.SafeRun(t, fmt.Sprintf("%s/%s", testTarget.Name(), tc.Query), testTarget.Name(), tc.Query, MustLevel, func(t *testing.T) {
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

func newPromAPI(queryURL string) (v1.API, error) {
	cl, err := api.NewClient(api.Config{Address: queryURL})
	if err != nil {
		return nil, err
	}
	return v1.NewAPI(cl), nil
}

func waitReplayerAndTarget(ctx context.Context, replayer *compliance.ScrapeServer, promAPI v1.API, end time.Time, errCh <-chan error, name string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("timed out waiting for %s to scrape %d snapshots (scraped %d): %w", name, replayer.PayloadCount(), replayer.ScrapedCount(), ctx.Err())
	case err := <-errCh:
		return fmt.Errorf("target %s exited prematurely while scraping: %w", name, err)
	case <-replayer.Done():
	}

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		val, _, err := promAPI.Query(ctx, "count(demo_num_cpus)", end)
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

func labelPair(name, value string) *dto.LabelPair {
	return &dto.LabelPair{Name: proto.String(name), Value: proto.String(value)}
}

// generateDemoSnapshots builds deterministic PrometheusProto and OpenMetrics 1.0 scrape
// payloads covering [end-70m, end] for the demo_* metrics used by promql-test-queries.yml.
func generateDemoSnapshots(end time.Time) []compliance.ScrapePayload {
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
	bucketBounds := []float64{0.05, 0.1, 0.2, 0.4, 0.8}

	resetTime := end.Add(-4 * time.Minute)
	baseTime := timestamps[0]

	snapshots := make([]compliance.ScrapePayload, 0, len(timestamps))
	var protoBuf, omBuf bytes.Buffer
	stepsSinceReset := 0

	for stepIdx, ts := range timestamps {
		tsMs := ts.UnixMilli()
		elapsedSec := ts.Sub(baseTime).Seconds()
		stepsSinceReset++
		if ts.Equal(resetTime) {
			stepsSinceReset = 1
		}

		// 1. demo_num_cpus (gauge).
		numCPUs := &dto.MetricFamily{
			Name: proto.String("demo_num_cpus"),
			Type: dto.MetricType_GAUGE.Enum(),
		}
		for _, inst := range instances {
			numCPUs.Metric = append(numCPUs.Metric, &dto.Metric{
				Label:       []*dto.LabelPair{labelPair("instance", inst)},
				Gauge:       &dto.Gauge{Value: proto.Float64(4)},
				TimestampMs: proto.Int64(tsMs),
			})
		}

		// 2. demo_memory_usage_bytes (gauge).
		memUsage := &dto.MetricFamily{
			Name: proto.String("demo_memory_usage_bytes"),
			Type: dto.MetricType_GAUGE.Enum(),
		}
		for instIdx, inst := range instances {
			for typeIdx, mType := range memTypes {
				val := 1e8*float64(instIdx+1) + 2e7*float64(typeIdx+1) + 1e6*math.Sin(float64(stepIdx)/5.0+float64(instIdx+typeIdx))
				memUsage.Metric = append(memUsage.Metric, &dto.Metric{
					Label:       []*dto.LabelPair{labelPair("instance", inst), labelPair("type", mType)},
					Gauge:       &dto.Gauge{Value: proto.Float64(val)},
					TimestampMs: proto.Int64(tsMs),
				})
			}
		}

		// 3. demo_cpu_usage_seconds_total (counter with a reset at end-4m).
		cpuUsage := &dto.MetricFamily{
			Name: proto.String("demo_cpu_usage_seconds_total"),
			Type: dto.MetricType_COUNTER.Enum(),
		}
		counterElapsed := elapsedSec
		if !ts.Before(resetTime) {
			counterElapsed = ts.Sub(resetTime).Seconds()
		}
		for instIdx, inst := range instances {
			for modeIdx, mode := range cpuModes {
				ratePerSec := 0.25*float64(instIdx+1) + 0.15*float64(modeIdx+1)
				val := 10.0 + counterElapsed*ratePerSec
				cpuUsage.Metric = append(cpuUsage.Metric, &dto.Metric{
					Label:       []*dto.LabelPair{labelPair("instance", inst), labelPair("mode", mode)},
					Counter:     &dto.Counter{Value: proto.Float64(val)},
					TimestampMs: proto.Int64(tsMs),
				})
			}
		}

		// 4. demo_disk_usage_bytes (gauge with linear trend for deriv/predict_linear).
		diskUsage := &dto.MetricFamily{
			Name: proto.String("demo_disk_usage_bytes"),
			Type: dto.MetricType_GAUGE.Enum(),
		}
		for instIdx, inst := range instances {
			val := 5e9*float64(instIdx+1) + elapsedSec*1024.0*float64(instIdx+1)
			diskUsage.Metric = append(diskUsage.Metric, &dto.Metric{
				Label:       []*dto.LabelPair{labelPair("instance", inst)},
				Gauge:       &dto.Gauge{Value: proto.Float64(val)},
				TimestampMs: proto.Int64(tsMs),
			})
		}

		// 5. demo_batch_last_success_timestamp_seconds (gauge stepping every minute).
		batchSuccess := &dto.MetricFamily{
			Name: proto.String("demo_batch_last_success_timestamp_seconds"),
			Type: dto.MetricType_GAUGE.Enum(),
		}
		batchTS := ts.Truncate(time.Minute).Unix()
		for instIdx, inst := range instances {
			batchSuccess.Metric = append(batchSuccess.Metric, &dto.Metric{
				Label:       []*dto.LabelPair{labelPair("instance", inst)},
				Gauge:       &dto.Gauge{Value: proto.Float64(float64(batchTS - int64(instIdx*10)))},
				TimestampMs: proto.Int64(tsMs),
			})
		}

		families := []*dto.MetricFamily{
			numCPUs,
			memUsage,
			cpuUsage,
			diskUsage,
			batchSuccess,
		}

		// 6. demo_intermittent_metric (gauge with a >5m gap so lookback staleness triggers).
		if ts.Before(end.Add(-8*time.Minute)) || !ts.Before(end.Add(-2*time.Minute)) {
			intermittent := &dto.MetricFamily{
				Name: proto.String("demo_intermittent_metric"),
				Type: dto.MetricType_GAUGE.Enum(),
			}
			for instIdx, inst := range instances {
				intermittent.Metric = append(intermittent.Metric, &dto.Metric{
					Label:       []*dto.LabelPair{labelPair("instance", inst)},
					Gauge:       &dto.Gauge{Value: proto.Float64(float64(instIdx + 1))},
					TimestampMs: proto.Int64(tsMs),
				})
			}
			families = append(families, intermittent)
		}

		// 7. demo_api_request_duration_seconds (classic histogram).
		classicHist := &dto.MetricFamily{
			Name: proto.String("demo_api_request_duration_seconds"),
			Type: dto.MetricType_HISTOGRAM.Enum(),
		}
		for instIdx, inst := range instances {
			for methodIdx, method := range methods {
				baseCount := uint64(stepIdx+1) * uint64(instIdx+1) * uint64(methodIdx+1)
				buckets := make([]*dto.Bucket, 0, len(bucketBounds))
				for bIdx, ub := range bucketBounds {
					buckets = append(buckets, &dto.Bucket{
						CumulativeCount: proto.Uint64(baseCount * uint64(bIdx+1)),
						UpperBound:      proto.Float64(ub),
					})
				}
				totalCount := baseCount * uint64(len(bucketBounds)+1)
				classicHist.Metric = append(classicHist.Metric, &dto.Metric{
					Label: []*dto.LabelPair{labelPair("instance", inst), labelPair("method", method)},
					Histogram: &dto.Histogram{
						SampleCount: proto.Uint64(totalCount),
						SampleSum:   proto.Float64(float64(totalCount) * 0.125),
						Bucket:      buckets,
					},
					TimestampMs: proto.Int64(tsMs),
				})
			}
		}
		families = append(families, classicHist)

		// 8. demo_api_request_duration_native_seconds (native histogram with a counter reset at end-4m).
		nativeHist := &dto.MetricFamily{
			Name: proto.String("demo_api_request_duration_native_seconds"),
			Type: dto.MetricType_HISTOGRAM.Enum(),
		}
		for instIdx, inst := range instances {
			for methodIdx, method := range methods {
				baseCount := int64(stepsSinceReset) * int64(instIdx+1) * int64(methodIdx+1)
				totalCount := uint64(11 * baseCount)
				nativeHist.Metric = append(nativeHist.Metric, &dto.Metric{
					Label: []*dto.LabelPair{labelPair("instance", inst), labelPair("method", method)},
					Histogram: &dto.Histogram{
						SampleCount:   proto.Uint64(totalCount),
						SampleSum:     proto.Float64(float64(baseCount) * 2.875),
						Schema:        proto.Int32(0),
						ZeroThreshold: proto.Float64(0.001),
						ZeroCount:     proto.Uint64(uint64(baseCount)),
						PositiveSpan: []*dto.BucketSpan{
							{Offset: proto.Int32(-4), Length: proto.Uint32(4)},
						},
						PositiveDelta: []int64{2 * baseCount, 2 * baseCount, -baseCount, -2 * baseCount},
					},
					TimestampMs: proto.Int64(tsMs),
				})
			}
		}

		protoBuf.Reset()
		omBuf.Reset()
		protoEnc := expfmt.NewEncoder(&protoBuf, expfmt.NewFormat(expfmt.TypeProtoDelim))
		omEnc := expfmt.NewEncoder(&omBuf, expfmt.NewFormat(expfmt.TypeOpenMetrics))
		for _, mf := range families {
			if err := protoEnc.Encode(mf); err != nil {
				panic(err)
			}
			if err := omEnc.Encode(mf); err != nil {
				panic(err)
			}
		}
		if err := protoEnc.Encode(nativeHist); err != nil {
			panic(err)
		}
		if err := omEnc.(expfmt.Closer).Close(); err != nil {
			panic(err)
		}

		snapshots = append(snapshots, compliance.ScrapePayload{
			Proto: bytes.Clone(protoBuf.Bytes()),
			Text:  omBuf.String(),
		})
	}

	return snapshots
}

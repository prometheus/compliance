//go:build compliance
// +build compliance

package sender

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/compliance/remotewrite/sender/cases"
	v2 "github.com/prometheus/compliance/remotewrite/sender/cases/v2"
	"github.com/prometheus/compliance/remotewrite/sender/targets"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/stretchr/testify/require"
)

var (
	logger  = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	runners = map[string]targets.Target{
		"grafana":       targets.RunGrafanaAgent,
		"otelcollector": targets.RunOtelCollector,
		"prometheus":    targets.RunPrometheus,
		"telegraf":      targets.RunTelegraf,
		"vector":        targets.RunVector,
		"vmagent":       targets.RunVMAgent,
	}
	tests = []func() cases.Test{
		// Test each type.
		cases.CounterTest,
		cases.GaugeTest,
		cases.HistogramTest,
		cases.SummaryTest,

		// Test Up metrics.
		cases.UpTest,
		cases.InvalidTest,

		// Test for various labels
		cases.JobLabelTest,
		cases.InstanceLabelTest,
		cases.SortedLabelsTest,
		cases.RepeatedLabelsTest,
		cases.EmptyLabelsTest,
		cases.NameLabelTest,
		cases.HonorLabelsTest,

		// Other misc tests.
		cases.StalenessTest,
		cases.TimestampTest,
		cases.HeadersTest,
		cases.OrderingTest,
		cases.Retries500Test,
		cases.Retries400Test,

		// TODO:
		// - Test labels have valid characters.
	}

	testsV2 = []func() cases.Test{
		// V2 feature tests
		v2.ExemplarTest,
		v2.HistogramTest,
		v2.MetadataTest,
		v2.CTZeroSampleTest,
	}
)

func TestRemoteWrite(t *testing.T) {
	runTestSuite(t, tests, runTestV1)
}

func TestRemoteWriteV2(t *testing.T) {
	runTestSuite(t, testsV2, runTestV2)
}

func runTestSuite(t *testing.T, testFunctions []func() cases.Test, testRunner func(*testing.T, cases.Test, targets.Target)) {
	for name, runner := range runners {
		t.Run(name, func(t *testing.T) {
			for _, fn := range testFunctions {
				tc := fn()
				t.Run(tc.Name, func(t *testing.T) {
					t.Parallel()
					testRunner(t, tc, runner)
				})
			}
		})
	}
}

func runTestV1(t *testing.T, tc cases.Test, runner targets.Target) {
	runTest(t, tc, runner, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1})
}

func runTestV2(t *testing.T, tc cases.Test, runner targets.Target) {
	runTest(t, tc, runner, []config.RemoteWriteProtoMsg{config.RemoteWriteProtoMsgV1, config.RemoteWriteProtoMsgV2})
}

func runTest(t *testing.T, tc cases.Test, runner targets.Target, protocols []config.RemoteWriteProtoMsg) {
	ap := cases.Appendable{}
	writeHandler := remote.NewWriteHandler(logger, nil, &ap, protocols)
	if tc.Writes != nil {
		writeHandler = tc.Writes(writeHandler)
	}

	// Start a HTTP server to expose some metrics and a receive remote write requests.
	m := http.NewServeMux()
	m.Handle("/metrics", tc.Metrics)
	m.Handle("/push", writeHandler)
	s := http.Server{
		Handler: m,
	}
	l, err := net.Listen("tcp", "localhost:")
	require.NoError(t, err)
	go s.Serve(l)
	defer s.Close()

	// Run Prometheus to scrape and send metrics.
	scrapeTarget := l.Addr().String()
	receiveEndpoint := fmt.Sprintf("http://%s/push", l.Addr().String())
	require.NoError(t, runner(targets.TargetOptions{
		ScrapeTarget:    scrapeTarget,
		ReceiveEndpoint: receiveEndpoint,
		Timeout:         10 * time.Second,
	}))

	// Check we got some data.
	tc.Expected(t, ap.Batches)
}

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
	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/compliance/remotewrite/sender/cases"
	"github.com/prometheus/compliance/remotewrite/sender/targets"
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

	// Remote Write 2.0 Protocol Tests (https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/)
	// These tests require RW 2.0 sender configuration:
	//   remote_write:
	//     - url: <endpoint>
	//       protobuf_message: "io.prometheus.write.v2.Request"
	testsV2 = cases.ProtocolTests
)

func TestRemoteWrite(t *testing.T) {
	runTestSuite(t, tests, remote.MessageTypes{
		remote.WriteV1MessageType,
	})
}

func TestRemoteWriteV2(t *testing.T) {
	runTestSuite(t, testsV2, remote.MessageTypes{
		remote.WriteV2MessageType,
	})
}

func runTestSuite(t *testing.T, testFns []func() cases.Test, acceptedVersions remote.MessageTypes) {
	for name, runner := range runners {
		t.Run(name, func(t *testing.T) {
			for _, fn := range testFns {
				tc := fn()
				t.Run(tc.Name, func(t *testing.T) {
					t.Parallel()
					runTest(t, tc, runner, acceptedVersions)
				})
			}
		})
	}
}

func runTest(t *testing.T, tc cases.Test, runner targets.Target, acceptedVersions remote.MessageTypes) {
	collector := cases.SampleCollector{}

	writeHandler := remote.NewWriteHandler(&collector, acceptedVersions)
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
		UseRW2Protocol:  acceptedVersions[0] == remote.WriteV2MessageType,
	}))

	// Check we got some data.
	tc.Expected(t, collector.Batches)
}

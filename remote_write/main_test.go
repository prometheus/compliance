// +build compliance

package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/compliance/remote_write/cases"
	"github.com/prometheus/compliance/remote_write/targets"
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
		"vmagent":       targets.RunVMAgent,
	}
	tests = []func() cases.Test{
		cases.CounterTest,
		cases.GaugeTest,
		cases.HistogramTest,

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

		// Other misc tests.
		cases.StalenessTest,
		cases.TimestampTest,

		// TODO:
		// - Test for ordering correctness.
		// - Test for correct headers.
		// - Test labels have valid characters.
	}
)

func TestRemoteWrite(t *testing.T) {
	for name, runner := range runners {
		t.Run(name, func(t *testing.T) {
			for _, fn := range tests {
				tc := fn()
				t.Run(tc.Name, func(t *testing.T) {
					t.Parallel()
					runTest(t, tc, runner)
				})
			}
		})
	}
}

func runTest(t *testing.T, tc cases.Test, runner targets.Target) {
	ap := cases.Appendable{}

	// Start a HTTP server to expose some metrics and a receive remote write requests.
	m := http.NewServeMux()
	m.Handle("/metrics", tc.Metrics)
	m.Handle("/push", remote.NewWriteHandler(logger, &ap))
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

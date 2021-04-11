package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/remote-write-compliance/cases"
	"github.com/prometheus/remote-write-compliance/targets"
	"github.com/stretchr/testify/require"
)

var (
	logger  = log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	runners = map[string]targets.Target{
		"prometheus":    targets.RunPrometheus,
		"otelcollector": targets.RunOtelCollector,
		"telegraf":      targets.RunTelegraf,
		"grafana":       targets.RunGrafanaAgent,
		//"vmagent":       targets.RunVMAgent, // No download for Mac yet.
	}
	tests = []func() cases.Test{
		cases.GaugeTest,
		cases.UpTest,
		cases.InvalidTest,
		cases.StalenessTest,
		cases.HistogramTest,
		cases.SortedLabelsTest,
		cases.JobLabelTest,
		cases.RepeatedLabelsTest,
		cases.EmptyLabelsTest,
		cases.NameLabelTest,
		cases.TimestampTest,
		// TODO:
		// - Test for instance label.
		// - Test for ordering correctness.
		// - Test for timestamps being reasonable.
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

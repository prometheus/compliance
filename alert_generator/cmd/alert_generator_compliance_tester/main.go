package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-kit/log/level"
	"github.com/prometheus/common/promlog"

	"github.com/prometheus/compliance/alert_generator/cases"
	"github.com/prometheus/compliance/alert_generator/testsuite"
)

func main() {
	// TODO: take auth credentials via config.
	remoteWriteURL := flag.String("remote-write-url", "http://localhost:9090/api/v1/write", "URL for remote writing samples.")
	baseURL := flag.String("api-base-url", "http://localhost:9090", "Base URL including any prefix to request GET <base-url>/api/v1/rules and GET <base-url>/api/v1/alerts.")
	promQLBaseURL := flag.String("promql-base-url", "http://localhost:9090", "URL where the test suite can access the time series data via PromQL including any prefix to request GET <promql-base-url>/api/v1/query and GET <promql-base-url>/api/v1/query_range.")
	alertServerPort := flag.String("alert-server-port", "8080", "Port to run a server for accepting alerts.")
	flag.Parse()

	log := promlog.New(&promlog.Config{})

	t, err := testsuite.NewTestSuite(testsuite.TestSuiteOptions{
		Logger:           log,
		Cases:            cases.AllCases,
		RemoteWriteURL:   *remoteWriteURL,
		BaseAlertsAPIURL: *baseURL,
		PromQLBaseURL:    *promQLBaseURL,
		AlertServerPort:  *alertServerPort,
	})
	if err != nil {
		level.Error(log).Log("msg", "Failed to create the test suite instance", "err", err)
		os.Exit(1)
	}

	level.Info(log).Log("msg", "Starting the test suite")

	t.Start()
	t.Wait()

	if err := t.Error(); err != nil {
		level.Error(log).Log("msg", "Some error in the test suite", "err", err)
		os.Exit(1)
	}

	yes, describe := t.WasTestSuccessful()
	exitCode := 0
	stream := os.Stdout
	if !yes {
		exitCode = 1
		stream = os.Stderr
	}
	// TODO: gracefully shutdown on Ctrl+C and show any errors occured till now.
	fmt.Fprintln(stream, describe)
	os.Exit(exitCode)
}

package main

import (
	"flag"
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
	flag.Parse()

	log := promlog.New(&promlog.Config{})

	m, err := testsuite.NewManager(testsuite.ManagerOptions{
		Logger:         log,
		Cases:          cases.AllCases,
		RemoteWriteURL: *remoteWriteURL,
		BaseApiURL:     *baseURL,
		PromQLBaseURL:  *promQLBaseURL,
	})
	if err != nil {
		level.Error(log).Log("msg", "Failed to create the test suite instance", "err", err)
		os.Exit(1)
	}

	level.Info(log).Log("msg", "Starting the test suite", "url", *remoteWriteURL)

	m.Start()
	m.Wait()

	if err := m.Error(); err != nil {
		level.Error(log).Log("msg", "Some error in the test suite", "err", err)
		os.Exit(1)
	}
}

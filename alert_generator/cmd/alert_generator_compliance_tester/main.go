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
	remoteWriteURL := flag.String("remote-write-url", "http://localhost:9090/api/v1/write", "URL for remote write.")
	//apiBaseURL := flag.String("api-base-url", "http://localhost:9090", "Base URL including any prefix to request GET <base>/api/v1/rules and GET <base>/api/v1/alerts.")
	flag.Parse()

	log := promlog.New(&promlog.Config{})

	// TODO: Make remote writing testsuite.Manager's task.
	remoteWriter, err := testsuite.NewRemoteWriter(*remoteWriteURL)
	if err != nil {
		level.Error(log).Log("msg", "Failed to create the remote writer", "err", err)
		os.Exit(1)
	}

	for _, c := range cases.AllCases {
		remoteWriter.AddTimeSeries(c.SamplesToRemoteWrite())
	}

	level.Info(log).Log("msg", "Starting to remote write", "url", *remoteWriteURL)

	remoteWriter.Start()

	remoteWriter.Wait()
	select {
	case err := <-remoteWriter.Error():
		if err != nil {
			level.Error(log).Log("msg", "Some error in the remote writer", "err", err)
			os.Exit(1)
		}
	default:
	}
}

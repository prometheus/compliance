package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/go-kit/log/level"
	"github.com/prometheus/common/promlog"

	"github.com/prometheus/compliance/alert_generator"
	"github.com/prometheus/compliance/alert_generator/cases"
	"github.com/prometheus/compliance/alert_generator/config"
)

func main() {
	// TODO: give option to set log level.
	configFile := flag.String("config-file", "config.yaml", "Path to the config file.")

	flag.Parse()
	log := promlog.New(&promlog.Config{})

	cfg, err := config.LoadFromFile(*configFile)
	if err != nil {
		level.Error(log).Log("msg", "Failed to load config file", "err", err)
		os.Exit(1)
	}

	casesToRun := cases.AllCases()
	if len(cfg.TestCases) > 0 {
		casesToRun = []cases.TestCase{}
		for _, cn := range cfg.TestCases {
			tc, ok := cases.AllCasesMap[cn]
			if !ok {
				level.Error(log).Log("msg", "Test case not found", "test_case", cn)
				os.Exit(1)
			}
			casesToRun = append(casesToRun, tc)
		}
	}

	if cfg.Settings.AlertMessageParser == "" {
		cfg.Settings.AlertMessageParser = "default"
	}

	alertMessageParser, ok := testsuite.AlertMessageParsers[cfg.Settings.AlertMessageParser]
	if !ok {
		level.Error(log).Log("msg", "Alert message parser not found", "name", cfg.Settings.AlertMessageParser)
		os.Exit(1)
	}

	t, err := testsuite.NewTestSuite(testsuite.TestSuiteOptions{
		Logger:             log,
		Cases:              casesToRun,
		Config:             *cfg,
		AlertMessageParser: alertMessageParser,
	})
	if err != nil {
		level.Error(log).Log("msg", "Failed to start the test suite", "err", err)
		os.Exit(1)
	}

	level.Info(log).Log("msg", "Starting the test suite")
	t.Start()

	tu := t.TestUntil()
	level.Info(log).Log("msg", fmt.Sprintf("Test will run until %s approximately", tu.Format(time.RFC3339)), "time_remaining", time.Until(tu))

	var wg sync.WaitGroup
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	interrupted := false
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range c {
			level.Info(log).Log("msg", "Received SIGINT, stopping the test")
			interrupted = true
			t.Stop()
			return
		}
	}()

	t.Wait()
	close(c)
	wg.Wait()

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
	} else if interrupted {
		exitCode = 1
		stream = os.Stderr
		describe = "Test was incomplete"
	}

	if len(casesToRun) != len(cases.AllCases()) {
		describe += "\n\n**NOTE: Not all test cases were run**"
	}

	fmt.Fprintln(stream, describe)
	os.Exit(exitCode)
}

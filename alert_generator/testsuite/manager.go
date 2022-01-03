package testsuite

import (
	"fmt"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/pkg/timestamp"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"

	"github.com/prometheus/compliance/alert_generator/cases"
)

// TestSuite runs the entire test suite from start to end.
type TestSuite struct {
	opts                            TestSuiteOptions
	baseAlertsAPIURL, promqlBaseURL *url.URL

	remoteWriter         *RemoteWriter
	remoteWriteStartTime time.Time

	ruleGroupTestsMtx   sync.RWMutex
	ruleGroupTests      map[string]cases.TestCase // Group name -> TestCase.
	ruleGroupTestErrors map[string][]error        // Group name -> slice of errors in them.

	minGroupInterval model.Duration

	stopc chan struct{}
	wg    sync.WaitGroup
}

type TestSuiteOptions struct {
	Logger log.Logger
	// All the test cases to test.
	Cases []cases.TestCase
	// RemoteWriteURL is URL to remote write samples.
	RemoteWriteURL string
	// BaseAlertsAPIURL is the URL to query the GET <BaseApiURL>/api/v1/rules and <BaseApiURL>/api/v1/alerts.
	BaseAlertsAPIURL string
	// PromQLBaseURL is the URL to query the database via PromQL via GET <PromQLBaseURL>/query and <PromQLBaseURL>/query_range.
	PromQLBaseURL string
}

func NewTestSuite(opts TestSuiteOptions) (*TestSuite, error) {
	err := validateOpts(opts)
	if err != nil {
		return nil, errors.Wrap(err, "validate options")
	}

	m := &TestSuite{
		opts:                opts,
		ruleGroupTests:      make(map[string]cases.TestCase, len(opts.Cases)),
		ruleGroupTestErrors: make(map[string][]error),
		stopc:               make(chan struct{}),
	}

	m.remoteWriter, err = NewRemoteWriter(opts.RemoteWriteURL, opts.Logger)
	if err != nil {
		return nil, errors.Wrap(err, "create remote writer")
	}

	for i, c := range opts.Cases {
		m.remoteWriter.AddTimeSeries(c.SamplesToRemoteWrite())
		groupName, _ := c.Describe()
		m.ruleGroupTests[groupName] = c

		rg, err := c.RuleGroup()
		if err != nil {
			return nil, err
		}
		if i == 0 || rg.Interval < m.minGroupInterval {
			m.minGroupInterval = rg.Interval
		}
	}

	{
		u, err := url.Parse(m.opts.BaseAlertsAPIURL)
		if err != nil {
			return nil, err
		}
		u.Path = path.Join(u.Path, "/api/v1/alerts")
		m.baseAlertsAPIURL = u
	}

	{
		u, err := url.Parse(opts.PromQLBaseURL)
		if err != nil {
			return nil, err
		}
		u.Path = path.Join(u.Path, "/api/v1/query")
		m.promqlBaseURL = u
	}

	return m, nil
}

// minConfiguredGroupInterval is the minimum group interval for any rule.
// The API/PromQL check interval is based on the group interval per rule.
// Hence, we have a minimum to keep that interval not so small.
const minConfiguredGroupInterval = model.Duration(0 * time.Second)

// TODO(codesome): verify the validation.
func validateOpts(opts TestSuiteOptions) error {
	if opts.RemoteWriteURL == "" {
		return fmt.Errorf("no remote write URL found")
	}
	if opts.BaseAlertsAPIURL == "" {
		return fmt.Errorf("no API URL found")
	}
	if opts.PromQLBaseURL == "" {
		return fmt.Errorf("no PromQL URL found")
	}

	seenRuleGroups := make(map[string]bool)
	seenAlertNames := make(map[string]bool)

	for _, c := range opts.Cases {
		rg, err := c.RuleGroup()
		if err != nil {
			return err
		}
		if rg.Interval < minConfiguredGroupInterval {
			return fmt.Errorf("group interval too small for the group %q, min is %s, got %s", rg.Name, minConfiguredGroupInterval.String(), rg.Interval.String())
		}
		if len(rg.Rules) == 0 {
			return fmt.Errorf("group %q has 0 rules, need at least 1", rg.Name)
		}
		if rg.Name == "" {
			return fmt.Errorf("group name cannot be empty")
		}
		if seenRuleGroups[rg.Name] {
			return fmt.Errorf("group name cannot repeat, %q has been used more than once", rg.Name)
		}
		seenRuleGroups[rg.Name] = true

		merr := tsdb_errors.NewMulti()
		for i, r := range rg.Rules {
			if r.Alert.Value == "" {
				return fmt.Errorf("alert name cannot be empty, %q group has one empty", rg.Name)
			}
			if seenAlertNames[r.Alert.Value] {
				return fmt.Errorf("alert name cannot repeat to make testing easy, %q has been used more than once", r.Alert.Value)
			}
			seenAlertNames[r.Alert.Value] = true

			if r.Labels["rulegroup"] != rg.Name {
				return fmt.Errorf(`alerting rule (with alert name %q) does not have rulegroup="<groupName>" label`, r.Alert.Value)
			}

			for _, node := range rg.Rules[i].Validate() {
				merr.Add(&rulefmt.Error{
					Group:    rg.Name,
					Rule:     i + 1,
					RuleName: r.Alert.Value,
					Err:      node,
				})
			}

			if merr.Err() != nil {
				return merr.Err()
			}
		}
		if merr.Err() != nil {
			return merr.Err()
		}
	}

	return nil
}

func (ts *TestSuite) Start() {
	level.Info(ts.opts.Logger).Log("msg", "Starting the remote writer", "url", ts.opts.RemoteWriteURL)
	ts.remoteWriteStartTime = ts.remoteWriter.Start()
	for _, c := range ts.ruleGroupTests {
		c.Init(timestamp.FromTime(ts.remoteWriteStartTime))
		gn, desc := c.Describe()
		level.Info(ts.opts.Logger).Log("msg", "Starting test for a rule group", "rulegroup", gn, "description", desc)
	}

	ts.wg.Add(2)
	go ts.checkAlertsLoop()
	go ts.checkMetricsLoop()
}

func (ts *TestSuite) checkAlertsLoop() {
	defer ts.wg.Done()
	defer ts.Stop()

Loop:
	for !ts.isOver() {
		select {
		case <-ts.stopc:
			return
		case <-time.After(time.Duration(ts.minGroupInterval)):
			nowTs := timestamp.FromTime(time.Now())

			u := ts.baseAlertsAPIURL
			b, err := DoGetRequest(u.String())
			if err != nil {
				level.Error(ts.opts.Logger).Log("msg", "Error in fetching alerts", "url", u.String(), "err", err)
				continue Loop
			}

			mappedAlerts, err := ParseAndGroupAlerts(b)
			if err != nil {
				level.Error(ts.opts.Logger).Log("msg", "Error in parsing alerts response", "url", u.String(), "err", err)
				continue Loop
			}

			groupsToRemove := make(map[string]error)
			ts.ruleGroupTestsMtx.RLock()
			for groupName, c := range ts.ruleGroupTests {
				if c.TestUntil() < nowTs {
					groupsToRemove[groupName] = nil
					continue
				}
				err := c.CheckAlerts(nowTs, mappedAlerts[groupName])
				if err != nil {
					groupsToRemove[groupName] = err
				}
			}
			ts.ruleGroupTestsMtx.RUnlock()

			ts.removeGroups(groupsToRemove)
		}
	}
}

func (ts *TestSuite) checkMetricsLoop() {
	defer ts.wg.Done()
	defer ts.Stop()

Loop:
	for !ts.isOver() {
		select {
		case <-ts.stopc:
			return
		case <-time.After(time.Duration(ts.minGroupInterval)):
			nowTs := timestamp.FromTime(time.Now())

			u := ts.promqlBaseURL
			q := u.Query()
			q.Set("query", "ALERTS")
			q.Set("time", timestamp.Time(nowTs).Format(time.RFC3339))
			u.RawQuery = q.Encode()

			b, err := DoGetRequest(u.String())
			if err != nil {
				level.Error(ts.opts.Logger).Log("msg", "Error in fetching metrics", "url", u.String(), "err", err)
				continue Loop
			}

			mappedMetrics, err := ParseAndGroupMetrics(b)
			if err != nil {
				level.Error(ts.opts.Logger).Log("msg", "Error in parsing metrics response", "url", u.String(), "err", err)
				continue Loop
			}

			groupsToRemove := make(map[string]error)
			ts.ruleGroupTestsMtx.RLock()
			for groupName, c := range ts.ruleGroupTests {
				if c.TestUntil() < nowTs {
					groupsToRemove[groupName] = nil
					continue
				}
				err := c.CheckMetrics(nowTs, mappedMetrics[groupName])
				if err != nil {
					groupsToRemove[groupName] = err
				}
			}
			ts.ruleGroupTestsMtx.RUnlock()

			ts.removeGroups(groupsToRemove)
		}
	}
}

func (ts *TestSuite) removeGroups(groupsToRemove map[string]error) {
	ts.ruleGroupTestsMtx.Lock()
	defer ts.ruleGroupTestsMtx.Unlock()
	for gn, err := range groupsToRemove {
		delete(ts.ruleGroupTests, gn)
		if err != nil {
			ts.ruleGroupTestErrors[gn] = append(ts.ruleGroupTestErrors[gn], err)
			level.Error(ts.opts.Logger).Log("msg", "Test failed for a rule group", "rulegroup", gn, "err", err)
		} else {
			level.Info(ts.opts.Logger).Log("msg", "Test finished successfully for a rule group", "rulegroup", gn)
		}
	}
}

func (ts *TestSuite) isOver() bool {
	ts.ruleGroupTestsMtx.RLock()
	defer ts.ruleGroupTestsMtx.RUnlock()
	return len(ts.ruleGroupTests) == 0
}

func (ts *TestSuite) Stop() {
	select {
	case <-ts.stopc:
		// Already stopped.
	default:
		ts.remoteWriter.Stop()
		close(ts.stopc)
	}
}

func (ts *TestSuite) Wait() {
	ts.remoteWriter.Wait()
	ts.wg.Wait()
}

// Error() returns any error occured during execution of test and does
// not tell if the tests passed or failed.
func (ts *TestSuite) Error() error {
	merr := tsdb_errors.NewMulti()
	merr.Add(errors.Wrap(ts.remoteWriter.Error(), "remote writer"))
	return merr.Err()
}

// WasTestSuccessful tells if all the tests passed.
// It returns an explanation if any test failed.
// Before calling this method:
// 	* Error() should be checked for no errors.
//  * The test should have finished (i.e. Wait() is not blocking).
func (ts *TestSuite) WasTestSuccessful() (yes bool, describe string) {
	select {
	case <-ts.stopc:
	default:
		return false, "test is still running"
	}

	if err := ts.Error(); err != nil {
		return false, fmt.Sprintf("got some error in Error(): %q", err.Error())
	}

	if len(ts.ruleGroupTestErrors) == 0 {
		return true, "Congrats! All tests passed"
	}

	describe = "------------------------------------------\n"
	describe += "The following rule groups failed the test:\n"
	for gn, errs := range ts.ruleGroupTestErrors {
		describe += "\nGroup Name: " + gn + "\n"
		for i, err := range errs {
			describe += fmt.Sprintf("\tError %d: %s\n", i+1, err.Error())
		}
	}

	return false, describe
}

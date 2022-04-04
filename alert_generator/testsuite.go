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
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/model/timestamp"

	"github.com/prometheus/compliance/alert_generator/cases"
	"github.com/prometheus/compliance/alert_generator/config"
)

// TestSuite runs the entire test suite from start to end.
type TestSuite struct {
	logger                    log.Logger
	opts                      TestSuiteOptions
	alertsAPIURL, rulesAPIURL string
	promqlURL                 *url.URL

	remoteWriter         *RemoteWriter
	remoteWriteStartTime time.Time

	as *alertsServer

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

	Config config.Config

	AlertMessageParser AlertMessageParser
}

func NewTestSuite(opts TestSuiteOptions) (*TestSuite, error) {
	err := validateOpts(opts)
	if err != nil {
		return nil, errors.Wrap(err, "validate options")
	}

	m := &TestSuite{
		logger:              log.With(opts.Logger, "component", "testsuite"),
		opts:                opts,
		ruleGroupTests:      make(map[string]cases.TestCase, len(opts.Cases)),
		ruleGroupTestErrors: make(map[string][]error),
		stopc:               make(chan struct{}),
		as:                  newAlertsServer(opts.Config.Settings.AlertReceptionServerPort, opts.Config.Settings.DisableAlertsReceptionCheck, opts.Logger, opts.AlertMessageParser),
	}

	m.remoteWriter, err = NewRemoteWriter(opts.Config, opts.Logger)
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
		u, err := url.Parse(opts.Config.Settings.RulesAndAlertsAPIBaseURL)
		if err != nil {
			return nil, err
		}
		orgPath := u.Path
		u.Path = path.Join(orgPath, "/api/v1/alerts")
		m.alertsAPIURL = u.String()
		u.Path = path.Join(orgPath, "/api/v1/rules")
		m.rulesAPIURL = u.String()
	}

	{
		u, err := url.Parse(opts.Config.Settings.QueryBaseURL)
		if err != nil {
			return nil, err
		}
		u.Path = path.Join(u.Path, "/api/v1/query")
		m.promqlURL = u
	}

	return m, nil
}

// minConfiguredGroupInterval is the minimum group interval for any rule.
// The API/PromQL check interval is based on the group interval per rule.
// Hence, we have a minimum to keep that interval not so small.
// TODO: set this.
const minConfiguredGroupInterval = model.Duration(0 * time.Second)

// TODO(codesome): verify the validation.
func validateOpts(opts TestSuiteOptions) error {
	if opts.Config.Settings.DisableAlertsAPICheck &&
		opts.Config.Settings.DisableRulesAPICheck &&
		opts.Config.Settings.DisableAlertsMetricsCheck &&
		opts.Config.Settings.DisableAlertsReceptionCheck {
		return errors.New("all checks are disabled, at least one check should be enabled")
	}

	if opts.AlertMessageParser == nil {
		return errors.New("AlertMessageParser is not set")
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

		merr := NewMulti()
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
	level.Info(ts.logger).Log("msg", "Starting the alert receiving server", "port", ts.opts.Config.Settings.AlertReceptionServerPort)
	ts.as.Start()

	level.Info(ts.logger).Log("msg", "Starting the remote writer", "url", ts.opts.Config.Settings.RemoteWriteURL)
	ts.remoteWriteStartTime = ts.remoteWriter.Start()
	for _, c := range ts.ruleGroupTests {
		gn, desc := c.Describe()
		level.Info(ts.logger).Log("msg", "Starting test for a rule group", "rulegroup", gn, "description", desc)

		c.Init(timestamp.FromTime(ts.remoteWriteStartTime))
		ts.as.addExpectedAlerts(c.ExpectedAlerts()...)
	}

	time.Sleep(15 * time.Second / 2)

	if !ts.opts.Config.Settings.DisableAlertsAPICheck {
		ts.wg.Add(1)
		go ts.checkAlertsLoop()
	}
	if !ts.opts.Config.Settings.DisableRulesAPICheck {
		ts.wg.Add(1)
		go ts.checkRulesLoop()
	}
	if !ts.opts.Config.Settings.DisableAlertsMetricsCheck {
		ts.wg.Add(1)
		go ts.checkMetricsLoop()
	}
	if !ts.opts.Config.Settings.DisableAlertsReceptionCheck {
		ts.wg.Add(1)
		go ts.monitorAlertReception()
	}
}

func (ts *TestSuite) checkAlertsLoop() {
	defer ts.wg.Done()

	ts.loopTillItsOver(func() {
		nowTs := timestamp.FromTime(time.Now())

		b, err := DoGetRequest(ts.alertsAPIURL, ts.opts.Config.Auth.RulesAndAlertsAPI)
		if err != nil {
			level.Error(ts.logger).Log("msg", "Error in fetching alerts", "url", ts.alertsAPIURL, "err", err)
			return
		}

		mappedAlerts, err := ParseAndGroupAlerts(b)
		if err != nil {
			level.Error(ts.logger).Log("msg", "Error in parsing alerts response", "url", ts.alertsAPIURL, "err", err)
			return
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
	})
}

func (ts *TestSuite) checkRulesLoop() {
	defer ts.wg.Done()

	ts.loopTillItsOver(func() {
		nowTs := timestamp.FromTime(time.Now())

		b, err := DoGetRequest(ts.rulesAPIURL, ts.opts.Config.Auth.RulesAndAlertsAPI)
		if err != nil {
			level.Error(ts.logger).Log("msg", "Error in fetching rules", "url", ts.rulesAPIURL, "err", err)
			return
		}

		mappedGroups, err := ParseAndGroupRules(b)
		if err != nil {
			level.Error(ts.logger).Log("msg", "Error in parsing rules response", "url", ts.rulesAPIURL, "err", err)
			return
		}

		groupsToRemove := make(map[string]error)
		ts.ruleGroupTestsMtx.RLock()
		for groupName, c := range ts.ruleGroupTests {
			if c.TestUntil() < nowTs {
				groupsToRemove[groupName] = nil
				continue
			}
			err := c.CheckRuleGroup(nowTs, mappedGroups[groupName])
			if err != nil {
				groupsToRemove[groupName] = err
			}
		}
		ts.ruleGroupTestsMtx.RUnlock()

		ts.removeGroups(groupsToRemove)
	})
}

func (ts *TestSuite) checkMetricsLoop() {
	defer ts.wg.Done()

	ts.loopTillItsOver(func() {
		nowTs := timestamp.FromTime(time.Now())

		u := ts.promqlURL
		q := u.Query()
		q.Set("query", "ALERTS")
		q.Set("time", timestamp.Time(nowTs).Format(time.RFC3339))
		u.RawQuery = q.Encode()

		b, err := DoGetRequest(u.String(), ts.opts.Config.Auth.Query)
		if err != nil {
			level.Error(ts.logger).Log("msg", "Error in fetching metrics", "url", u.String(), "err", err)
			return
		}

		mappedMetrics, err := ParseAndGroupMetrics(b)
		if err != nil {
			level.Error(ts.logger).Log("msg", "Error in parsing metrics response", "url", u.String(), "err", err)
			return
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
	})
}

func (ts *TestSuite) monitorAlertReception() {
	defer ts.wg.Done()

	ts.loopTillItsOver(func() {
		nowTs := timestamp.FromTime(time.Now())
		groupsToRemove := make(map[string]error)
		for groupName := range ts.as.groupsFacingErrors() {
			groupsToRemove[groupName] = errors.New("error in alert reception")
		}

		ts.ruleGroupTestsMtx.RLock()
		for groupName, c := range ts.ruleGroupTests {
			if c.TestUntil() < nowTs {
				groupsToRemove[groupName] = nil
			}
		}
		ts.ruleGroupTestsMtx.RUnlock()

		ts.removeGroups(groupsToRemove)
	})
}

// loopTillItsOver runs the given function in intervals until the test has ended.
func (ts *TestSuite) loopTillItsOver(f func()) {
	defer ts.Stop()

	for !ts.isOver() {
		select {
		case <-ts.stopc:
			return
		case <-time.After(time.Duration(ts.minGroupInterval)):
			f()
		}
	}
}

func (ts *TestSuite) removeGroups(groupsToRemove map[string]error) {
	ts.ruleGroupTestsMtx.Lock()
	defer ts.ruleGroupTestsMtx.Unlock()
	for gn, err := range groupsToRemove {
		if _, ok := ts.ruleGroupTests[gn]; !ok {
			// Has been already removed.
			continue
		}
		delete(ts.ruleGroupTests, gn)
		if err != nil {
			ts.ruleGroupTestErrors[gn] = append(ts.ruleGroupTestErrors[gn], err)
			level.Error(ts.logger).Log("msg", "Test failed for a rule group", "rulegroup", gn, "err", err)
		} else {
			level.Info(ts.logger).Log("msg", "Test finished successfully for a rule group", "rulegroup", gn)
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
		// TODO: there might still be a race in calling Stop twice. Low priority to fix it.
		close(ts.stopc)
		ts.as.Stop()
		ts.remoteWriter.Stop()
	}
}

func (ts *TestSuite) Wait() {
	ts.as.Wait()
	ts.remoteWriter.Wait()
	ts.wg.Wait()
}

// Error() returns any error occured during execution of test and does
// not tell if the tests passed or failed.
func (ts *TestSuite) Error() error {
	merr := NewMulti()
	merr.Add(errors.Wrap(ts.remoteWriter.Error(), "remote writer"))
	merr.Add(errors.Wrap(ts.as.runningError(), "alert server"))
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

	groupsFacingErrors := ts.as.groupsFacingErrors()
	expectedAlertsError := ts.as.expectedAlertsError()

	if len(ts.ruleGroupTestErrors) == 0 && len(groupsFacingErrors) == 0 && len(expectedAlertsError) == 0 {
		if ts.opts.Config.Settings.DisableAlertsAPICheck ||
			ts.opts.Config.Settings.DisableRulesAPICheck ||
			ts.opts.Config.Settings.DisableAlertsMetricsCheck ||
			ts.opts.Config.Settings.DisableAlertsReceptionCheck {

			describe = "Congrats! The following tests passed:"

			if !ts.opts.Config.Settings.DisableAlertsAPICheck {
				describe += " AlertsAPICheck"
			}
			if !ts.opts.Config.Settings.DisableRulesAPICheck {
				describe += " RulesAPICheck"
			}
			if !ts.opts.Config.Settings.DisableAlertsMetricsCheck {
				describe += " AlertsMetricsCheck"
			}
			if !ts.opts.Config.Settings.DisableAlertsReceptionCheck {
				describe += " AlertsReceptionCheck"
			}

			return true, describe
		}
		return true, "Congrats! All tests passed"
	}

	if len(ts.ruleGroupTestErrors) > 0 {
		describe += "------------------------------------------\n"
		describe += "The following rule groups failed the API and metrics check:\n"
		for gn, errs := range ts.ruleGroupTestErrors {
			describe += "\nGroup Name: " + gn + "\n"
			for i, err := range errs {
				describe += fmt.Sprintf("\tError %d: %s\n", i+1, err.Error())
			}
		}
	}

	alertServerErrors := ts.as.groupError()
	if len(groupsFacingErrors) > 0 {
		describe += "------------------------------------------\n"
		describe += "The following rule groups faced alert reception issues:\n"
		for gn := range groupsFacingErrors {
			errs := alertServerErrors[gn]
			describe += "\nGroup Name: " + gn + "\n"

			if len(errs.missedAlerts) > 0 {
				describe += "\tReason: Missed some alerts that were expected (time is approx)\n"
				for i, ma := range errs.missedAlerts {
					state := "firing"
					if ma.Resolved {
						state = "resolved"
					}
					describe += fmt.Sprintf("\t\t%d: Expected time: %s, Labels: %s, Annotations: %s, State: %s, Resend: %t\n",
						i+1,
						ma.Ts.Format(time.RFC3339Nano),
						ma.Alert.Labels.String(),
						ma.Alert.Annotations.String(),
						state,
						ma.Resend,
					)
				}
			}

			if len(errs.matchingErrs) > 0 {
				describe += "\tReason: Alerts mismatch while received at right time\n"
				for i, err := range errs.matchingErrs {
					state := "firing"
					if err.expectedAlert.Resolved {
						state = "resolved"
					}
					describe += fmt.Sprintf("\t\t%d: At %s, Expected State: %s, Labels: %s, Annotations: %s, Error: %s\n",
						i+1,
						err.t.Format(time.RFC3339Nano),
						state,
						err.alert.Labels.String(),
						err.alert.Annotations.String(),
						err.err.Error(),
					)
				}
			}

			if len(errs.unexpectedAlerts) > 0 {
				describe += "\tReason: Unexpected alerts (Example: alerts that we didn't expect OR received outside expected time range OR duplicate alerts)\n"
				for i, alert := range errs.unexpectedAlerts {
					describe += fmt.Sprintf("\t\t%d: At %s, Labels: %s, Annotations: %s, StartsAt: %s, EndsAt: %s, GeneratorURL: %s\n",
						i+1,
						alert.t.Format(time.RFC3339Nano),
						alert.alert.Labels.String(),
						alert.alert.Annotations.String(),
						alert.alert.StartsAt.Format(time.RFC3339Nano),
						alert.alert.EndsAt.Format(time.RFC3339Nano),
						alert.alert.GeneratorURL,
					)
				}
			}

		}
	}

	if len(expectedAlertsError) > 0 {
		desc := ""
		for _, eas := range expectedAlertsError {
			gn := eas[0].Alert.Labels.Get("rulegroup")
			if groupsFacingErrors[gn] {
				// This group had some other error above.
				continue
			}
			desc += "\nGroup Name: " + gn + "\n"
			for i, ea := range eas {
				state := "firing"
				if ea.Resolved {
					state = "resolved"
				}
				desc += fmt.Sprintf("\t%d: Expected time: %s, Labels: %s, Annotations: %s, State: %s, Resend: %t\n",
					i+1,
					ea.Ts.Format(time.RFC3339Nano),
					ea.Alert.Labels.String(),
					ea.Alert.Annotations.String(),
					state,
					ea.Resend,
				)
			}
		}
		if desc != "" {
			describe += "------------------------------------------\n"
			describe += "The following alerts were still expected but were not received in time:\n"
			describe += desc
		}
	}

	return false, describe
}

func (ts *TestSuite) TestUntil() time.Time {
	var tu int64
	ts.ruleGroupTestsMtx.RLock()
	for _, c := range ts.ruleGroupTests {
		ctu := c.TestUntil()
		if ctu > tu {
			tu = ctu
		}
	}
	ts.ruleGroupTestsMtx.RUnlock()
	return timestamp.Time(tu)
}

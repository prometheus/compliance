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

// Manager runs the entire test suite from start to end.
type Manager struct {
	opts                 ManagerOptions
	remoteWriter         *RemoteWriter
	remoteWriteStartTime time.Time

	ruleGroupTestsMtx   sync.RWMutex
	ruleGroupTests      map[string]cases.TestCase // Group name -> TestCase.
	ruleGroupTestErrors map[string][]error        // Group name -> slice of errors in them.

	minGroupInterval model.Duration

	stopc chan struct{}
	wg    sync.WaitGroup
}

type ManagerOptions struct {
	Logger log.Logger
	// All the test cases to test.
	Cases []cases.TestCase
	// RemoteWriteURL is URL to remote write samples.
	RemoteWriteURL string
	// BaseApiURL is the URL to query the GET <BaseApiURL>/api/v1/rules and <BaseApiURL>/api/v1/alerts.
	BaseApiURL string
	// PromQLBaseURL is the URL to query the database via PromQL via GET <PromQLBaseURL>/query and <PromQLBaseURL>/query_range.
	PromQLBaseURL string
}

func NewManager(opts ManagerOptions) (*Manager, error) {
	if err := validateOpts(opts); err != nil {
		return nil, errors.Wrap(err, "validate options")
	}

	remoteWriter, err := NewRemoteWriter(opts.RemoteWriteURL, opts.Logger)
	if err != nil {
		return nil, errors.Wrap(err, "create remote writer")
	}

	ruleGroupTests := make(map[string]cases.TestCase, len(opts.Cases))
	var minGroupInterval model.Duration
	for i, c := range opts.Cases {
		remoteWriter.AddTimeSeries(c.SamplesToRemoteWrite())
		groupName, _ := c.Describe()
		ruleGroupTests[groupName] = c

		rg, err := c.RuleGroup()
		if err != nil {
			return nil, err
		}
		if i == 0 || rg.Interval < minGroupInterval {
			minGroupInterval = rg.Interval
		}
	}

	return &Manager{
		remoteWriter:     remoteWriter,
		opts:             opts,
		ruleGroupTests:   ruleGroupTests,
		minGroupInterval: minGroupInterval,
		stopc:            make(chan struct{}),
	}, nil
}

// minGroupInterval is the minimum group interval for any rule.
// The API/PromQL check interval is based on the group interval per rule.
// Hence, we have a minimum to keep that interval not so small.
const minGroupInterval = model.Duration(20 * time.Second)

// TODO(codesome): verify the validation.
func validateOpts(opts ManagerOptions) error {
	if opts.RemoteWriteURL == "" {
		return fmt.Errorf("no remote write URL found")
	}

	seenRuleGroups := make(map[string]bool)
	seenAlertNames := make(map[string]bool)

	for _, c := range opts.Cases {
		rg, err := c.RuleGroup()
		if err != nil {
			return err
		}
		if rg.Interval < minGroupInterval {
			return fmt.Errorf("group interval too small for the group %q, min is %s, got %s", rg.Name, minGroupInterval.String(), rg.Interval.String())
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

func (m *Manager) Start() {
	level.Info(m.opts.Logger).Log("msg", "Starting the remote writer", "url", m.opts.RemoteWriteURL)
	m.remoteWriteStartTime = m.remoteWriter.Start()
	for _, c := range m.ruleGroupTests {
		c.Init(timestamp.FromTime(m.remoteWriteStartTime))
		gn, desc := c.Describe()
		level.Info(m.opts.Logger).Log("msg", "Starting test for a rule group", "rulegroup", gn, "description", desc)
	}

	m.wg.Add(2)
	go m.checkAlertsLoop()
	go m.checkMetricsLoop()
}

func (m *Manager) checkAlertsLoop() {
	defer m.wg.Done()
	defer m.Stop()

Loop:
	for {
		select {
		case <-m.stopc:
			return
		case <-time.After(time.Duration(m.minGroupInterval)):
			nowTs := timestamp.FromTime(time.Now())
			u, err := url.Parse(m.opts.BaseApiURL)
			if err != nil {
				level.Error(m.opts.Logger).Log("msg", "Error in parsing API URL", "url", m.opts.BaseApiURL, "err", err)
				continue Loop
			}

			u.Path = path.Join(u.Path, "/api/v1/alerts")
			b, err := DoGetRequest(u.String())
			if err != nil {
				level.Error(m.opts.Logger).Log("msg", "Error in fetching alerts", "url", u.String(), "err", err)
				continue Loop
			}

			mappedAlerts, err := ParseAndGroupAlerts(b)
			if err != nil {
				level.Error(m.opts.Logger).Log("msg", "Error in parsing alerts response", "url", u.String(), "err", err)
				continue Loop
			}

			groupsToRemove := make(map[string]error)
			m.ruleGroupTestsMtx.RLock()
			for groupName, c := range m.ruleGroupTests {
				if c.TestUntil() < nowTs {
					groupsToRemove[groupName] = nil
					continue
				}
				err := c.CheckAlerts(nowTs, mappedAlerts[groupName])
				if err != nil {
					groupsToRemove[groupName] = err
				}
			}
			m.ruleGroupTestsMtx.RUnlock()

			if m.removeGroups(groupsToRemove) {
				return
			}
		}
	}
}

func (m *Manager) checkMetricsLoop() {
	defer m.wg.Done()
	defer m.Stop()

Loop:
	for {
		select {
		case <-m.stopc:
			return
		case <-time.After(time.Duration(m.minGroupInterval)):
			nowTs := timestamp.FromTime(time.Now())

			u, err := url.Parse(m.opts.PromQLBaseURL)
			if err != nil {
				level.Error(m.opts.Logger).Log("msg", "Error in parsing PromQL URL", "url", m.opts.BaseApiURL, "err", err)
				continue Loop
			}

			u.Path = path.Join(u.Path, "/api/v1/query")
			q := u.Query()
			q.Add("query", "ALERTS")
			q.Add("time", timestamp.Time(nowTs).Format(time.RFC3339))
			u.RawQuery = q.Encode()

			b, err := DoGetRequest(u.String())
			if err != nil {
				level.Error(m.opts.Logger).Log("msg", "Error in fetching metrics", "url", u.String(), "err", err)
				continue Loop
			}

			mappedMetrics, err := ParseAndGroupMetrics(b)
			if err != nil {
				level.Error(m.opts.Logger).Log("msg", "Error in parsing metrics response", "url", u.String(), "err", err)
				continue Loop
			}

			groupsToRemove := make(map[string]error)
			m.ruleGroupTestsMtx.RLock()
			for groupName, c := range m.ruleGroupTests {
				if c.TestUntil() < nowTs {
					groupsToRemove[groupName] = nil
					continue
				}
				err := c.CheckMetrics(nowTs, mappedMetrics[groupName])
				if err != nil {
					groupsToRemove[groupName] = err
				}
			}
			m.ruleGroupTestsMtx.RUnlock()

			if m.removeGroups(groupsToRemove) {
				return
			}
		}
	}
}

func (m *Manager) removeGroups(groupsToRemove map[string]error) (empty bool) {
	m.ruleGroupTestsMtx.Lock()
	defer m.ruleGroupTestsMtx.Unlock()
	for gn, err := range groupsToRemove {
		delete(m.ruleGroupTests, gn)
		if err != nil {
			m.ruleGroupTestErrors[gn] = append(m.ruleGroupTestErrors[gn], err)
			level.Error(m.opts.Logger).Log("msg", "Test failed for a rule group", "rulegroup", gn, "err", err)
		} else {
			level.Info(m.opts.Logger).Log("msg", "Test finished successfully for a rule group", "rulegroup", gn)
		}
	}
	return len(m.ruleGroupTests) == 0
}

func (m *Manager) Stop() {
	select {
	case <-m.stopc:
		// Already stopped.
	default:
		m.remoteWriter.Stop()
		close(m.stopc)
	}
}

func (m *Manager) Wait() {
	m.remoteWriter.Wait()
	m.wg.Wait()
}

// Error() returns any error occured during execution of test and does
// not tell if the tests passed or failed.
func (m *Manager) Error() error {
	merr := tsdb_errors.NewMulti()
	merr.Add(errors.Wrap(m.remoteWriter.Error(), "remote writer"))
	return merr.Err()
}

// WasTestSuccessful tells if all the tests passed.
// It returns an explanation if any test failed.
// Before calling this method:
// 	* Error() should be checked for no errors.
//  * The test should have finished (i.e. Wait() is not blocking).
func (m *Manager) WasTestSuccessful() (yes bool, describe string) {
	select {
	case <-m.stopc:
	default:
		return false, "test is still running"
	}

	if err := m.Error(); err != nil {
		return false, fmt.Sprintf("got some error in Error(): %q", err.Error())
	}

	if len(m.ruleGroupTestErrors) == 0 {
		return true, "Congrats! All tests passed"
	}

	describe = "The following rule groups failed the test:\n"
	for gn, errs := range m.ruleGroupTestErrors {
		describe += "\nGroup Name: " + gn + "\n"
		for i, err := range errs {
			describe += fmt.Sprintf("\tError %d: %s\n", i+1, err.Error())
		}
	}

	return false, describe
}

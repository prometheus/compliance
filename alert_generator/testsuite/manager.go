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
	"github.com/prometheus/compliance/alert_generator/cases"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/pkg/timestamp"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
)

// Manager runs the entire test suite from start to end.
type Manager struct {
	opts                 ManagerOptions
	remoteWriter         *RemoteWriter
	remoteWriteStartTime time.Time
	ruleGroupTests       map[string]*RuleGroupTest // Group name -> RuleGroupTest.

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

	ruleGroupTests := make(map[string]*RuleGroupTest, len(opts.Cases))
	var minGroupInterval model.Duration
	for i, c := range opts.Cases {
		remoteWriter.AddTimeSeries(c.SamplesToRemoteWrite())
		rgt, err := NewRuleGroupTest(c, opts.Logger)
		if err != nil {
			return nil, errors.Wrap(err, "get rule group test")
		}
		groupName, _ := c.Describe()
		ruleGroupTests[groupName] = rgt

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
	for _, rgt := range m.ruleGroupTests {
		rgt.Start(m.remoteWriteStartTime)
	}

	m.wg.Add(2)
	go m.checkAlertsLoop()
	go m.checkMetricsLoop()
}

func (m *Manager) checkAlertsLoop() {
	defer m.wg.Done()

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

			for groupName, rgt := range m.ruleGroupTests {
				ok, _ := rgt.CheckAlerts(nowTs, mappedAlerts[groupName])
				if !ok {
					// TODO: add error here.
					level.Error(m.opts.Logger).Log("msg", "Check alerts failed", "group_name", groupName)
				}
			}
		}
	}
}

func (m *Manager) checkMetricsLoop() {
	defer m.wg.Done()

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

			for groupName, rgt := range m.ruleGroupTests {
				ok, _ := rgt.CheckMetrics(nowTs, mappedMetrics[groupName])
				if !ok {
					// TODO: add error here.
					level.Error(m.opts.Logger).Log("msg", "Check metrics failed", "group_name", groupName)
				}
			}
		}
	}
}

func (m *Manager) Stop() {
	close(m.stopc)
	m.remoteWriter.Stop()
	for _, rgt := range m.ruleGroupTests {
		rgt.Stop()
	}
}

func (m *Manager) Wait() {
	m.wg.Wait()
	m.remoteWriter.Wait()
	for _, rgt := range m.ruleGroupTests {
		rgt.Wait()
	}
}

func (m *Manager) Error() error {
	merr := tsdb_errors.NewMulti()
	merr.Add(errors.Wrap(m.remoteWriter.Error(), "remote writer"))
	for _, rgt := range m.ruleGroupTests {
		merr.Add(errors.Wrapf(rgt.Error(), "error from rule group %q", rgt.rg.Name))
	}
	return merr.Err()
}

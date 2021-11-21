package testsuite

import (
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"

	"github.com/prometheus/compliance/alert_generator/cases"
)

// Manager runs the entire test suite from start to end.
type Manager struct {
	opts         ManagerOptions
	remoteWriter *RemoteWriter

	apiCheckInterval time.Duration
}

type ManagerOptions struct {
	Logger log.Logger
	// All the test cases to test.
	Cases []cases.TestCase
	// URL to remote write samples.
	RemoteWriteURL string
	// URL to query the GET APIs.
	BaseApiURL string
	// URL to query the database via PromQL, without the /query or /query_range suffix.
	PromQLURL string
}

func NewManager(opts ManagerOptions) (*Manager, error) {
	if err := validateOpts(opts); err != nil {
		return nil, errors.Wrap(err, "validate options")
	}

	remoteWriter, err := NewRemoteWriter(opts.RemoteWriteURL, opts.Logger)
	if err != nil {
		return nil, errors.Wrap(err, "create remote writer")
	}

	for _, c := range opts.Cases {
		remoteWriter.AddTimeSeries(c.SamplesToRemoteWrite())
	}

	return &Manager{
		remoteWriter: remoteWriter,
		opts:         opts,
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
	go m.remoteWriter.Start()

}

func (m *Manager) Stop() {
	if m.remoteWriter != nil {
		m.remoteWriter.Stop()
	}
}

func (m *Manager) Wait() {
	if m.remoteWriter != nil {
		m.remoteWriter.Wait()
	}

}

func (m *Manager) Error() error {
	if m.remoteWriter != nil {
		return errors.Wrap(m.remoteWriter.Error(), "remote writer")
	}
	return nil
}

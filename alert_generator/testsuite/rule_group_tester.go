package testsuite

import (
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/promql"
	v1 "github.com/prometheus/prometheus/web/api/v1"

	"github.com/prometheus/compliance/alert_generator/cases"
)

type RuleGroupTest struct {
	rwStartTime time.Time
	c           cases.TestCase
	logger      log.Logger
	rg          rulefmt.RuleGroup
}

func NewRuleGroupTest(c cases.TestCase, l log.Logger) (*RuleGroupTest, error) {
	rg, err := c.RuleGroup()
	if err != nil {
		return nil, errors.Wrap(err, "get rule group")
	}
	return &RuleGroupTest{
		c:      c,
		logger: l,
		rg:     rg,
	}, nil
}

func (r *RuleGroupTest) Start(rwStartTime time.Time) {
	r.rwStartTime = rwStartTime
	level.Info(r.logger).Log("msg", "Starting test for the rule group", "group", r.rg.Name)

	r.c.Init(timestamp.FromTime(rwStartTime))

}

func (r *RuleGroupTest) CheckAlerts(ts int64, alerts []v1.Alert) (ok bool, expected []v1.Alert) {
	return r.c.CheckAlerts(ts, alerts)
}

func (r *RuleGroupTest) CheckMetrics(ts int64, metrics []promql.Sample) (ok bool, expected string) {
	return r.c.CheckMetrics(ts, metrics)
}

func (r *RuleGroupTest) Stop() {
	level.Info(r.logger).Log("msg", "Stopping test for the rule group", "group", r.rg.Name)
}

func (r *RuleGroupTest) Wait() {}

func (r *RuleGroupTest) Error() error { return nil }

package cases

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/web/api/v1"
)

const (
	sourceTimeSeriesName = "alert_generator_test_suite"
)

func metricLabels(groupName, alertName string) labels.Labels {
	return labels.FromStrings(
		"__name__", sourceTimeSeriesName,
		"rulegroup", groupName,
		"alertname", alertName,
	)
}

func toProtoLabels(lbls labels.Labels) []prompb.Label {
	res := make([]prompb.Label, 0, len(lbls))
	for _, l := range lbls {
		res = append(res, prompb.Label{
			Name:  l.Name,
			Value: l.Value,
		})
	}
	return res
}

// sampleSlice take the interval and the sample values for the samples.
// The returned samples start at timestamp 0 with an increment of 'interval'
// in milliseconds.
// Each value notation must be of the form "V" or "AxB" where
//   * "V" is the absolute value of the sample in float.
//   * "AxB" A is the value increment per sample and B is the number of samples.
//     The initial value starts at 0 if this notation is the first value.
//     A is a float, B is an integer.
// Example:
//   Input values : [ "1x1",  "0x3",      "5x3",   "9", "8",   "-2x2" ]
//   Output values: [   1,   1, 1, 1,   6, 11, 16,  9,   8,     6, 4 ]
func sampleSlice(interval time.Duration, values ...string) []prompb.Sample {
	var samples []prompb.Sample
	ts := time.Unix(0, 0)
	var val float64
	for _, v := range values {
		splits := strings.Split(v, "x")
		if len(splits) == 2 {
			a, err := strconv.ParseFloat(splits[0], 64)
			if err != nil {
				panic(fmt.Sprintf("invalid values notation %s, err: %s", v, err.Error()))
			}

			b, err := strconv.Atoi(splits[1])
			if err != nil {
				panic(fmt.Sprintf("invalid values notation %s, err: %s", v, err.Error()))
			}

			for i := 0; i < b; i++ {
				val += a
				samples = append(samples, prompb.Sample{
					Timestamp: timestamp.FromTime(ts),
					Value:     val,
				})
				ts = ts.Add(interval)
			}
		} else if len(splits) == 1 {
			var err error
			val, err = strconv.ParseFloat(splits[0], 64)
			if err != nil {
				panic(fmt.Sprintf("invalid values notation %s, err: %s", splits[0], err.Error()))
			}
			samples = append(samples, prompb.Sample{
				Timestamp: timestamp.FromTime(ts),
				Value:     val,
			})
			ts = ts.Add(interval)
		} else {
			panic(fmt.Sprintf("invalid values notation %s", v))
		}

	}
	return samples
}

// betweenFunc returns a function that returns true if
// ts belongs to (start, end].
func betweenFunc(ts int64) func(start, end float64) bool {
	return func(start, end float64) bool {
		startTs := timestamp.FromFloatSeconds(start)
		// The remote written sample could have been delayed. So we need to
		// account for that as well.
		endTs := timestamp.FromFloatSeconds(end + float64(2*MaxRTT/time.Second))
		return ts > startTs && ts <= endTs
	}
}

// checkExpectedAlerts checks the actual alerts with all possible combinations of expected alerts
// provided. It returns an error if none of them match.
//
// Notes about parameters:
//   1) len(expAlerts) != 0
//   2) len(expAlerts) == len(expActiveAtRanges) (and) len(expAlerts[i]) == len(expActiveAtRanges[i])
//   3) expActiveAtRanges[i][0] <= actAlerts[i].ActiveAt <= expActiveAtRanges[i][1]
// TODO: write unit tests for this.
func checkExpectedAlerts(expAlerts [][]v1.Alert, actAlerts []v1.Alert, interval time.Duration) error {
	var errs []error
	for _, exp := range expAlerts {
		err := areAlertsEqual(exp, actAlerts, interval)
		if err == nil {
			// We only need one of the expected slice to match.
			return nil
		}
		errs = append(errs, err)
	}

	if len(errs) == 1 {
		return errors.Wrap(errs[0], "error in alerts")
	}

	errMsg := "one of the following errors happened in alerts:"
	for i, err := range errs {
		errMsg += fmt.Sprintf("\n\t\t(%d) %s", i+1, err.Error())
	}

	return errors.New(errMsg)
}

// areAlertsEqual tells whether both the expected and actual alerts match.
func areAlertsEqual(exp, act []v1.Alert, interval time.Duration) error {
	if len(exp) != len(act) {
		return errors.Errorf("different number of alerts - expected(%d): %v, actual(%d): %v", len(exp), exp, len(act), act)
	}

	sort.Slice(exp, func(i, j int) bool {
		return labels.Compare(exp[i].Labels, exp[j].Labels) <= 0
	})
	sort.Slice(act, func(i, j int) bool {
		return labels.Compare(act[i].Labels, act[j].Labels) <= 0
	})

	for i := range exp {
		e, a := exp[i], act[i]
		ev, err := strconv.ParseFloat(e.Value, 64)
		if err != nil {
			return errors.Errorf("unexpected error in the test suite - alert: %v, error: %s", e, err.Error())
		}
		av, err := strconv.ParseFloat(a.Value, 64)

		if err != nil {
			return errors.Errorf("error when parsing the value - alert: %v, error: %s", a, err.Error())
		}

		ok := labels.Compare(e.Labels, a.Labels) == 0 &&
			labels.Compare(e.Annotations, a.Annotations) == 0 &&
			e.State == a.State &&
			ev == av

		if !ok {
			return errors.Errorf("alerts mismatch - expected: %v, actual: %v", e, a)
		}
	}

	// The alerts match. Time to check the ActiveAt if it matches.
	for i := range act {
		if act[i].ActiveAt == nil {
			return errors.Errorf("ActiveAt not found for the alert - alert: %v", act[i])
		}
		t := *act[i].ActiveAt
		if t.Before(*exp[i].ActiveAt) || exp[i].ActiveAt.Add(interval+(2*MaxRTT)).Before(t) {
			// Out of the range.
			return errors.Errorf(
				"ActiveAt mismatch - alert: %v, expected ActiveAT range: [%s, %s], actual ActiveAt: %s",
				act[i],
				exp[i].ActiveAt.Format(time.RFC3339),
				exp[i].ActiveAt.Add(interval).Format(time.RFC3339),
				act[i].ActiveAt.Format(time.RFC3339),
			)
		}
	}

	return nil
}

// checkExpectedRuleGroup checks the actual rule group with all possible combinations of expected alerts
// provided and the rule group fields. It returns an error if none of them match.
// This runs the same logic as checkExpectedAlerts for checking the alerts of the rule group.
func checkExpectedRuleGroup(now time.Time, expRgs []v1.RuleGroup, actRg v1.RuleGroup) error {
	var actAlerts []v1.Alert
	var actRules []v1.AlertingRule
	for _, r := range actRg.Rules {
		ar, ok := r.(v1.AlertingRule)
		if !ok {
			return fmt.Errorf("found a rule that is not an alerting rule")
		}
		actRules = append(actRules, ar)
		for _, a := range ar.Alerts {
			actAlerts = append(actAlerts, *a)
		}
	}

	sort.Slice(actRules, func(i, j int) bool {
		l, r := actRules[i], actRules[j]
		if l.Name == r.Name {
			return labels.Compare(l.Labels, r.Labels) <= 0
		}
		return l.Name < r.Name
	})

	var errs []error
	collectErr := func(err error) {
		if err != nil {
			errs = append(errs, err)
		}
	}

	for _, rg := range expRgs {
		if rg.Name != actRg.Name {
			collectErr(fmt.Errorf("wrong group name, expected: %q, got: %q", rg.Name, actRg.Name))
			continue
		}

		if rg.Interval != actRg.Interval {
			collectErr(fmt.Errorf("wrong group interval, expected: %f, got: %f", rg.Interval, actRg.Interval))
			continue
		}

		// Evaluation should be within last interval time while considering the send delay.
		itvl := time.Duration(rg.Interval * float64(time.Second))
		cutOff := now.Add(-MaxRTT).Add(-itvl)
		if actRg.LastEvaluation.Before(cutOff) {
			collectErr(fmt.Errorf("expected a group evaluation after %s, but the last evaluation was on %s",
				cutOff.Format(time.RFC3339Nano), actRg.LastEvaluation.UTC().Format(time.RFC3339Nano)))
			continue
		}

		if len(rg.Rules) != len(actRg.Rules) {
			collectErr(fmt.Errorf("different number of rules, expected: %d, got: %d", len(rg.Rules), len(actRg.Rules)))
			continue
		}

		err := areRulesEqual(now, itvl, rg.Rules, actRules, actAlerts)
		if err == nil {
			// This rule group matched.
			return nil
		}
		collectErr(err)
	}

	if len(errs) == 1 {
		return errors.Wrap(errs[0], "error in rules")
	}

	errMsg := "one of the following errors happened in rules:"
	for i, err := range errs {
		errMsg += fmt.Sprintf("\n\t\t(%d) %s", i+1, err.Error())
	}

	return errors.New(errMsg)
}

func areRulesEqual(now time.Time, itvl time.Duration, exp []v1.Rule, actRules []v1.AlertingRule, actAlerts []v1.Alert) error {
	var expAlerts []v1.Alert
	var expRules []v1.AlertingRule
	for _, r := range exp {
		ar, ok := r.(v1.AlertingRule)
		if !ok {
			panic("expected rules can only be alerting rules")
		}
		expRules = append(expRules, ar)
		for _, a := range ar.Alerts {
			expAlerts = append(expAlerts, *a)
		}
	}

	sort.Slice(expRules, func(i, j int) bool {
		l, r := expRules[i], expRules[j]
		if l.Name == r.Name {
			return labels.Compare(l.Labels, r.Labels) <= 0
		}
		return l.Name < r.Name
	})

	for i := range expRules {
		e, a := expRules[i], actRules[i]
		mismatch := ""
		eq, err := parser.ParseExpr(e.Query)
		if err != nil {
			panic("expecting query is not parsing: " + err.Error())
		}
		aq, err := parser.ParseExpr(a.Query)
		if err != nil {
			return fmt.Errorf("error in parsing query: %w ", err)
		}
		switch {
		case e.State != a.State:
			mismatch = "State"
		case e.Name != a.Name:
			mismatch = "Name"
		case eq.String() != aq.String():
			mismatch = "Query"
		case e.Duration != a.Duration:
			mismatch = "Duration"
		case labels.Compare(e.Labels, a.Labels) != 0:
			mismatch = "Labels"
		case labels.Compare(e.Annotations, a.Annotations) != 0:
			mismatch = "Annotations"
		case e.Health != a.Health:
			mismatch = "Health"
		case e.Type != a.Type:
			mismatch = "Type"
		case e.LastError != a.LastError:
			mismatch = "LastError"
		}

		if mismatch != "" {
			return fmt.Errorf("rules do not match, mismatch in %q, \n\t\texpected(ignoring Alerts and LastEvaluation): %#v, \n\t\tgot: %#v", mismatch, expRules, actRules)
		}

		cutOff := now.Add(-MaxRTT).Add(-itvl)
		if a.LastEvaluation.Before(cutOff) {
			return fmt.Errorf("expected evaluation for %q rule after %s, but the last evaluation was on %s", a.Name,
				cutOff.Format(time.RFC3339Nano), a.LastEvaluation.UTC().Format(time.RFC3339Nano))
		}
	}

	return checkExpectedAlerts([][]v1.Alert{expAlerts}, actAlerts, itvl)
}

// checkExpectedSamples checks the actual samples with all possible combinations of expected samples
// provided. It returns an error if none of them match.
// TODO: write unit tests for this.
func checkExpectedSamples(expSamples [][]promql.Sample, act []promql.Sample) error {
	var errs []error
	for _, exp := range expSamples {
		err := areSamplesEqual(exp, act)
		if err == nil {
			// We only need one of the expected slice to match.
			return nil
		}

		errs = append(errs, err)
	}

	if len(errs) == 1 {
		return errors.New("error in metrics: " + errs[0].Error())
	}

	errMsg := "one of the following errors happened in metrics:"
	for i, err := range errs {
		errMsg += fmt.Sprintf(" (%d) %s", i+1, err.Error())
	}

	return errors.New(errMsg)
}

// areSamplesEqual tells whether both the expected and actual samples match.
func areSamplesEqual(exp, act []promql.Sample) error {
	if len(exp) != len(act) {
		return errors.Errorf("different number of metrics - expected(%d): %v, actual(%d): %v", len(exp), exp, len(act), act)
	}

	sort.Slice(exp, func(i, j int) bool {
		return labels.Compare(exp[i].Metric, exp[j].Metric) <= 0
	})
	sort.Slice(act, func(i, j int) bool {
		return labels.Compare(act[i].Metric, act[j].Metric) <= 0
	})

	for i := range exp {
		e, a := exp[i], act[i]
		ok := labels.Compare(e.Metric, a.Metric) == 0 &&
			e.T == a.T && e.V == a.V
		if !ok {
			return errors.Errorf("metrics mismatch - expected: %v, actual: %v", e, a)
		}
	}
	return nil
}

// devPrint is used for printing stuff during development.
func devPrint(s string, a []v1.Alert) {
	//fmt.Println(s, a)
}

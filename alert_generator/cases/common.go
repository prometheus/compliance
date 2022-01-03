package cases

import (
	"fmt"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/notifier"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/web/api/v1"
)

// TestCase defines a single test case for the alert generator.
type TestCase interface {
	// Describe returns the test case's rule group name and description.
	// NOTE: The group name must be unique across all the test cases.
	Describe() (groupName string, description string)

	// RuleGroup returns the alerting rule group that this test case is testing.
	// NOTE: All the rules in the group must include a label `rulegroup="<groupName>"`
	//       which should also be attached to the resultant alerts.
	RuleGroup() (rulefmt.RuleGroup, error)

	// SamplesToRemoteWrite gives all the samples that needs to be remote-written at their
	// respective timestamps. The last sample indicates the end of this test case hence appropriate
	// additional samples must be considered beforehand.
	//
	// All the timestamps returned here are in milliseconds and starts at 0,
	// which is the time when the test suite would start the test.
	// The test suite is responsible for translating these 0 based
	// timestamp to the relevant timestamps for the current time.
	//
	// The samples must be delivered to the remote storage after the timestamp specified on the samples
	// and must be delivered within 10 seconds of that timestamp.
	SamplesToRemoteWrite() []prompb.TimeSeries

	// Init tells the test case the actual timestamp for the 0 time.
	Init(zeroTime int64)

	// TestUntil returns a unix timestamp upto which the test must be running on this TestCase.
	// This must be called after Init() and the returned timestamp refers to absolute unix
	// timestamp (i.e. not relative like the SamplesToRemoteWrite())
	TestUntil() int64

	// CheckAlerts returns nil if the alerts provided are as expected at the given timestamp.
	// Returns an error otherwise describing what is the problem.
	// This must be checked with a min interval of the rule group's interval from RuleGroup().
	CheckAlerts(ts int64, alerts []v1.Alert) error

	// CheckMetrics returns nil if at give timestamp the metrics contain the expected metrics.
	// Returns an error otherwise describing what is the problem.
	// This must be checked with a min interval of the rule group's interval from RuleGroup().
	CheckMetrics(ts int64, metrics []promql.Sample) error

	// ExpectedAlerts returns all the expected alerts that must be received for this test case.
	// This must be called only after Init().
	ExpectedAlerts() []ExpectedAlert
}

const resendDelay = time.Minute

const sourceTimeSeriesName = "alert_generator_test_suite"

func baseLabels(groupName, alertName string) labels.Labels {
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

func sampleSlice(interval time.Duration, values ...float64) []prompb.Sample {
	samples := make([]prompb.Sample, 0, len(values))
	ts := time.Unix(0, 0)
	for _, v := range values {
		samples = append(samples, prompb.Sample{
			Timestamp: timestamp.FromTime(ts),
			Value:     v,
		})
		ts = ts.Add(interval)
	}
	return samples
}

// betweenFunc returns a function that returns true if
// ts belongs to (start, end].
func betweenFunc(ts int64) func(start, end float64) bool {
	return func(start, end float64) bool {
		startTs := timestamp.FromFloatSeconds(start)
		endTs := timestamp.FromFloatSeconds(end)
		return ts > startTs && ts <= endTs
	}
}

// checkAllPossibleExpectedAlerts checks the actual alerts with all possible combinations of expected alerts
// provided. It returns an error if none of them match.
//
// Notes about parameters:
//   1) len(expAlerts) != 0
//   2) len(expAlerts) == len(expActiveAtRanges) (and) len(expAlerts[i]) == len(expActiveAtRanges[i])
//   3) expActiveAtRanges[i][0] <= actAlerts[i].ActiveAt <= expActiveAtRanges[i][1]
// TODO: write unit tests for this.
func checkAllPossibleExpectedAlerts(expAlerts [][]v1.Alert, expActiveAtRanges [][][2]time.Time, actAlerts []v1.Alert) error {
	var errs []error
	for i, exp := range expAlerts {
		err := areAlertsEqual(exp, actAlerts, expActiveAtRanges[i])
		if err == nil {
			// We only need one of the expected slice to match.
			return nil
		}
		errs = append(errs, err)
	}

	if len(errs) == 1 {
		return errors.New("error in alerts: " + errs[0].Error())
	}

	errMsg := "one of the following errors happened in alerts:"
	for i, err := range errs {
		errMsg += fmt.Sprintf(" (%d) %s", i+1, err.Error())
	}

	return errors.New(errMsg)
}

// areAlertsEqual tells whether both the expected and actual alerts match.
func areAlertsEqual(exp, act []v1.Alert, expActiveAtRanges [][2]time.Time) error {
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
		if t.Before(expActiveAtRanges[i][0]) || expActiveAtRanges[i][1].Before(t) {
			// Out of the range.
			fmt.Printf("range %v", expActiveAtRanges[i])
			return errors.Errorf(
				"ActiveAt mismatch - alert: %v, expected ActiveAT range: [%s, %s], actual ActiveAt: %s",
				act[i],
				expActiveAtRanges[i][0].Format(time.RFC3339),
				expActiveAtRanges[i][1].Format(time.RFC3339),
				act[i].ActiveAt.Format(time.RFC3339),
			)
		}
	}

	return nil
}

func convertRelativeToAbsoluteTimes(zeroTime int64, original [][2]time.Duration) [][2]time.Time {
	converted := make([][2]time.Time, len(original))
	for i, r := range original {
		converted[i][0] = timestamp.Time(zeroTime).Add(r[0])
		converted[i][1] = timestamp.Time(zeroTime).Add(r[1])
	}
	return converted
}

// checkAllPossibleExpectedSamples checks the actual samples with all possible combinations of expected samples
// provided. It returns an error if none of them match.
// TODO: write unit tests for this.
func checkAllPossibleExpectedSamples(expSamples [][]promql.Sample, act []promql.Sample) error {
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

// ExpectedAlert describes the characteristics of a receiving alert.
// The alert is considered as "may or may not come" (hence no error if not received) in these scenarios:
//   1. (Ts + TimeTolerance) crosses the ResolvedTime time when Resolved is false.
//      Because it can get resolved during the tolerance period.
//   2. (Ts + TimeTolerance) crosses ResolvedTime+15m when Resolved is true.
type ExpectedAlert struct {
	// TimeTolerance is the tolerance to be considered when
	// comparing the time of the alert receiving and alert payload fields.
	// This is usually the group interval.
	// TODO: have some additional tolerance on the http request delay on top of group interval.
	TimeTolerance time.Duration

	// This alert should come at Ts time.
	Ts time.Time

	// If it is a Resolved alert, Resolved must be set to true.
	Resolved bool

	// ResolvedTime is the time when the alert becomes Resolved. time.Unix(0,0) if never Resolved.
	// This is also the EndsAt of the alert when the alert is Resolved.
	ResolvedTime time.Time

	// EndsAtDelta is the duration w.r.t. the alert reception time when the EndsAt must be set.
	// This is only for pending and firing alerts.
	// It is usually 4*resendDelay or 4*groupInterval, whichever is higher.
	EndsAtDelta time.Duration

	// This is the expected alert.
	Alert *notifier.Alert
}

// Matches tells if the given alert satisfies the expected alert description.
func (ea *ExpectedAlert) Matches(now time.Time, a notifier.Alert) error {
	if labels.Compare(ea.Alert.Labels, a.Labels) != 0 {
		return fmt.Errorf("labels mismatch, expected: %s, got: %s", ea.Alert.Labels.String(), a.Labels.String())
	}
	if labels.Compare(ea.Alert.Annotations, a.Annotations) != 0 {
		return fmt.Errorf("annotations mismatch, expected: %s, got: %s", ea.Alert.Annotations.String(), a.Annotations.String())
	}

	matchesTime := func(exp, act time.Time) bool {
		return act.After(exp) && act.Before(exp.Add(ea.TimeTolerance))
	}

	if !matchesTime(ea.Ts, now) {
		return fmt.Errorf("got the alert a little late, expected range: [%s, %s], got: %s",
			ea.Ts.Format(time.RFC3339),
			ea.Ts.Add(ea.TimeTolerance).Format(time.RFC3339),
			now.Format(time.RFC3339),
		)
	}

	if !a.StartsAt.Equal(time.Time{}) && !matchesTime(ea.Alert.StartsAt, a.StartsAt) {
		return fmt.Errorf("mismatch in StartsAt, expected range: [%s, %s], got: %s",
			ea.Alert.StartsAt.Format(time.RFC3339),
			ea.Alert.StartsAt.Add(ea.TimeTolerance).Format(time.RFC3339),
			a.StartsAt.Format(time.RFC3339),
		)
	}

	if !a.EndsAt.Equal(time.Time{}) {
		expEndsAt := now.Add(ea.EndsAtDelta)
		if ea.Resolved {
			expEndsAt = ea.ResolvedTime
		}

		if !matchesTime(expEndsAt, a.EndsAt) {
			return fmt.Errorf("mismatch in EndsAt, expected range: [%s, %s], got: %s",
				expEndsAt.Format(time.RFC3339),
				expEndsAt.Add(ea.TimeTolerance).Format(time.RFC3339),
				a.EndsAt.Format(time.RFC3339),
			)
		}
	}

	if a.GeneratorURL != "" {
		_, err := url.Parse(a.GeneratorURL)
		if err != nil {
			return fmt.Errorf("generator URL %q does not parse as a URL", a.GeneratorURL)
		}
	}

	return nil
}

package cases

import (
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/web/api/v1"
)

// TestCase defines a single test case for the alert generator.
type TestCase interface {
	// RuleGroup returns the alerting rule group that this test case is testing.
	RuleGroup() rulefmt.RuleGroup

	// SamplesToRemoteWrite gives all the samples that needs to be remote-written at their
	// respective timestamps.
	//
	// All the timestamps returned here are in milliseconds and starts at 0,
	// which is the time when the test suite would start the test.
	// The test suite is responsible for translating these 0 based
	// timestamp to the relevant timestmaps for the current time.
	//
	// The samples must be delivered to the remote storage after the timestamp specified on the samples
	// and must be delivered within 10 seconds of that timestamp.
	SamplesToRemoteWrite() []prompb.TimeSeries

	// Init tells the test case the actual timestamp for the 0 time.
	Init(zeroTime int64)

	// CheckAlerts returns true if the alerts provided are as expected at the given timestamp.
	// In case it's not correct, it returns false and the expected alerts.
	CheckAlerts(ts int64, alerts []v1.Alert) (ok bool, expected []v1.Alert)

	// CheckMetrics returns true if at give timestamp the metrics contain the expected metrics.
	// In case it's not correct, it returns false and the expected metrics.
	CheckMetrics(ts int64, metrics string) (ok bool, expected string)
}

// testCase implements TestCase.
type testCase struct {
	ruleGroup            func() rulefmt.RuleGroup
	samplesToRemoteWrite func() []prompb.TimeSeries
	init                 func(zeroTime int64)
	checkAlerts          func(ts int64, alerts []v1.Alert) (ok bool, expected []v1.Alert)
	checkMetrics         func(ts int64, metrics string) (ok bool, expected string)
}

func (tc *testCase) RuleGroup() rulefmt.RuleGroup {
	if tc.ruleGroup != nil {
		return tc.ruleGroup()
	}
	return rulefmt.RuleGroup{}
}

func (tc *testCase) SamplesToRemoteWrite() []prompb.TimeSeries {
	if tc.samplesToRemoteWrite != nil {
		return tc.samplesToRemoteWrite()
	}
	return nil
}

func (tc *testCase) Init(zeroTime int64) {
	if tc.init != nil {
		tc.init(zeroTime)
	}
}

func (tc *testCase) CheckAlerts(ts int64, alerts []v1.Alert) (ok bool, expected []v1.Alert) {
	if tc.checkAlerts != nil {
		return tc.checkAlerts(ts, alerts)
	}
	return false, nil
}

func (tc *testCase) CheckMetrics(ts int64, metrics string) (ok bool, expected string) {
	if tc.checkMetrics != nil {
		return tc.checkMetrics(ts, metrics)
	}
	return false, ""
}

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

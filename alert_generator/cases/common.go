package cases

import (
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
}

// testCase implements TestCase.
// All variables must be initialised.
type testCase struct {
	describe             func() (title string, description string)
	ruleGroup            func() (rulefmt.RuleGroup, error)
	samplesToRemoteWrite func() []prompb.TimeSeries
	init                 func(zeroTime int64)
	testUntil            func() int64
	checkAlerts          func(ts int64, alerts []v1.Alert) error
	checkMetrics         func(ts int64, metrics []promql.Sample) error
}

// This makes sure that it always implements TestCase and help catch regressions
// early during development.
var _ TestCase = &testCase{}

func (tc *testCase) Describe() (groupName string, description string) { return tc.describe() }

func (tc *testCase) RuleGroup() (rulefmt.RuleGroup, error) { return tc.ruleGroup() }

func (tc *testCase) SamplesToRemoteWrite() []prompb.TimeSeries { return tc.samplesToRemoteWrite() }

func (tc *testCase) Init(zeroTime int64) { tc.init(zeroTime) }

func (tc *testCase) TestUntil() int64 { return tc.testUntil() }

func (tc *testCase) CheckAlerts(ts int64, alerts []v1.Alert) error {
	return tc.checkAlerts(ts, alerts)
}

func (tc *testCase) CheckMetrics(ts int64, metrics []promql.Sample) error {
	return tc.checkMetrics(ts, metrics)
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

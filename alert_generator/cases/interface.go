package cases

import (
	"time"

	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/web/api/v1"
)

const (
	ResendDelay = time.Minute

	// MaxRTT is the max request time for alert-generator sending the alert or making GET requests to the API.
	MaxRTT = 5 * time.Second
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

	// CheckRuleGroup returns nil if the rule group provided is as expected at the given timestamp.
	// Returns an error otherwise describing what is the problem.
	// This must be checked with a min interval of the rule group's interval from RuleGroup().
	CheckRuleGroup(ts int64, rg *v1.RuleGroup) error

	// CheckMetrics returns nil if at give timestamp the metrics contain the expected metrics.
	// Returns an error otherwise describing what is the problem.
	// This must be checked with a min interval of the rule group's interval from RuleGroup().
	CheckMetrics(ts int64, metrics []promql.Sample) error

	// ExpectedAlerts returns all the expected alerts that must be received for this test case.
	// This must be called only after Init().
	ExpectedAlerts() []ExpectedAlert
}

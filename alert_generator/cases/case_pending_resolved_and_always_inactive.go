package cases

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/web/api/v1"
	"gopkg.in/yaml.v3"
)

// PendingAndResolved_AlwaysInactive tests the following cases:
// * Alert that goes from pending->inactive.
// * Rule that never becomes active (i.e. alerts in pending or firing).
// * Alert goes into inactive when there is no more data in pending.
func PendingAndResolved_AlwaysInactive() TestCase {
	groupName := "PendingAndResolved_AlwaysInactive"
	pendingAlertName := groupName + "_PendingAlert"
	inactiveAlertName := groupName + "_InactiveAlert"
	pendingLabels := metricLabels(groupName, pendingAlertName)
	inactiveLabels := metricLabels(groupName, inactiveAlertName)
	tc := &pendingAndResolved{
		groupName:            groupName,
		pendingAlertName:     pendingAlertName,
		pendingQuery:         fmt.Sprintf("%s > 10", pendingLabels.String()),
		pendingMetricLabels:  pendingLabels,
		inactiveAlertName:    inactiveAlertName,
		inactiveQuery:        fmt.Sprintf("%s > 99", inactiveLabels.String()),
		inactiveMetricLabels: inactiveLabels,
		rwInterval:           15 * time.Second,
		groupInterval:        30 * time.Second,
	}
	tc.forDuration = model.Duration(12 * tc.rwInterval)
	return tc
}

type pendingAndResolved struct {
	groupName                                 string
	pendingAlertName, inactiveAlertName       string
	pendingQuery, inactiveQuery               string
	pendingMetricLabels, inactiveMetricLabels labels.Labels
	rwInterval, groupInterval                 time.Duration
	forDuration                               model.Duration
	totalSamples                              int

	zeroTime int64
}

func (tc *pendingAndResolved) Describe() (title string, description string) {
	return tc.groupName,
		"(1) Alert that goes from pending->inactive. " +
			"(2) Rule that never becomes active (i.e. alerts in pending or firing)." +
			"(3) Alert goes into inactive when there is no more data in pending."
}

func (tc *pendingAndResolved) RuleGroup() (rulefmt.RuleGroup, error) {
	var pendingAlert, inactiveAlert yaml.Node
	if err := pendingAlert.Encode(tc.pendingAlertName); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	if err := inactiveAlert.Encode(tc.inactiveAlertName); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	var pendingExpr, inactiveExpr yaml.Node
	if err := pendingExpr.Encode(tc.pendingQuery); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	if err := inactiveExpr.Encode(tc.inactiveQuery); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	return rulefmt.RuleGroup{
		Name:     tc.groupName,
		Interval: model.Duration(tc.groupInterval),
		Rules: []rulefmt.RuleNode{
			{ // inactive -> pending -> inactive.
				Alert:       pendingAlert,
				Expr:        pendingExpr,
				For:         tc.forDuration,
				Labels:      map[string]string{"foo": "bar", "rulegroup": tc.groupName},
				Annotations: map[string]string{"description": "SimpleAlert is firing"},
			},
			{ // Always inactive.
				Alert:       inactiveAlert,
				Expr:        inactiveExpr,
				For:         tc.forDuration,
				Labels:      map[string]string{"ba_dum": "tss", "rulegroup": tc.groupName},
				Annotations: map[string]string{"description": "This should never fire"},
			},
		},
	}, nil
}

func (tc *pendingAndResolved) SamplesToRemoteWrite() []prompb.TimeSeries {
	samples := sampleSlice(tc.rwInterval,
		// All comment times is assuming 15s interval.
		"3", "5", "0x2", "9", // 1m (3 is @0 time).
		"0x3", "11", // 1m block. Gets into pending at value 11@2m.
		// Firing after 4m more, so we let it be in pending for 2m30s more, and then inactive again.
		"0x10", // 2m30s.
		// Resolved. 10m more of 9s. Should not get any alerts.
		"9",
	)
	tc.totalSamples = len(samples) + 40 // Check for more time to expect inactive at the end.
	return []prompb.TimeSeries{
		{
			Labels:  toProtoLabels(tc.pendingMetricLabels),
			Samples: samples,
		},
		{
			Labels:  toProtoLabels(tc.inactiveMetricLabels),
			Samples: samples,
		},
	}
}

func (tc *pendingAndResolved) Init(zt int64) {
	tc.zeroTime = zt
}

func (tc *pendingAndResolved) TestUntil() int64 {
	return timestamp.FromTime(timestamp.Time(tc.zeroTime).Add(time.Duration(tc.totalSamples) * tc.rwInterval))
}

func (tc *pendingAndResolved) CheckAlerts(ts int64, alerts []v1.Alert) error {
	expAlerts := tc.expAlerts(ts, alerts)
	return checkExpectedAlerts(expAlerts, alerts, tc.groupInterval)
}

func (tc *pendingAndResolved) CheckRuleGroup(ts int64, rg *v1.RuleGroup) error {
	if ts-tc.zeroTime < 2*int64(tc.groupInterval/time.Millisecond) {
		// We wait till 1 evaluation is done.
		return nil
	}
	if rg == nil {
		return errors.New("no rule group found")
	}
	expRgs := tc.expRuleGroups(ts)
	return checkExpectedRuleGroup(timestamp.Time(ts), expRgs, *rg)
}

func (tc *pendingAndResolved) CheckMetrics(ts int64, samples []promql.Sample) error {
	expSamples := tc.expMetrics(ts)
	return checkExpectedSamples(expSamples, samples)
}

func (tc *pendingAndResolved) expAlerts(ts int64, alerts []v1.Alert) (expAlerts [][]v1.Alert) {
	relTs := ts - tc.zeroTime
	canBeInactive, canBePending := tc.allPossibleStates(relTs)
	activeAt := timestamp.Time(tc.zeroTime + int64(8*tc.rwInterval/time.Millisecond))

	desc := "-----"
	if canBeInactive {
		expAlerts = append(expAlerts, []v1.Alert{})
		desc += "/inactive"
	}
	if canBePending {
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.pendingAlertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
				State:       "pending",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		})
		desc += "/pending"
	}

	// TODO: temporary for development.
	devPrint(desc, alerts)

	return expAlerts
}

func (tc *pendingAndResolved) expRuleGroups(ts int64) (expRgs []v1.RuleGroup) {
	relTs := ts - tc.zeroTime
	canBeInactive, canBePending := tc.allPossibleStates(relTs)
	activeAt := timestamp.Time(tc.zeroTime + int64(8*tc.rwInterval/time.Millisecond))

	getRg := func(state string, alerts []*v1.Alert) v1.RuleGroup {
		return v1.RuleGroup{
			Name:     tc.groupName,
			Interval: float64(tc.groupInterval / time.Second),
			Rules: []v1.Rule{
				v1.AlertingRule{
					State:       state,
					Name:        tc.pendingAlertName,
					Query:       tc.pendingQuery,
					Duration:    float64(time.Duration(tc.forDuration) / time.Second),
					Labels:      labels.FromStrings("foo", "bar", "rulegroup", tc.groupName),
					Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
					Alerts:      alerts,
					Health:      "ok",
					Type:        "alerting",
				},
				v1.AlertingRule{
					State:       "inactive",
					Name:        tc.inactiveAlertName,
					Query:       tc.inactiveQuery,
					Duration:    float64(time.Duration(tc.forDuration) / time.Second),
					Labels:      labels.FromStrings("ba_dum", "tss", "rulegroup", tc.groupName),
					Annotations: labels.FromStrings("description", "This should never fire"),
					Health:      "ok",
					Type:        "alerting",
				},
			},
		}
	}

	if canBeInactive {
		expRgs = append(expRgs, getRg("inactive", nil))
	}
	if canBePending {
		expRgs = append(expRgs, getRg("pending", []*v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.pendingAlertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
				State:       "pending",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		}))
	}

	return expRgs
}

func (tc *pendingAndResolved) expMetrics(ts int64) (expSamples [][]promql.Sample) {
	relTs := ts - tc.zeroTime
	canBeInactive, canBePending := tc.allPossibleStates(relTs)

	if canBeInactive {
		expSamples = append(expSamples, nil)
	}
	if canBePending {
		expSamples = append(expSamples, []promql.Sample{
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "pending", "alertname", tc.pendingAlertName, "foo", "bar", "rulegroup", tc.groupName),
			},
		})
	}

	return expSamples
}

// ts is relative time w.r.t. zeroTime.
func (tc *pendingAndResolved) allPossibleStates(ts int64) (canBeInactive, canBePending bool) {
	between := betweenFunc(ts)

	rwItvlSecFloat, grpItvlSecFloat := float64(tc.rwInterval/time.Second), float64(tc.groupInterval/time.Second)
	_8th := 8 * rwItvlSecFloat   // Goes into pending.
	_19th := 19 * rwItvlSecFloat // Becomes inactive.
	canBeInactive = between(0, _8th+grpItvlSecFloat) || between(_19th, 240*rwItvlSecFloat)
	canBePending = between(_8th-1, _19th+grpItvlSecFloat)
	return
}

func (tc *pendingAndResolved) ExpectedAlerts() []ExpectedAlert {
	// We expect no alerts to be sent.
	return nil
}

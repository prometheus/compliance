package cases

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/web/api/v1"
	"gopkg.in/yaml.v3"
)

func PendingAndFiringAndResolved() TestCase {
	groupName := "PendingAndFiringAndResolved"
	alertName := groupName + "_SimpleAlert"
	lbls := metricLabels(groupName, alertName)
	query := fmt.Sprintf("%s > 10", lbls.String())
	tc := &pendingAndFiringAndResolved{
		groupName:    groupName,
		alertName:    alertName,
		query:        query,
		metricLabels: lbls,
		// TODO: make this 15 and 30 for final use.
		rwInterval:    5 * time.Second,
		groupInterval: 10 * time.Second,
	}
	tc.forDuration = model.Duration(12 * tc.rwInterval)
	return tc
}

type pendingAndFiringAndResolved struct {
	groupName                 string
	alertName                 string
	query                     string
	metricLabels              labels.Labels
	rwInterval, groupInterval time.Duration
	forDuration               model.Duration
	totalSamples              int

	zeroTime int64
}

func (tc *pendingAndFiringAndResolved) Describe() (title string, description string) {
	return tc.groupName,
		"An alert goes from pending to firing to resolved state and stays in resolved state"
}

func (tc *pendingAndFiringAndResolved) RuleGroup() (rulefmt.RuleGroup, error) {
	var alert yaml.Node
	var expr yaml.Node
	if err := alert.Encode(tc.alertName); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	if err := expr.Encode(tc.query); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	return rulefmt.RuleGroup{
		Name:     tc.groupName,
		Interval: model.Duration(tc.groupInterval),
		Rules: []rulefmt.RuleNode{
			{
				Alert:       alert,
				Expr:        expr,
				For:         tc.forDuration,
				Labels:      map[string]string{"foo": "bar", "rulegroup": tc.groupName},
				Annotations: map[string]string{"description": "SimpleAlert is firing"},
			},
		},
	}, nil
}

func (tc *pendingAndFiringAndResolved) SamplesToRemoteWrite() []prompb.TimeSeries {
	samples := sampleSlice(tc.rwInterval,
		// All comment times is assuming 15s interval.
		"3", "5", "0x2", "9", // 1m (3 is @0 time).
		"0x3", "11", // 1m block. Gets into pending at value 11@2m.
		// 6m more of this, upto end of 8m.
		// Firing at 5m hence should get min 2 alerts, one after resend delay of 1m.
		"0x24", // 6m.
		// Resolved at 8m15s.
		// 20m more of 9s. Hence must get multiple resolved alert but not after 15m of being resolved.
		"9", "0x79",
	)
	tc.totalSamples = len(samples)
	return []prompb.TimeSeries{
		{
			Labels:  toProtoLabels(tc.metricLabels),
			Samples: samples,
		},
	}
}

func (tc *pendingAndFiringAndResolved) Init(zt int64) {
	tc.zeroTime = zt
}

func (tc *pendingAndFiringAndResolved) TestUntil() int64 {
	return timestamp.FromTime(timestamp.Time(tc.zeroTime).Add(time.Duration(tc.totalSamples) * tc.rwInterval))
}

func (tc *pendingAndFiringAndResolved) CheckAlerts(ts int64, alerts []v1.Alert) error {
	expAlerts := tc.expAlerts(ts, alerts)
	return checkExpectedAlerts(expAlerts, alerts, tc.groupInterval)
}

func (tc *pendingAndFiringAndResolved) CheckRuleGroup(ts int64, rg *v1.RuleGroup) error {
	if ts-tc.zeroTime < int64(tc.groupInterval/time.Millisecond) {
		// We wait till 1 evaluation is done.
		return nil
	}
	if rg == nil {
		return errors.New("no rule group found")
	}
	expRgs := tc.expRuleGroups(ts)
	return checkExpectedRuleGroup(timestamp.Time(ts), expRgs, *rg)
}

func (tc *pendingAndFiringAndResolved) CheckMetrics(ts int64, samples []promql.Sample) error {
	expSamples := tc.expMetrics(ts)
	return checkExpectedSamples(expSamples, samples)
}

func (tc *pendingAndFiringAndResolved) expAlerts(ts int64, alerts []v1.Alert) (expAlerts [][]v1.Alert) {
	relTs := ts - tc.zeroTime
	canBeInactive, canBePending, canBeFiring := tc.allPossibleStates(relTs)
	activeAt := timestamp.Time(tc.zeroTime + int64(8*tc.rwInterval/time.Millisecond))

	desc := "-----"
	if canBeInactive {
		expAlerts = append(expAlerts, []v1.Alert{})
		desc += "/inactive"
	}
	if canBePending {
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
				State:       "pending",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		})
		desc += "/pending"
	}
	if canBeFiring {
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
				State:       "firing",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		})
		desc += "/firing"
	}

	// TODO: temporary for development.
	fmt.Println(desc, alerts)

	return expAlerts
}

func (tc *pendingAndFiringAndResolved) expRuleGroups(ts int64) (expRgs []v1.RuleGroup) {
	relTs := ts - tc.zeroTime
	canBeInactive, canBePending, canBeFiring := tc.allPossibleStates(relTs)
	activeAt := timestamp.Time(tc.zeroTime + int64(8*tc.rwInterval/time.Millisecond))

	getRg := func(state string, alerts []*v1.Alert) v1.RuleGroup {
		return v1.RuleGroup{
			Name:     tc.groupName,
			Interval: float64(tc.groupInterval / time.Second),
			Rules: []v1.Rule{
				v1.AlertingRule{
					State:       state,
					Name:        tc.alertName,
					Query:       tc.query,
					Duration:    float64(time.Duration(tc.forDuration) / time.Second),
					Labels:      labels.FromStrings("foo", "bar", "rulegroup", tc.groupName),
					Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
					Alerts:      alerts,
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
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
				State:       "pending",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		}))
	}
	if canBeFiring {
		expRgs = append(expRgs, getRg("firing", []*v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
				State:       "firing",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		}))
	}

	return expRgs
}

func (tc *pendingAndFiringAndResolved) expMetrics(ts int64) (expSamples [][]promql.Sample) {
	relTs := ts - tc.zeroTime
	canBeInactive, canBePending, canBeFiring := tc.allPossibleStates(relTs)

	if canBeInactive {
		expSamples = append(expSamples, nil)
	}
	if canBePending {
		expSamples = append(expSamples, []promql.Sample{
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "pending", "alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
			},
		})
	}
	if canBeFiring {
		expSamples = append(expSamples, []promql.Sample{
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "firing", "alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
			},
		})
	}

	return expSamples
}

// ts is relative time w.r.t. zeroTime.
func (tc *pendingAndFiringAndResolved) allPossibleStates(ts int64) (canBeInactive, canBePending, canBeFiring bool) {
	between := betweenFunc(ts)

	rwItvlSecFloat, grpItvlSecFloat := float64(tc.rwInterval/time.Second), float64(tc.groupInterval/time.Second)
	_8th := 8 * rwItvlSecFloat   // Goes into pending.
	_20th := 20 * rwItvlSecFloat // Goes into firing.
	_33rd := 33 * rwItvlSecFloat // Resolved.
	canBeInactive = between(0, _8th+grpItvlSecFloat) || between(_33rd, 240*rwItvlSecFloat)
	canBePending = between(_8th-1, _20th+grpItvlSecFloat)
	canBeFiring = between(_20th-1, _33rd+grpItvlSecFloat)
	return
}

func (tc *pendingAndFiringAndResolved) ExpectedAlerts() []ExpectedAlert {
	_20th := 20 * int64(tc.rwInterval/time.Millisecond) // Firing.
	_33rd := 33 * int64(tc.rwInterval/time.Millisecond) // Resolved.
	_33rdPlus15m := _33rd + int64(15*time.Minute/time.Millisecond)

	var exp []ExpectedAlert
	endsAtDelta := 4 * ResendDelay
	if endsAtDelta < 4*tc.groupInterval {
		endsAtDelta = 4 * tc.groupInterval
	}

	// TODO: there is a bug which is making firing alert to be detected as "missed" when it is
	// already resolved. This is randomly occurring. Maybe some tracking error in the server.
	resendDelayMs := int64(ResendDelay / time.Millisecond)
	for ts := _20th; ts < _33rd; ts += resendDelayMs {
		exp = append(exp, ExpectedAlert{
			OrderingID:    int(ts),
			TimeTolerance: tc.groupInterval,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      false,
			Resend:        ts != _20th,
			ResolvedTime:  timestamp.Time(tc.zeroTime + _33rd),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
				StartsAt:    timestamp.Time(tc.zeroTime + _20th),
			},
		})
	}

	for ts := _33rd; ts < _33rdPlus15m; ts += resendDelayMs {
		tolerance := tc.groupInterval
		if ts == _33rd {
			// Since the alert state is reset, the alert sent time for resolved alert can be upto
			// 1 groupInterval late compared to actual time when it gets resolved. So we need to
			// account for this delay plus the usual tolerance.
			// We don't change tolerance for other resolved alerts because their Ts will be adjusted
			// based on this first resolved alert.
			tolerance = 2 * tc.groupInterval
		}
		exp = append(exp, ExpectedAlert{
			OrderingID:    int(ts),
			TimeTolerance: tolerance,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      true,
			Resend:        ts != _33rd,
			ResolvedTime:  timestamp.Time(tc.zeroTime + _33rd),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
				StartsAt:    timestamp.Time(tc.zeroTime + _20th),
			},
		})
	}

	return exp
}

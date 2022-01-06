package cases

import (
	"fmt"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/web/api/v1"
	"gopkg.in/yaml.v3"
)

func PendingAndFiringAndResolved() TestCase {
	groupName := "PendingAndFiringAndResolved"
	alertName := groupName + "_SimpleAlert"
	return &pendingAndFiringAndResolved{
		groupName: groupName,
		alertName: alertName,
		lbls:      baseLabels(groupName, alertName),
		// TODO: make this 15 and 30 for final use.
		rwInterval:    15 * time.Second,
		groupInterval: 30 * time.Second,
	}
}

type pendingAndFiringAndResolved struct {
	groupName                 string
	alertName                 string
	lbls                      labels.Labels
	rwInterval, groupInterval time.Duration

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
	if err := expr.Encode(fmt.Sprintf("%s > 10", tc.lbls.String())); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	return rulefmt.RuleGroup{
		Name:     tc.groupName,
		Interval: model.Duration(tc.groupInterval),
		Rules: []rulefmt.RuleNode{
			{
				Alert:       alert,
				Expr:        expr,
				For:         model.Duration(12 * tc.rwInterval),
				Labels:      map[string]string{"foo": "bar", "rulegroup": tc.groupName},
				Annotations: map[string]string{"description": "SimpleAlert is firing"},
			},
		},
	}, nil
}

func (tc *pendingAndFiringAndResolved) SamplesToRemoteWrite() []prompb.TimeSeries {
	// TODO: consider using the `load 15s metric 1+1x5` etc notation used in Prometheus tests.
	return []prompb.TimeSeries{
		{
			Labels: toProtoLabels(tc.lbls),
			Samples: sampleSlice(tc.rwInterval,
				// All comment times is assuming 15s interval.
				3, 5, 5, 5, 9, // 1m (3 is @0 time).
				9, 9, 9, 11, // 1m block. Gets into pending at value 11@2m.
				// 6m more of this, upto end of 8m.
				// Firing at 5m hence should get min 2 alerts, one after resend delay of 1m.
				11, 11, 11, 11, 11, 11, 11, 11, 11, 11, 11, 11, // 3m block.
				11, 11, 11, 11, 11, 11, 11, 11, 11, 11, 11, 11, // 3m block.
				// Resolved at 8m15s.
				// 18m more of 9s. Hence must get multiple resolved alert but not after 15m of being resolved.
				9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, // 5m block.
				9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, // 5m block.
				9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, // 5m block.
				9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, // 3m block.
			),
		},
	}
}

func (tc *pendingAndFiringAndResolved) Init(zt int64) {
	tc.zeroTime = zt
}

func (tc *pendingAndFiringAndResolved) TestUntil() int64 {
	return timestamp.FromTime(timestamp.Time(tc.zeroTime).Add(118 * tc.rwInterval))
}

func (tc *pendingAndFiringAndResolved) CheckAlerts(ts int64, alerts []v1.Alert) error {
	expAlerts, expRanges, _ := tc.expAlertsMetricsRules(ts, alerts)
	return checkAllPossibleExpectedAlerts(expAlerts, expRanges, alerts)
}

func (tc *pendingAndFiringAndResolved) CheckMetrics(ts int64, samples []promql.Sample) error {
	_, _, expSamples := tc.expAlertsMetricsRules(ts, nil)
	return checkAllPossibleExpectedSamples(expSamples, samples)
}

func (tc *pendingAndFiringAndResolved) expAlertsMetricsRules(ts int64, alerts []v1.Alert) (expAlerts [][]v1.Alert, expActiveAtRanges [][][2]time.Time, expSamples [][]promql.Sample) {
	relTs := ts - tc.zeroTime
	inactive, maybePending, pending, maybeFiring, firing, maybeResolved, resolved := tc.allPossibleStates(relTs)

	activeAtRange := convertRelativeToAbsoluteTimes(tc.zeroTime, [][2]time.Duration{
		{8 * tc.rwInterval, (8 * tc.rwInterval) + tc.groupInterval},
	})

	pendingAlerts := []v1.Alert{
		{
			Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
			Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
			State:       "pending",
			Value:       "11",
		},
	}
	pendingSample := []promql.Sample{
		{
			Point:  promql.Point{T: ts / 1000, V: 1},
			Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "pending", "alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
		},
	}

	firingAlerts := []v1.Alert{
		{
			Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
			Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
			State:       "firing",
			Value:       "11",
		},
	}
	firingSample := []promql.Sample{
		{
			Point:  promql.Point{T: ts / 1000, V: 1},
			Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "firing", "alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
		},
	}

	fmt.Printf("\n")
	if inactive || maybePending || maybeResolved || resolved {
		expAlerts = append(expAlerts, []v1.Alert{})
		expActiveAtRanges = append(expActiveAtRanges, nil)
		expSamples = append(expSamples, nil)
	}
	if maybePending || pending || maybeFiring {
		expAlerts = append(expAlerts, pendingAlerts)
		expActiveAtRanges = append(expActiveAtRanges, activeAtRange)
		expSamples = append(expSamples, pendingSample)
	}
	if maybeFiring || firing || maybeResolved {
		expAlerts = append(expAlerts, firingAlerts)
		expActiveAtRanges = append(expActiveAtRanges, activeAtRange)
		expSamples = append(expSamples, firingSample)
	}

	switch {
	case inactive:
		fmt.Println("inactive", alerts)
	case maybePending:
		fmt.Println("maybePending", alerts)
	case pending:
		fmt.Println("pending", alerts)
	case maybeFiring:
		fmt.Println("maybeFiring", alerts)
	case firing:
		fmt.Println("firing", alerts)
	case maybeResolved:
		fmt.Println("maybeResolved", alerts)
	case resolved:
		fmt.Println("resolved", alerts)
	// TODO: there should be no alerts found after a point.
	default:
	}

	return expAlerts, expActiveAtRanges, expSamples
}

// ts is relative time w.r.t. zeroTime.
func (tc *pendingAndFiringAndResolved) allPossibleStates(ts int64) (inactive, maybePending, pending, maybeFiring, firing, maybeResolved, resolved bool) {
	between := betweenFunc(ts)

	rwItvlSecFloat, grpItvlSecFloat := float64(tc.rwInterval/time.Second), float64(tc.groupInterval/time.Second)
	_8th := 8 * rwItvlSecFloat   // Goes into pending.
	_20th := 20 * rwItvlSecFloat // Goes into firing.
	_33rd := 33 * rwItvlSecFloat // Resolved.
	inactive = between(0, _8th-1)
	maybePending = between(_8th-1, _8th+grpItvlSecFloat)
	pending = between(_8th+grpItvlSecFloat, _20th-1)
	maybeFiring = between(_20th-1, _20th+grpItvlSecFloat)
	firing = between(_20th+grpItvlSecFloat, _33rd-1)
	maybeResolved = between(_33rd-1, _33rd+grpItvlSecFloat)
	resolved = between(_33rd+grpItvlSecFloat, 240*rwItvlSecFloat)
	return
}

func (tc *pendingAndFiringAndResolved) ExpectedAlerts() []ExpectedAlert {
	_20th := 20 * int64(tc.rwInterval/time.Millisecond) // Firing.
	_33rd := 33 * int64(tc.rwInterval/time.Millisecond) // Resolved.
	_33rd_plus_15m := _33rd + int64(15*time.Minute/time.Millisecond)

	var exp []ExpectedAlert
	endsAtDelta := 4 * ResendDelay
	if endsAtDelta < 4*tc.groupInterval {
		endsAtDelta = 4 * tc.groupInterval
	}

	resendDelayMs := int64(ResendDelay / time.Millisecond)
	for ts := _20th; ts < _33rd; ts += resendDelayMs {
		exp = append(exp, ExpectedAlert{
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

	for ts := _33rd; ts < _33rd_plus_15m; ts += resendDelayMs {
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

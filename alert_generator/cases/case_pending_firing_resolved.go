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

// PendingAndFiringAndResolved tests the following cases:
// * Alert that goes from pending->firing->inactive.
// * pending alerts having changing annotation values (checked via API).
// * firing and inactive alerts being sent when they first went into those states.
// * firing alert being re-sent at expected intervals when the alert is active with changing annotation contents.
// * inactive alert being re-sent at expected intervals up to a certain time and not after that.
// * Alert that becomes active after having fired already and gone into inactive state where 'for' duration is non zero where inactive alert was still being sent.
func PendingAndFiringAndResolved() TestCase {
	groupName := "PendingAndFiringAndResolved"
	alertName := groupName + "_SimpleAlert"
	lbls := metricLabels(groupName, alertName)
	query := fmt.Sprintf("%s > 10", lbls.String())
	tc := &pendingAndFiringAndResolved{
		groupName:     groupName,
		alertName:     alertName,
		query:         query,
		metricLabels:  lbls,
		rwInterval:    15 * time.Second,
		groupInterval: 30 * time.Second,
	}
	tc.forDuration = model.Duration(24 * tc.rwInterval)
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
		"(1) Alert that goes from pending->firing->inactive. " +
			"(2) pending alerts having changing annotation values (checked via API). " +
			"(3) firing and inactive alerts being sent when they first went into those states. " +
			"(4) firing alert being re-sent at expected intervals when the alert is active with changing annotation contents. " +
			"(5) inactive alert being re-sent at expected intervals up to a certain time and not after that. " +
			"(6) Alert that becomes active after having fired already and gone into inactive state where 'for' duration is non zero where inactive alert was still being sent."
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
				Alert:  alert,
				Expr:   expr,
				For:    tc.forDuration,
				Labels: map[string]string{"foo": "bar", "rulegroup": tc.groupName},
				Annotations: map[string]string{
					"description": "SimpleAlert is firing",
					"summary":     "The value is {{$value}} {{.Value}}",
				},
			},
		},
	}, nil
}

func (tc *pendingAndFiringAndResolved) SamplesToRemoteWrite() []prompb.TimeSeries {
	samples := sampleSlice(tc.rwInterval,
		// All comment times is assuming 15s interval.
		"3", "5", "0x2", "9", // 1m (3 is @0 time).
		"0x3", "11", // 1m block. Gets into pending at value 11@2m.
		// 15m more of active state. Firing at 8m hence should get min 2 alerts, one after resend delay of 1m.
		"0x12",       // 3m.
		"15", "0x11", // 3m of changed value. Goes into firing at the end here.
		"0x20",       // 5m.
		"19", "0x15", // 4m of changed value.
		// Resolved.
		// 5m more of 9s. Hence must get multiple resolved alerts.
		"9", "0x19",
		// Pending again.
		"15", "0x24", // 6m. Goes into firing at the end here.
		"0x20",      // 5m. Firing.
		"8", "0x15", // 4m of resolved.
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

func (tc *pendingAndFiringAndResolved) CheckMetrics(ts int64, samples []promql.Sample) error {
	expSamples := tc.expMetrics(ts)
	return checkExpectedSamples(expSamples, samples)
}

func (tc *pendingAndFiringAndResolved) expAlerts(ts int64, alerts []v1.Alert) (expAlerts [][]v1.Alert) {
	relTs := ts - tc.zeroTime
	canBeInactive, canBePending1, canBePending2, canBeFiring1, canBeFiring2, pendingAgain, firingAgain := tc.allPossibleStates(relTs)
	activeAt := timestamp.Time(tc.zeroTime + int64(8*tc.rwInterval/time.Millisecond))
	activeAt2 := timestamp.Time(tc.zeroTime + int64(89*tc.rwInterval/time.Millisecond))

	desc := "-----"
	if canBeInactive {
		expAlerts = append(expAlerts, []v1.Alert{})
		desc += "/inactive"
	}
	if canBePending1 {
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 11 11"),
				State:       "pending",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		})
		desc += "/pending"
	}
	if canBePending2 || pendingAgain {
		aa := activeAt
		if pendingAgain {
			aa = activeAt2
		}
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 15 15"),
				State:       "pending",
				Value:       "15",
				ActiveAt:    &aa,
			},
		})
		desc += "/pending"
	}
	if canBeFiring1 || firingAgain {
		aa := activeAt
		if firingAgain {
			aa = activeAt2
		}
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 15 15"),
				State:       "firing",
				Value:       "15",
				ActiveAt:    &aa,
			},
		})
		desc += "/firing"
	}
	if canBeFiring2 {
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 19 19"),
				State:       "firing",
				Value:       "19",
				ActiveAt:    &activeAt,
			},
		})
		desc += "/firing"
	}

	// TODO: temporary for development.
	devPrint(desc, alerts)

	return expAlerts
}

func (tc *pendingAndFiringAndResolved) expRuleGroups(ts int64) (expRgs []v1.RuleGroup) {
	relTs := ts - tc.zeroTime
	canBeInactive, canBePending1, canBePending2, canBeFiring1, canBeFiring2, pendingAgain, firingAgain := tc.allPossibleStates(relTs)
	activeAt := timestamp.Time(tc.zeroTime + int64(8*tc.rwInterval/time.Millisecond))
	activeAt2 := timestamp.Time(tc.zeroTime + int64(89*tc.rwInterval/time.Millisecond))

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
					Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is {{$value}} {{.Value}}"),
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
	if canBePending1 {
		expRgs = append(expRgs, getRg("pending", []*v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 11 11"),
				State:       "pending",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		}))
	}
	if canBePending2 || pendingAgain {
		aa := activeAt
		if pendingAgain {
			aa = activeAt2
		}
		expRgs = append(expRgs, getRg("pending", []*v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 15 15"),
				State:       "pending",
				Value:       "15",
				ActiveAt:    &aa,
			},
		}))
	}
	if canBeFiring1 || firingAgain {
		aa := activeAt
		if firingAgain {
			aa = activeAt2
		}
		expRgs = append(expRgs, getRg("firing", []*v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 15 15"),
				State:       "firing",
				Value:       "15",
				ActiveAt:    &aa,
			},
		}))
	}
	if canBeFiring2 {
		expRgs = append(expRgs, getRg("firing", []*v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 19 19"),
				State:       "firing",
				Value:       "19",
				ActiveAt:    &activeAt,
			},
		}))
	}

	return expRgs
}

func (tc *pendingAndFiringAndResolved) expMetrics(ts int64) (expSamples [][]promql.Sample) {
	relTs := ts - tc.zeroTime
	canBeInactive, canBePending1, canBePending2, canBeFiring1, canBeFiring2, pendingAgain, firingAgain := tc.allPossibleStates(relTs)

	if canBeInactive {
		expSamples = append(expSamples, nil)
	}
	if canBePending1 || canBePending2 || pendingAgain {
		expSamples = append(expSamples, []promql.Sample{
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "pending", "alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
			},
		})
	}
	if canBeFiring1 || canBeFiring2 || firingAgain {
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
func (tc *pendingAndFiringAndResolved) allPossibleStates(ts int64) (
	canBeInactive bool,
	canBePending1, canBePending2 bool,
	canBeFiring1, canBeFiring2 bool,
	pendingAgain, firingAgain bool,
) {
	between := betweenFunc(ts)

	rwItvlSecFloat, grpItvlSecFloat := float64(tc.rwInterval/time.Second), float64(tc.groupInterval/time.Second)
	_8th := 8 * rwItvlSecFloat     // Goes into pending.
	_21st := 21 * rwItvlSecFloat   // Pending, but another value.
	_32nd := 32 * rwItvlSecFloat   // Goes into firing.
	_53rd := 53 * rwItvlSecFloat   // Firing, but another value.
	_69th := 69 * rwItvlSecFloat   // Resolved.
	_89th := 89 * rwItvlSecFloat   // Pending again.
	_113th := 113 * rwItvlSecFloat // Firing again.
	_134th := 134 * rwItvlSecFloat // Resolved again.
	canBeInactive = between(0, _8th+grpItvlSecFloat) ||
		between(_69th-1, _89th+grpItvlSecFloat) ||
		between(_134th, 240*rwItvlSecFloat)
	canBePending1 = between(_8th-1, _21st+grpItvlSecFloat)
	canBePending2 = between(_21st-1, _32nd+grpItvlSecFloat)
	canBeFiring1 = between(_32nd-1, _53rd+grpItvlSecFloat)
	canBeFiring2 = between(_53rd-1, _69th+grpItvlSecFloat)

	pendingAgain = between(_89th-1, _113th+grpItvlSecFloat)
	firingAgain = between(_113th-1, _134th+grpItvlSecFloat)
	return
}

func (tc *pendingAndFiringAndResolved) ExpectedAlerts() []ExpectedAlert {
	_32nd := 32 * int64(tc.rwInterval/time.Millisecond)   // Firing.
	_53rd := 53 * int64(tc.rwInterval/time.Millisecond)   // Firing with value change.
	_69th := 69 * int64(tc.rwInterval/time.Millisecond)   // Resolved.
	_89th := 89 * int64(tc.rwInterval/time.Millisecond)   // Pending again.
	_113th := 113 * int64(tc.rwInterval/time.Millisecond) // Firing again.
	_134th := 134 * int64(tc.rwInterval/time.Millisecond) // Resolved again.
	_134thPlus15m := _134th + int64(15*time.Minute/time.Millisecond)

	var exp []ExpectedAlert
	endsAtDelta := 4 * ResendDelay
	if endsAtDelta < 4*tc.groupInterval {
		endsAtDelta = 4 * tc.groupInterval
	}

	orderingID := 0
	addAlert := func(ea ExpectedAlert) {
		orderingID++
		ea.OrderingID = orderingID
		exp = append(exp, ea)
	}

	resendDelayMs := int64(ResendDelay / time.Millisecond)
	for ts := _32nd; ts < _53rd; ts += resendDelayMs {
		addAlert(ExpectedAlert{
			TimeTolerance: tc.groupInterval,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      false,
			Resend:        ts != _32nd,
			NextState:     timestamp.Time(tc.zeroTime + _53rd),
			ResolvedTime:  timestamp.Time(tc.zeroTime + _69th),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 15 15"),
				StartsAt:    timestamp.Time(tc.zeroTime + _32nd),
			},
		})
	}
	// Value change.
	for ts := _53rd; ts < _69th; ts += resendDelayMs {
		addAlert(ExpectedAlert{
			TimeTolerance: tc.groupInterval,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      false,
			Resend:        true,
			NextState:     timestamp.Time(tc.zeroTime + _69th),
			ResolvedTime:  timestamp.Time(tc.zeroTime + _69th),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 19 19"),
				StartsAt:    timestamp.Time(tc.zeroTime + _32nd),
			},
		})
	}

	for ts := _69th; ts < _89th; ts += resendDelayMs {
		tolerance := tc.groupInterval
		if ts == _69th {
			// Since the alert state is reset, the alert sent time for resolved alert can be upto
			// 1 groupInterval late compared to actual time when it gets resolved. So we need to
			// account for this delay plus the usual tolerance.
			// We don't change tolerance for other resolved alerts because their Ts will be adjusted
			// based on this first resolved alert.
			tolerance = 2 * tc.groupInterval
		}
		addAlert(ExpectedAlert{
			TimeTolerance: tolerance,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      true,
			Resend:        ts != _69th,
			NextState:     timestamp.Time(tc.zeroTime + _89th),
			ResolvedTime:  timestamp.Time(tc.zeroTime + _69th),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 19 19"),
				StartsAt:    timestamp.Time(tc.zeroTime + _32nd),
			},
		})
	}

	// Firing again.
	for ts := _113th; ts < _134th; ts += resendDelayMs {
		addAlert(ExpectedAlert{
			TimeTolerance: tc.groupInterval,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      false,
			Resend:        ts != _113th,
			NextState:     timestamp.Time(tc.zeroTime + _134th),
			ResolvedTime:  timestamp.Time(tc.zeroTime + _134th),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 15 15"),
				StartsAt:    timestamp.Time(tc.zeroTime + _113th),
			},
		})
	}

	for ts := _134th; ts < _134thPlus15m; ts += resendDelayMs {
		tolerance := tc.groupInterval
		if ts == _134th {
			// Since the alert state is reset, the alert sent time for resolved alert can be upto
			// 1 groupInterval late compared to actual time when it gets resolved. So we need to
			// account for this delay plus the usual tolerance.
			// We don't change tolerance for other resolved alerts because their Ts will be adjusted
			// based on this first resolved alert.
			tolerance = 2 * tc.groupInterval
		}
		addAlert(ExpectedAlert{
			TimeTolerance: tolerance,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      true,
			Resend:        ts != _134th,
			ResolvedTime:  timestamp.Time(tc.zeroTime + _134th),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.alertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing", "summary", "The value is 15 15"),
				StartsAt:    timestamp.Time(tc.zeroTime + _113th),
			},
		})
	}

	return exp
}

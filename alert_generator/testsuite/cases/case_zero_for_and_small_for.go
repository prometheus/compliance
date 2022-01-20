package cases

import (
	"fmt"
	"github.com/prometheus/prometheus/notifier"
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

func ZeroFor_SmallFor() TestCase {
	groupName := "ZeroFor_SmallFor"
	zfAlertName := groupName + "_ZeroFor"
	sfAlertName := groupName + "_SmallFor"
	zfLabels := metricLabels(groupName, zfAlertName)
	sfLabels := metricLabels(groupName, sfAlertName)
	tc := &zeroAndSmallFor{
		groupName:      groupName,
		zfAlertName:    zfAlertName,
		zfQuery:        fmt.Sprintf("%s > 10", zfLabels.String()),
		zfMetricLabels: zfLabels,
		sfAlertName:    sfAlertName,
		sfQuery:        fmt.Sprintf("%s > 10", sfLabels.String()),
		sfMetricLabels: sfLabels,
		// TODO: make this 15 and 30 for final use.
		rwInterval:    5 * time.Second,
		groupInterval: 10 * time.Second,
	}
	tc.forDuration = model.Duration(tc.groupInterval / 2)
	return tc
}

type zeroAndSmallFor struct {
	groupName                      string
	zfAlertName, sfAlertName       string
	zfQuery, sfQuery               string
	zfMetricLabels, sfMetricLabels labels.Labels
	rwInterval, groupInterval      time.Duration
	forDuration                    model.Duration // For the "small for".
	totalSamples                   int

	zeroTime int64
}

func (tc *zeroAndSmallFor) Describe() (title string, description string) {
	return tc.groupName,
		"An alert goes from pending to firing to resolved state and stays in resolved state"
}

func (tc *zeroAndSmallFor) RuleGroup() (rulefmt.RuleGroup, error) {
	var zfAlert, sfAlert yaml.Node
	if err := zfAlert.Encode(tc.zfAlertName); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	if err := sfAlert.Encode(tc.sfAlertName); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	var zfExpr, sfExpr yaml.Node
	if err := zfExpr.Encode(tc.zfQuery); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	if err := sfExpr.Encode(tc.sfQuery); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	return rulefmt.RuleGroup{
		Name:     tc.groupName,
		Interval: model.Duration(tc.groupInterval),
		Rules: []rulefmt.RuleNode{
			{ // Zero for.
				Alert:       zfAlert,
				Expr:        zfExpr,
				Labels:      map[string]string{"foo": "bar", "rulegroup": tc.groupName},
				Annotations: map[string]string{"description": "This should immediately fire"},
			},
			{ // Small for.
				Alert:       sfAlert,
				Expr:        sfExpr,
				For:         tc.forDuration,
				Labels:      map[string]string{"ba_dum": "tss", "rulegroup": tc.groupName},
				Annotations: map[string]string{"description": "This should fire after an interval"},
			},
		},
	}, nil
}

func (tc *zeroAndSmallFor) SamplesToRemoteWrite() []prompb.TimeSeries {
	samples := sampleSlice(tc.rwInterval,
		// All comment times is assuming 15s interval.
		"3", "5", "0x2", "9", // 1m (3 is @0 time).
		"0x3", "11", // 1m block. Gets into firing or pending at value 11@2m.
		"0x12", // 3m of active state.
		// Resolved. 10m more of 9s. Should not get any alerts.
		"9", "0x39",
	)
	tc.totalSamples = len(samples)
	return []prompb.TimeSeries{
		{
			Labels:  toProtoLabels(tc.zfMetricLabels),
			Samples: samples,
		},
		{
			Labels:  toProtoLabels(tc.sfMetricLabels),
			Samples: samples,
		},
	}
}

func (tc *zeroAndSmallFor) Init(zt int64) {
	tc.zeroTime = zt
}

func (tc *zeroAndSmallFor) TestUntil() int64 {
	return timestamp.FromTime(timestamp.Time(tc.zeroTime).Add(time.Duration(tc.totalSamples) * tc.rwInterval))
}

func (tc *zeroAndSmallFor) CheckAlerts(ts int64, alerts []v1.Alert) error {
	expAlerts := tc.expAlerts(ts, alerts)
	return checkExpectedAlerts(expAlerts, alerts, tc.groupInterval)
}

func (tc *zeroAndSmallFor) CheckRuleGroup(ts int64, rg *v1.RuleGroup) error {
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

func (tc *zeroAndSmallFor) CheckMetrics(ts int64, samples []promql.Sample) error {
	expSamples := tc.expMetrics(ts)
	return checkExpectedSamples(expSamples, samples)
}

func (tc *zeroAndSmallFor) expAlerts(ts int64, alerts []v1.Alert) (expAlerts [][]v1.Alert) {
	relTs := ts - tc.zeroTime
	canBeInactive, zfFiring, sfPending, sfFiring := tc.allPossibleStates(relTs)
	activeAt := timestamp.Time(tc.zeroTime + int64(8*tc.rwInterval/time.Millisecond))

	desc := "-----"
	if canBeInactive {
		expAlerts = append(expAlerts, []v1.Alert{})
		desc += "/inactive"
	}
	if zfFiring && sfPending {
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.zfAlertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "This should immediately fire"),
				State:       "firing",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
			{
				Labels:      labels.FromStrings("alertname", tc.sfAlertName, "ba_dum", "tss", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "This should fire after an interval"),
				State:       "pending",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		})
		desc += "/firing/pending"
	}
	if zfFiring && sfFiring {
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.zfAlertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "This should immediately fire"),
				State:       "firing",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
			{
				Labels:      labels.FromStrings("alertname", tc.sfAlertName, "ba_dum", "tss", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "This should fire after an interval"),
				State:       "firing",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		})
		desc += "/firing/firing"
	}

	// TODO: temporary for development.
	fmt.Println(desc, alerts)

	return expAlerts
}

func (tc *zeroAndSmallFor) expRuleGroups(ts int64) (expRgs []v1.RuleGroup) {
	relTs := ts - tc.zeroTime
	canBeInactive, zfFiring, sfPending, sfFiring := tc.allPossibleStates(relTs)
	activeAt := timestamp.Time(tc.zeroTime + int64(8*tc.rwInterval/time.Millisecond))

	getRg := func(s1, s2 string, a1, a2 []*v1.Alert) v1.RuleGroup {
		return v1.RuleGroup{
			Name:     tc.groupName,
			Interval: float64(tc.groupInterval / time.Second),
			Rules: []v1.Rule{
				v1.AlertingRule{
					State:       s1,
					Name:        tc.zfAlertName,
					Query:       tc.zfQuery,
					Labels:      labels.FromStrings("foo", "bar", "rulegroup", tc.groupName),
					Annotations: labels.FromStrings("description", "This should immediately fire"),
					Alerts:      a1,
					Health:      "ok",
					Type:        "alerting",
				},
				v1.AlertingRule{
					State:       s2,
					Name:        tc.sfAlertName,
					Query:       tc.sfQuery,
					Duration:    float64(time.Duration(tc.forDuration) / time.Second),
					Labels:      labels.FromStrings("ba_dum", "tss", "rulegroup", tc.groupName),
					Annotations: labels.FromStrings("description", "This should fire after an interval"),
					Alerts:      a2,
					Health:      "ok",
					Type:        "alerting",
				},
			},
		}
	}

	if canBeInactive {
		expRgs = append(expRgs, getRg("inactive", "inactive", nil, nil))
	}
	if zfFiring && sfPending {
		expRgs = append(expRgs, getRg("firing", "pending",
			[]*v1.Alert{
				{
					Labels:      labels.FromStrings("alertname", tc.zfAlertName, "foo", "bar", "rulegroup", tc.groupName),
					Annotations: labels.FromStrings("description", "This should immediately fire"),
					State:       "firing",
					Value:       "11",
					ActiveAt:    &activeAt,
				},
			},
			[]*v1.Alert{
				{
					Labels:      labels.FromStrings("alertname", tc.sfAlertName, "ba_dum", "tss", "rulegroup", tc.groupName),
					Annotations: labels.FromStrings("description", "This should fire after an interval"),
					State:       "pending",
					Value:       "11",
					ActiveAt:    &activeAt,
				},
			},
		))
	}
	if zfFiring && sfFiring {
		expRgs = append(expRgs, getRg("firing", "firing",
			[]*v1.Alert{
				{
					Labels:      labels.FromStrings("alertname", tc.zfAlertName, "foo", "bar", "rulegroup", tc.groupName),
					Annotations: labels.FromStrings("description", "This should immediately fire"),
					State:       "firing",
					Value:       "11",
					ActiveAt:    &activeAt,
				},
			},
			[]*v1.Alert{
				{
					Labels:      labels.FromStrings("alertname", tc.sfAlertName, "ba_dum", "tss", "rulegroup", tc.groupName),
					Annotations: labels.FromStrings("description", "This should fire after an interval"),
					State:       "firing",
					Value:       "11",
					ActiveAt:    &activeAt,
				},
			},
		))
	}

	return expRgs
}

func (tc *zeroAndSmallFor) expMetrics(ts int64) (expSamples [][]promql.Sample) {
	relTs := ts - tc.zeroTime
	canBeInactive, zfFiring, sfPending, sfFiring := tc.allPossibleStates(relTs)

	if canBeInactive {
		expSamples = append(expSamples, nil)
	}
	if zfFiring && sfPending {
		expSamples = append(expSamples, []promql.Sample{
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "firing", "alertname", tc.zfAlertName, "foo", "bar", "rulegroup", tc.groupName),
			},
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "pending", "alertname", tc.sfAlertName, "ba_dum", "tss", "rulegroup", tc.groupName),
			},
		})
	}
	if zfFiring && sfFiring {
		expSamples = append(expSamples, []promql.Sample{
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "firing", "alertname", tc.zfAlertName, "foo", "bar", "rulegroup", tc.groupName),
			},
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "firing", "alertname", tc.sfAlertName, "ba_dum", "tss", "rulegroup", tc.groupName),
			},
		})
	}

	return expSamples
}

// ts is relative time w.r.t. zeroTime.
func (tc *zeroAndSmallFor) allPossibleStates(ts int64) (canBeInactive, zfFiring, sfPending, sfFiring bool) {
	between := betweenFunc(ts)

	rwItvlSecFloat, grpItvlSecFloat := float64(tc.rwInterval/time.Second), float64(tc.groupInterval/time.Second)
	_8th := 8 * rwItvlSecFloat   // Goes into pending.
	_21st := 21 * rwItvlSecFloat // Becomes inactive again.
	canBeInactive = between(0, _8th+grpItvlSecFloat) || between(_21st, 240*rwItvlSecFloat)

	zfFiring = between(_8th-1, _21st+grpItvlSecFloat)
	sfPending = between(_8th-1, _8th+(2*grpItvlSecFloat))
	sfFiring = between(_8th+grpItvlSecFloat, _21st+grpItvlSecFloat)
	return
}

func (tc *zeroAndSmallFor) ExpectedAlerts() []ExpectedAlert {
	_8th := 8 * int64(tc.rwInterval/time.Millisecond)               // Zero for firing.
	_8th_plus_gi := _8th + int64(tc.groupInterval/time.Millisecond) // Small for firing.
	_21st := 21 * int64(tc.rwInterval/time.Millisecond)             // Resolved.
	_21stPlus15m := _21st + int64(15*time.Minute/time.Millisecond)

	var exp []ExpectedAlert
	endsAtDelta := 4 * ResendDelay
	if endsAtDelta < 4*tc.groupInterval {
		endsAtDelta = 4 * tc.groupInterval
	}

	resendDelayMs := int64(ResendDelay / time.Millisecond)
	for ts := _8th; ts < _21st; ts += resendDelayMs {
		exp = append(exp, ExpectedAlert{
			OrderingID:    int(ts),
			TimeTolerance: tc.groupInterval,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      false,
			Resend:        ts != _8th,
			NextState:     timestamp.Time(tc.zeroTime + _21st),
			ResolvedTime:  timestamp.Time(tc.zeroTime + _21st),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.zfAlertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "This should immediately fire"),
				StartsAt:    timestamp.Time(tc.zeroTime + _8th),
			},
		})
	}
	for ts := _21st; ts < _21stPlus15m; ts += resendDelayMs {
		tolerance := tc.groupInterval
		if ts == _21st {
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
			Resend:        ts != _21st,
			ResolvedTime:  timestamp.Time(tc.zeroTime + _21st),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.zfAlertName, "foo", "bar", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "This should immediately fire"),
				StartsAt:    timestamp.Time(tc.zeroTime + _8th),
			},
		})
	}

	for ts := _8th_plus_gi; ts < _21st; ts += resendDelayMs {
		exp = append(exp, ExpectedAlert{
			OrderingID:    int(ts),
			TimeTolerance: tc.groupInterval,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      false,
			Resend:        ts != _8th_plus_gi,
			NextState:     timestamp.Time(tc.zeroTime + _21st),
			ResolvedTime:  timestamp.Time(tc.zeroTime + _21st),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.sfAlertName, "ba_dum", "tss", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "This should fire after an interval"),
				StartsAt:    timestamp.Time(tc.zeroTime + _8th_plus_gi),
			},
		})
	}
	for ts := _21st; ts < _21stPlus15m; ts += resendDelayMs {
		tolerance := tc.groupInterval
		if ts == _21st {
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
			Resend:        ts != _21st,
			ResolvedTime:  timestamp.Time(tc.zeroTime + _21st),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.sfAlertName, "ba_dum", "tss", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", "This should fire after an interval"),
				StartsAt:    timestamp.Time(tc.zeroTime + _8th_plus_gi),
			},
		})
	}

	return exp
}

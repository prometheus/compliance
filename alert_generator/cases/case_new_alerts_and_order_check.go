package cases

import (
	"fmt"
	"sort"
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

// NewAlerts_OrderCheck tests the following cases:
// * Rule that produces new alerts that go from pending->firing->inactive while already having active alerts.
// * A rule group having rules which are dependent on the ALERTS series from the rules above it in the same group.
// * Expansion of template in annotations only use the labels from the query result as source data even if those labels get overridden by the rules. They do not use the rules' additional labels.
func NewAlerts_OrderCheck() TestCase {
	groupName := "NewAlerts_OrderCheck"
	r1AlertName := groupName + "_Rule1"
	r2AlertName := groupName + "_Rule2"
	r1Labels := metricLabels(groupName, r1AlertName)

	tc := &newAlertsAndOrderCheck{
		groupName:      groupName,
		r1AlertName:    r1AlertName,
		r1Query:        fmt.Sprintf("%s > 10", r1Labels.String()),
		r1MetricLabels: r1Labels,
		r2AlertName:    r2AlertName,
		r2Query: fmt.Sprintf(
			`(ALERTS{alertstate="firing", alertname="%s", foo="bar", rulegroup="%s", variant="one"} + ignoring(variant) ALERTS{alertstate="firing", alertname="%s", foo="bar", rulegroup="%s", variant="two"}) == 2`,
			r1AlertName, groupName, r1AlertName, groupName),
		rwInterval:    15 * time.Second,
		groupInterval: 30 * time.Second,
	}
	tc.forDuration = model.Duration(12 * tc.rwInterval) // 3m with 15s rw interval.
	return tc
}

type newAlertsAndOrderCheck struct {
	groupName                 string
	r1AlertName, r2AlertName  string
	r1Query, r2Query          string
	r1MetricLabels            labels.Labels
	rwInterval, groupInterval time.Duration
	forDuration               model.Duration // For the "new alerts".
	totalSamples              int

	zeroTime int64
}

func (tc *newAlertsAndOrderCheck) Describe() (title string, description string) {
	return tc.groupName,
		"(1) Rule that produces new alerts that go from pending->firing->inactive while already having active alerts. " +
			"(2) A rule group having rules which are dependent on the ALERTS series from the rules above it in the same group. " +
			"(3) Expansion of template in annotations only use the labels from the query result as source data even if those labels get overridden by the rules. They do not use the rules' additional labels."
}

func (tc *newAlertsAndOrderCheck) RuleGroup() (rulefmt.RuleGroup, error) {
	var r1Alert, r2Alert yaml.Node
	if err := r1Alert.Encode(tc.r1AlertName); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	if err := r2Alert.Encode(tc.r2AlertName); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	var r1Expr, r2Expr yaml.Node
	if err := r1Expr.Encode(tc.r1Query); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	if err := r2Expr.Encode(tc.r2Query); err != nil {
		return rulefmt.RuleGroup{}, err
	}
	return rulefmt.RuleGroup{
		Name:     tc.groupName,
		Interval: model.Duration(tc.groupInterval),
		Rules: []rulefmt.RuleNode{
			{ // New alerts.
				Alert:       r1Alert,
				Expr:        r1Expr,
				For:         tc.forDuration,
				Labels:      map[string]string{"foo": "bar", "rulegroup": tc.groupName},
				Annotations: map[string]string{"description": "This should produce more alerts later"},
			},
			{ // ALERTS{} based.
				Alert:  r2Alert,
				Expr:   r2Expr,
				Labels: map[string]string{"foo": "baz", "ba_dum": "tss", "rulegroup": tc.groupName},
				// The expression also has alertname. So this template variable should result in r1AlertName.
				Annotations: map[string]string{"description": "Based on ALERTS. Old alertname was {{$labels.alertname}}. foo was {{.Labels.foo}}."},
			},
		},
	}, nil
}

func (tc *newAlertsAndOrderCheck) SamplesToRemoteWrite() []prompb.TimeSeries {
	series1 := append(tc.r1MetricLabels.Copy(), labels.Label{Name: "variant", Value: "one"})
	sort.Sort(series1)
	samples1 := sampleSlice(tc.rwInterval,
		// All comment times is assuming 15s interval.
		"1", "0x7", // 2m of inactive.
		"11", // Pending @2m.
		// 16m more of this. Goes into firing here.
		"0x64",
		// Resolved now.
		"9", "0x20",
	)

	series2 := append(tc.r1MetricLabels.Copy(), labels.Label{Name: "variant", Value: "two"})
	sort.Sort(series2)
	samples2 := sampleSlice(tc.rwInterval,
		// All comment times is assuming 15s interval.
		"1", "0x31", // 8m of inactive.
		"11", // Pending @8m.
		// 8m more of this. Goes into firing here.
		"0x32",
		// Resolved now.
		"9", "0x20",
	)

	tc.totalSamples = len(samples1)
	if len(samples2) > tc.totalSamples {
		tc.totalSamples = len(samples2)
	}

	return []prompb.TimeSeries{
		{
			Labels:  toProtoLabels(series1),
			Samples: samples1,
		},
		{
			Labels:  toProtoLabels(series2),
			Samples: samples2,
		},
	}
}

func (tc *newAlertsAndOrderCheck) Init(zt int64) {
	tc.zeroTime = zt
}

func (tc *newAlertsAndOrderCheck) TestUntil() int64 {
	return timestamp.FromTime(timestamp.Time(tc.zeroTime).Add(time.Duration(tc.totalSamples) * tc.rwInterval))
}

func (tc *newAlertsAndOrderCheck) CheckAlerts(ts int64, alerts []v1.Alert) error {
	expAlerts := tc.expAlerts(ts, alerts)
	return checkExpectedAlerts(expAlerts, alerts, tc.groupInterval)
}

func (tc *newAlertsAndOrderCheck) CheckRuleGroup(ts int64, rg *v1.RuleGroup) error {
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

func (tc *newAlertsAndOrderCheck) CheckMetrics(ts int64, samples []promql.Sample) error {
	expSamples := tc.expMetrics(ts)
	return checkExpectedSamples(expSamples, samples)
}

func (tc *newAlertsAndOrderCheck) expAlerts(ts int64, alerts []v1.Alert) (expAlerts [][]v1.Alert) {
	relTs := ts - tc.zeroTime
	r11Inactive, r11Pending, r11Firing,
		r12Inactive, r12Pending, r12Firing := tc.allPossibleStates(relTs)

	activeAt := timestamp.Time(tc.zeroTime + int64(8*tc.rwInterval/time.Millisecond))
	activeAt2 := timestamp.Time(tc.zeroTime + int64(32*tc.rwInterval/time.Millisecond))
	activeAt3 := timestamp.Time(tc.zeroTime + int64(44*tc.rwInterval/time.Millisecond))

	desc := "-----"

	if r11Inactive && r12Inactive {
		expAlerts = append(expAlerts, []v1.Alert{})
		desc += "/inactive"
	}

	if r11Pending && r12Inactive {
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				State:       "pending",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		})
		desc += "/pending"
	}

	if r11Firing && r12Inactive {
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				State:       "firing",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		})
		desc += "/firing"
	}

	if r11Firing && r12Pending {
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				State:       "firing",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
			{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "two"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				State:       "pending",
				Value:       "11",
				ActiveAt:    &activeAt2,
			},
		})
		desc += "/firing-pending"
	}

	if r11Firing && r12Firing {
		expAlerts = append(expAlerts, []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				State:       "firing",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
			{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "two"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				State:       "firing",
				Value:       "11",
				ActiveAt:    &activeAt2,
			},
			{
				Labels:      labels.FromStrings("alertname", tc.r2AlertName, "alertstate", "firing", "foo", "baz", "ba_dum", "tss", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", fmt.Sprintf("Based on ALERTS. Old alertname was %s. foo was bar.", tc.r1AlertName)),
				State:       "firing",
				Value:       "2",
				ActiveAt:    &activeAt3,
			},
		})
		desc += "/firing-firing-firing"
	}

	// TODO: temporary for development.
	devPrint(desc, alerts)

	return expAlerts
}

func (tc *newAlertsAndOrderCheck) expRuleGroups(ts int64) (expRgs []v1.RuleGroup) {
	relTs := ts - tc.zeroTime
	r11Inactive, r11Pending, r11Firing,
		r12Inactive, r12Pending, r12Firing := tc.allPossibleStates(relTs)

	activeAt := timestamp.Time(tc.zeroTime + int64(8*tc.rwInterval/time.Millisecond))
	activeAt2 := timestamp.Time(tc.zeroTime + int64(32*tc.rwInterval/time.Millisecond))
	activeAt3 := timestamp.Time(tc.zeroTime + int64(44*tc.rwInterval/time.Millisecond))

	getRg := func(s1, s2 string, a1, a2 []*v1.Alert) v1.RuleGroup {
		return v1.RuleGroup{
			Name:     tc.groupName,
			Interval: float64(tc.groupInterval / time.Second),
			Rules: []v1.Rule{
				v1.AlertingRule{
					State:       s1,
					Name:        tc.r1AlertName,
					Query:       tc.r1Query,
					Duration:    float64(time.Duration(tc.forDuration) / time.Second),
					Labels:      labels.FromStrings("foo", "bar", "rulegroup", tc.groupName),
					Annotations: labels.FromStrings("description", "This should produce more alerts later"),
					Alerts:      a1,
					Health:      "ok",
					Type:        "alerting",
				},
				v1.AlertingRule{
					State:       s2,
					Name:        tc.r2AlertName,
					Query:       tc.r2Query,
					Labels:      labels.FromStrings("foo", "baz", "ba_dum", "tss", "rulegroup", tc.groupName),
					Annotations: labels.FromStrings("description", "Based on ALERTS. Old alertname was {{$labels.alertname}}. foo was {{.Labels.foo}}."),
					Alerts:      a2,
					Health:      "ok",
					Type:        "alerting",
				},
			},
		}
	}

	if r11Inactive && r12Inactive {
		expRgs = append(expRgs, getRg("inactive", "inactive", nil, nil))
	}

	if r11Pending && r12Inactive {
		expRgs = append(expRgs, getRg("pending", "inactive", []*v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				State:       "pending",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		}, nil))
	}

	if r11Firing && r12Inactive {
		// Only r11 firing.
		expRgs = append(expRgs, getRg("firing", "inactive", []*v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				State:       "firing",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
		}, nil))
	}

	if r11Firing && r12Pending {
		expRgs = append(expRgs, getRg("firing", "inactive", []*v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				State:       "firing",
				Value:       "11",
				ActiveAt:    &activeAt,
			},
			{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "two"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				State:       "pending",
				Value:       "11",
				ActiveAt:    &activeAt2,
			},
		}, nil))
	}

	if r11Firing && r12Firing {
		expRgs = append(expRgs, getRg("firing", "firing",
			[]*v1.Alert{
				{
					Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
					Annotations: labels.FromStrings("description", "This should produce more alerts later"),
					State:       "firing",
					Value:       "11",
					ActiveAt:    &activeAt,
				},
				{
					Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "two"),
					Annotations: labels.FromStrings("description", "This should produce more alerts later"),
					State:       "firing",
					Value:       "11",
					ActiveAt:    &activeAt2,
				},
			},
			[]*v1.Alert{
				{
					Labels:      labels.FromStrings("alertname", tc.r2AlertName, "alertstate", "firing", "foo", "baz", "ba_dum", "tss", "rulegroup", tc.groupName),
					Annotations: labels.FromStrings("description", fmt.Sprintf("Based on ALERTS. Old alertname was %s. foo was bar.", tc.r1AlertName)),
					State:       "firing",
					Value:       "2",
					ActiveAt:    &activeAt3,
				},
			},
		))
	}

	return expRgs
}

func (tc *newAlertsAndOrderCheck) expMetrics(ts int64) (expSamples [][]promql.Sample) {
	relTs := ts - tc.zeroTime
	r11Inactive, r11Pending, r11Firing,
		r12Inactive, r12Pending, r12Firing := tc.allPossibleStates(relTs)

	if r11Inactive && r12Inactive {
		expSamples = append(expSamples, nil)
	}

	if r11Pending && r12Inactive {
		expSamples = append(expSamples, []promql.Sample{
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "pending", "alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
			},
		})
	}

	if r11Firing && r12Inactive {
		expSamples = append(expSamples, []promql.Sample{
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "firing", "alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
			},
		})
	}

	if r11Firing && r12Pending {
		expSamples = append(expSamples, []promql.Sample{
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "firing", "alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
			},
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "pending", "alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "two"),
			},
		})
	}

	if r11Firing && r12Firing {
		expSamples = append(expSamples, []promql.Sample{
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "firing", "alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
			},
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "firing", "alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "two"),
			},
			{
				Point:  promql.Point{T: ts / 1000, V: 1},
				Metric: labels.FromStrings("__name__", "ALERTS", "alertstate", "firing", "alertname", tc.r2AlertName, "foo", "baz", "ba_dum", "tss", "rulegroup", tc.groupName),
			},
		})
	}

	return expSamples
}

// ts is relative time w.r.t. zeroTime.
func (tc *newAlertsAndOrderCheck) allPossibleStates(ts int64) (
	r11Inactive, r11Pending, r11Firing bool,
	r12Inactive, r12Pending, r12Firing bool,
) {
	between := betweenFunc(ts)

	rwItvlSecFloat, grpItvlSecFloat := float64(tc.rwInterval/time.Second), float64(tc.groupInterval/time.Second)

	// r11 (variant one).
	_8th := 8 * rwItvlSecFloat   // Goes into pending.
	_20th := 20 * rwItvlSecFloat // Firing.
	_73rd := 73 * rwItvlSecFloat // Resolved.

	r11Inactive = between(0, _8th+grpItvlSecFloat) ||
		between(_73rd, 240*rwItvlSecFloat)
	r11Pending = between(_8th-1, _20th+grpItvlSecFloat)
	r11Firing = between(_20th-1, _73rd+grpItvlSecFloat)

	// r12 (variant two).
	_32nd := 32 * rwItvlSecFloat // Goes into pending.
	_44th := 44 * rwItvlSecFloat // Firing.
	_65th := 65 * rwItvlSecFloat // Resolved.

	r12Inactive = between(0, _32nd+grpItvlSecFloat) ||
		between(_65th, 240*rwItvlSecFloat)
	r12Pending = between(_32nd-1, _44th+grpItvlSecFloat)
	r12Firing = between(_44th-1, _73rd+grpItvlSecFloat)

	// TODO: r2Firing is maybe at r12Firing+grpItvlSecFloat.

	return
}

func (tc *newAlertsAndOrderCheck) ExpectedAlerts() []ExpectedAlert {
	var exp []ExpectedAlert
	endsAtDelta := 4 * ResendDelay
	if endsAtDelta < 4*tc.groupInterval {
		endsAtDelta = 4 * tc.groupInterval
	}

	resendDelayMs := int64(ResendDelay / time.Millisecond)

	orderingID := 0
	addAlert := func(ea ExpectedAlert) {
		orderingID++
		ea.OrderingID = orderingID
		exp = append(exp, ea)
	}

	// r11.
	_20th := 20 * int64(tc.rwInterval/time.Millisecond) // Firing.
	_73rd := 73 * int64(tc.rwInterval/time.Millisecond) // Resolved.
	_73rdPlus15m := _73rd + int64(15*time.Minute/time.Millisecond)
	for ts := _20th; ts < _73rd; ts += resendDelayMs {
		addAlert(ExpectedAlert{
			TimeTolerance: tc.groupInterval,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      false,
			Resend:        ts != _20th,
			NextState:     timestamp.Time(tc.zeroTime + _73rd),
			ResolvedTime:  timestamp.Time(tc.zeroTime + _73rd),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				StartsAt:    timestamp.Time(tc.zeroTime + _20th),
			},
		})
	}
	for ts := _73rd; ts < _73rdPlus15m; ts += resendDelayMs {
		tolerance := tc.groupInterval
		if ts == _73rd {
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
			Resend:        ts != _73rd,
			ResolvedTime:  timestamp.Time(tc.zeroTime + _73rd),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "one"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				StartsAt:    timestamp.Time(tc.zeroTime + _20th),
			},
		})
	}

	// r12.
	_44th := 44 * int64(tc.rwInterval/time.Millisecond) // Firing.
	_65th := 65 * int64(tc.rwInterval/time.Millisecond) // Resolved.
	_65thPlus15m := _65th + int64(15*time.Minute/time.Millisecond)
	//_8th_plus_gi := _8th + int64(tc.groupInterval/time.Millisecond) // Small for firing.
	for ts := _44th; ts < _65th; ts += resendDelayMs {
		addAlert(ExpectedAlert{
			TimeTolerance: tc.groupInterval,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      false,
			Resend:        ts != _44th,
			NextState:     timestamp.Time(tc.zeroTime + _65th),
			ResolvedTime:  timestamp.Time(tc.zeroTime + _65th),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "two"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				StartsAt:    timestamp.Time(tc.zeroTime + _44th),
			},
		})

		// r2.
		addAlert(ExpectedAlert{
			TimeTolerance: tc.groupInterval,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      false,
			Resend:        ts != _44th,
			NextState:     timestamp.Time(tc.zeroTime + _65th),
			ResolvedTime:  timestamp.Time(tc.zeroTime + _65th),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.r2AlertName, "alertstate", "firing", "foo", "baz", "ba_dum", "tss", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", fmt.Sprintf("Based on ALERTS. Old alertname was %s. foo was bar.", tc.r1AlertName)),
				StartsAt:    timestamp.Time(tc.zeroTime + _44th),
			},
		})
	}
	for ts := _65th; ts < _65thPlus15m; ts += resendDelayMs {
		tolerance := tc.groupInterval
		if ts == _65th {
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
			Resend:        ts != _65th,
			ResolvedTime:  timestamp.Time(tc.zeroTime + _65th),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.r1AlertName, "foo", "bar", "rulegroup", tc.groupName, "variant", "two"),
				Annotations: labels.FromStrings("description", "This should produce more alerts later"),
				StartsAt:    timestamp.Time(tc.zeroTime + _44th),
			},
		})

		addAlert(ExpectedAlert{
			TimeTolerance: tolerance,
			Ts:            timestamp.Time(tc.zeroTime + ts),
			Resolved:      true,
			Resend:        ts != _65th,
			ResolvedTime:  timestamp.Time(tc.zeroTime + _65th),
			EndsAtDelta:   endsAtDelta,
			Alert: &notifier.Alert{
				Labels:      labels.FromStrings("alertname", tc.r2AlertName, "alertstate", "firing", "foo", "baz", "ba_dum", "tss", "rulegroup", tc.groupName),
				Annotations: labels.FromStrings("description", fmt.Sprintf("Based on ALERTS. Old alertname was %s. foo was bar.", tc.r1AlertName)),
				StartsAt:    timestamp.Time(tc.zeroTime + _44th),
			},
		})
	}

	return exp
}

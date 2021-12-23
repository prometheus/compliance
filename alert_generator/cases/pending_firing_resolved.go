package cases

import (
	"fmt"
	"github.com/prometheus/prometheus/pkg/labels"
	"time"

	"github.com/prometheus/common/model"
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
	lbls := baseLabels(groupName, alertName)
	zeroTime := int64(0)

	allPossibleStates := func(ts int64) (inactive, maybePending, pending, maybeFiring, firing, maybeResolved, resolved bool) {
		between := betweenFunc(ts)

		inactive = between(0, 120-1)
		maybePending = between(120-1, 120+30)
		pending = between(120+30, 300-1)
		maybeFiring = between(300-1, 300+30)
		firing = between(300+30, (8*60)+15-1)
		maybeResolved = between((8*60)+15-1, (8*60)+15+30)
		resolved = between((8*60)+15+30, 3600)
		return
	}

	expAlertsMetricsRules := func(ts int64, alerts []v1.Alert) (expAlerts [][]v1.Alert, expActiveAtRanges [][2]time.Time, expSamples []promql.Sample) {
		relTs := ts - zeroTime
		inactive, maybePending, pending, maybeFiring, firing, maybeResolved, resolved := allPossibleStates(relTs)

		pendingAlerts := []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", "PendingAndFiringAndResolved_SimpleAlert", "foo", "bar", "rulegroup", "PendingAndFiringAndResolved"),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
				State:       "pending",
				Value:       "1.1",
			},
		}
		firingAlerts := []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", "PendingAndFiringAndResolved_SimpleAlert", "foo", "bar", "rulegroup", "PendingAndFiringAndResolved"),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
				State:       "firing",
				Value:       "1.1",
			},
		}
		resolvedAlerts := []v1.Alert{
			{
				Labels:      labels.FromStrings("alertname", "PendingAndFiringAndResolved_SimpleAlert", "foo", "bar", "rulegroup", "PendingAndFiringAndResolved"),
				Annotations: labels.FromStrings("description", "SimpleAlert is firing"),
				State:       "inactive",
				Value:       "9",
			},
		}
		fmt.Printf("\n")
		switch {
		case inactive:
			fmt.Println("inactive", alerts)
			expAlerts = append(expAlerts, []v1.Alert{})
		case maybePending:
			fmt.Println("maybePending", alerts)
			expAlerts = append(expAlerts, []v1.Alert{}, pendingAlerts)
		case pending:
			fmt.Println("pending", alerts)
			expAlerts = append(expAlerts, pendingAlerts)
		case maybeFiring:
			fmt.Println("maybeFiring", alerts)
			expAlerts = append(expAlerts, pendingAlerts, firingAlerts)
		case firing:
			fmt.Println("firing", alerts)
			expAlerts = append(expAlerts, firingAlerts)
		case maybeResolved:
			fmt.Println("maybeResolved", alerts)
			expAlerts = append(expAlerts, firingAlerts, resolvedAlerts)
		case resolved:
			fmt.Println("resolved", alerts)
			// TODO: there should be no alerts found after a point.
			expAlerts = append(expAlerts, resolvedAlerts)
		default:
		}

		expActiveAtRanges = convertRelativeToAbsoluteTimes(zeroTime, [][2]int64{
			{120, 120 + 30},
		})
		return expAlerts, expActiveAtRanges, nil
	}

	return &testCase{
		describe: func() (title string, description string) {
			return groupName,
				"An alert goes from pending to firing to resolved state and stays in resolved state"
		},
		ruleGroup: func() (rulefmt.RuleGroup, error) {
			var alert yaml.Node
			var expr yaml.Node
			if err := alert.Encode(alertName); err != nil {
				return rulefmt.RuleGroup{}, err
			}
			if err := expr.Encode(fmt.Sprintf("%s > 10", lbls.String())); err != nil {
				return rulefmt.RuleGroup{}, err
			}
			return rulefmt.RuleGroup{
				Name:     groupName,
				Interval: model.Duration(30 * time.Second),
				Rules: []rulefmt.RuleNode{
					{
						Alert:       alert,
						Expr:        expr,
						For:         model.Duration(3 * time.Minute),
						Labels:      map[string]string{"foo": "bar", "rulegroup": groupName},
						Annotations: map[string]string{"description": "SimpleAlert is firing"},
					},
				},
			}, nil
		},
		samplesToRemoteWrite: func() []prompb.TimeSeries {
			// TODO: consider using the `load 15s metric 1+1x5` etc notation used in Prometheus tests.
			return []prompb.TimeSeries{
				{
					Labels: toProtoLabels(lbls),
					Samples: sampleSlice(15*time.Second,
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
		},
		init: func(zt int64) {
			zeroTime = zt
		},
		testUntil: func() int64 {
			return timestamp.FromTime(timestamp.Time(zeroTime).Add(26 * time.Minute))
		},
		checkAlerts: func(ts int64, alerts []v1.Alert) error {
			expAlerts, expRanges, _ := expAlertsMetricsRules(ts, alerts)
			return checkAllPossibleExpectedAlerts(expAlerts, expRanges, alerts)
		},
		checkMetrics: func(ts int64, metrics []promql.Sample) error {
			//_, _, expSamples := expAlertsMetricsRules(ts, nil)
			return nil
		},
	}
}

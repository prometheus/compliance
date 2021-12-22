package cases

import (
	"fmt"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/rulefmt"
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

	return &testCase{
		describe: func() (title string, description string) {
			return groupName, "An alert goes from pending to firing to resolved state and stays in resolved state"
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
		checkAlerts: func(ts int64, alerts []v1.Alert) (ok bool, expected []v1.Alert) {
			//relTs := ts - zeroTime
			//inactive, maybePending, pending, maybeFiring, firing, maybeResolved, resolved := allPossibleStates(relTs)

			//fmt.Printf("\n")
			//switch {
			//case inactive:
			//	fmt.Println("inactive", ts, alerts)
			//case maybePending:
			//	fmt.Println("maybePending", ts, alerts)
			//case pending:
			//	fmt.Println("pending", ts, alerts)
			//case maybeFiring:
			//	fmt.Println("maybeFiring", ts, alerts)
			//case firing:
			//	fmt.Println("firing", ts, alerts)
			//case maybeResolved:
			//	fmt.Println("maybeResolved", ts, alerts)
			//case resolved:
			//	fmt.Println("resolved", ts, alerts)
			//default:
			//	fmt.Println("default ", ts, alerts)
			//}

			//pending 1640105102651 [{{alertname="PendingAndFiringAndResolved_SimpleAlert", foo="bar", rulegroup="PendingAndFiringAndResolved"} {description="SimpleAlert is firing"} pending 2021-12-21 16:42:44.682709561 +0000 UTC 1.1e+01}]
			//level=debug ts=2021-12-21T16:45:17.612Z caller=remote_write.go:131 msg="Remote writing" timestamp=1640105117597 total_series=1
			//
			//maybeFiring 1640105117654 [{{alertname="PendingAndFiringAndResolved_SimpleAlert", foo="bar", rulegroup="PendingAndFiringAndResolved"} {description="SimpleAlert is firing"} pending 2021-12-21 16:42:44.682709561 +0000 UTC 1.1e+01}]
			//level=debug ts=2021-12-21T16:45:32.611Z caller=remote_write.go:131 msg="Remote writing" timestamp=1640105132597 total_series=1
			//
			//maybeFiring 1640105132661 [{{alertname="PendingAndFiringAndResolved_SimpleAlert", foo="bar", rulegroup="PendingAndFiringAndResolved"} {description="SimpleAlert is firing"} pending 2021-12-21 16:42:44.682709561 +0000 UTC 1.1e+01}]
			//level=debug ts=2021-12-21T16:45:47.611Z caller=remote_write.go:131 msg="Remote writing" timestamp=1640105147597 total_series=1
			//
			//firing 1640105147663 [{{alertname="PendingAndFiringAndResolved_SimpleAlert", foo="bar", rulegroup="PendingAndFiringAndResolved"} {description="SimpleAlert is firing"} firing 2021-12-21 16:42:44.682709561 +0000 UTC 1.1e+01}]
			//level=debug ts=2021-12-21T16:46:02.611Z caller=remote_write.go:131 msg="Remote writing" timestamp=1640105162597 total_series=1
			//
			//firing 1640105162665 [{{alertname="PendingAndFiringAndResolved_SimpleAlert", foo="bar", rulegroup="PendingAndFiringAndResolved"} {description="SimpleAlert is firing"} firing 2021-12-21 16:42:44.682709561 +0000 UTC 1.1e+01}]
			//level=debug ts=2021-12-21T16:46:17.602Z caller=remote_write.go:131 msg="Remote writing" timestamp=1640105177597 total_series=1
			//
			//firing 1640105177667 [{{alertname="PendingAndFiringAndResolved_SimpleAlert", foo="bar", rulegroup="PendingAndFiringAndResolved"} {description="SimpleAlert is firing"} firing 2021-12-21 16:42:44.682709561 +0000 UTC 1.1e+01}]
			//level=debug ts=2021-12-21T16:46:32.611Z caller=remote_write.go:131 msg="Remote writing" timestamp=1640105192597 total_series=1

			return true, expected
		},
		checkMetrics: func(ts int64, metrics []promql.Sample) (ok bool, expected string) {
			relTs := ts - zeroTime
			inactive, maybePending, pending, maybeFiring, firing, maybeResolved, resolved := allPossibleStates(relTs)

			fmt.Printf("\n")
			switch {
			case inactive:
				fmt.Println("inactive", ts, metrics)
			case maybePending:
				fmt.Println("maybePending", ts, metrics)
			case pending:
				fmt.Println("pending", ts, metrics)
			case maybeFiring:
				fmt.Println("maybeFiring", ts, metrics)
			case firing:
				fmt.Println("firing", ts, metrics)
			case maybeResolved:
				fmt.Println("maybeResolved", ts, metrics)
			case resolved:
				fmt.Println("resolved", ts, metrics)
			default:
				fmt.Println("default ", ts, metrics)
			}

			return true, expected
		},
	}
}

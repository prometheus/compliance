package cases

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/web/api/v1"
)

func PendingAndFiringAndResolved() TestCase {
	groupName := "PendingAndFiringAndResolved"
	alertName := groupName + "_SimpleAlert"
	lbls := baseLabels(groupName, alertName)
	zeroTime := int64(0)

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
						Labels:      map[string]string{"foo": "bar"},
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
						9, 9, 9, 11, // 1m block. Gets into pending at value 11.
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
			// TODO
			fmt.Println(alerts)
			return true, expected
		},
		checkMetrics: func(ts int64, metrics string) (ok bool, expected string) {
			// TODO
			fmt.Println(metrics)
			return true, expected
		},
	}
}

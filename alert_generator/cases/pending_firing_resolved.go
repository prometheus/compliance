package cases

import (
	"fmt"
	"gopkg.in/yaml.v3"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/web/api/v1"
)

func PendingAndFiringAndResolved() TestCase {
	groupName := "PendingAndFiringAndResolved"
	alertName := "SimpleAlert"
	lbls := baseLabels(groupName, alertName)
	zeroTime := int64(0)

	return &testCase{
		describe: func() (title string, description string) {
			return "PendingAndFiringAndResolved", "An alert goes from pending to firing to resolved state and stays in resolved state"
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
			// 15s scrape interval.
			series := prompb.TimeSeries{
				Labels: toProtoLabels(lbls),
				Samples: []prompb.Sample{
					{Value: 0},
				},
			}
			// Add the timestamps.
			ts := time.Unix(0, 0)
			for i := range series.Samples {
				series.Samples[i].Timestamp = timestamp.FromTime(ts)
				ts = ts.Add(15 * time.Second)
			}
			return []prompb.TimeSeries{series}
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

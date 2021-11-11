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
		ruleGroup: func() rulefmt.RuleGroup {
			return rulefmt.RuleGroup{
				Name:     groupName,
				Interval: model.Duration(30 * time.Second),
				Rules: []rulefmt.RuleNode{
					{
						Alert:       yaml.Node{Value: alertName},
						Expr:        yaml.Node{Value: fmt.Sprintf("%s > 10", lbls.String())},
						For:         model.Duration(3 * time.Minute),
						Labels:      map[string]string{"foo": "bar"},
						Annotations: map[string]string{"description": "SimpleAlert is firing"},
					},
				},
			}
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

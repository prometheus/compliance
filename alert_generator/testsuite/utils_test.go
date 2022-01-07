package testsuite

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
	"github.com/prometheus/prometheus/promql"
)

func TestParseAndGroupMetrics(t *testing.T) {
	cases := []struct {
		response   string
		expMetrics map[string][]promql.Sample
	}{
		{
			response: `
{
	"status": "success",
	"data": {
		"resultType": "vector",
		"result": [
			{
				"metric": {"__name__":"ALERTS","alertname":"PendingAndFiringAndResolved_SimpleAlert","alertstate":"pending","foo":"bar","rulegroup":"PendingAndFiringAndResolved"},
				"value": [1640145082,"1"]
			},
			{
				"metric": {"__name__":"ALERTS","alertname":"PendingAndFiringAndResolved_SimpleAlert2","alertstate":"firing","foo":"bar","rulegroup":"PendingAndFiringAndResolved"},
				"value": [1640145085,"1"]
			},
			{
				"metric": {"__name__":"ALERTS","alertname":"AnotherGroup_SimpleAlert","alertstate":"firing","foo":"bar","rulegroup":"AnotherGroup"},
				"value": [1640145083,"1"]
			}
		]
	}
}`,
			expMetrics: map[string][]promql.Sample{
				"PendingAndFiringAndResolved": {
					{
						Point: promql.Point{
							T: 1640145082,
							V: 1,
						},
						Metric: labels.FromStrings("__name__", "ALERTS", "alertname", "PendingAndFiringAndResolved_SimpleAlert", "alertstate", "pending", "foo", "bar", "rulegroup", "PendingAndFiringAndResolved"),
					}, {
						Point: promql.Point{
							T: 1640145085,
							V: 1,
						},
						Metric: labels.FromStrings("__name__", "ALERTS", "alertname", "PendingAndFiringAndResolved_SimpleAlert2", "alertstate", "firing", "foo", "bar", "rulegroup", "PendingAndFiringAndResolved"),
					},
				},
				"AnotherGroup": {
					{
						Point: promql.Point{
							T: 1640145083,
							V: 1,
						},
						Metric: labels.FromStrings("__name__", "ALERTS", "alertname", "AnotherGroup_SimpleAlert", "alertstate", "firing", "foo", "bar", "rulegroup", "AnotherGroup"),
					},
				},
			},
		},
	}

	for _, c := range cases {
		act, err := ParseAndGroupMetrics(([]byte)(c.response))
		require.NoError(t, err)
		require.Equal(t, c.expMetrics, act)
	}
}

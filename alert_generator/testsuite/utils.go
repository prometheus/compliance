package testsuite

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	v1 "github.com/prometheus/prometheus/web/api/v1"
)

// TODO: add retries and set some timeouts.
func DoGetRequest(u string) ([]byte, error) {
	resp, err := http.Get(u)
	if err != nil {
		return nil, errors.Wrap(err, "get request")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("non 200 response code %q", resp.StatusCode)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read body")
	}

	return b, nil
}

// ParseAndGroupAlerts parses the alerts and groups by the rule group name.
// The alerts are assumed to have a `rulegroup` label.
func ParseAndGroupAlerts(b []byte) (map[string][]v1.Alert, error) {
	var res GETAlertsResponse
	err := json.Unmarshal(b, &res)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal response into json")
	}

	if res.Status != "success" {
		return nil, errors.Errorf("got non success status %q", res.Status)
	}

	// Group alerts based on group name via the "rulegroup" label.
	mappedAlerts := make(map[string][]v1.Alert)
	for _, al := range res.Data.Alerts {
		groupName := al.Labels.Get("rulegroup")
		mappedAlerts[groupName] = append(mappedAlerts[groupName], al)
	}

	return mappedAlerts, nil
}

// ParseAndGroupMetrics parses samples and groups by the rule group name.
// The metrics are assumed to have a `rulegroup` label.
func ParseAndGroupMetrics(b []byte) (map[string][]promql.Sample, error) {

	var res GETMetricsResponse
	err := json.Unmarshal(b, &res)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal response into json")
	}

	if res.Status != "success" {
		return nil, errors.Errorf("got non success status %q", res.Status)
	}

	// Group metrics based on group name via the "rulegroup" label.
	mappedMetrics := make(map[string][]promql.Sample)
	for _, s := range res.Data.Result {
		groupName := s.Metric.Get("rulegroup")
		ts, vs := int64(s.Value[0].(float64)), s.Value[1].(string)
		val, err := strconv.ParseFloat(vs, 64)
		if err != nil {
			return nil, err
		}
		mappedMetrics[groupName] = append(mappedMetrics[groupName], promql.Sample{
			Point: promql.Point{
				T: ts,
				V: val,
			},
			Metric: s.Metric,
		})
	}

	return mappedMetrics, nil
}

type GETAlertsResponse struct {
	Status string `json:"status"`
	Data   Alerts `json:"data"`
}

type Alerts struct {
	Alerts []v1.Alert `json:"alerts"`
}

type GETMetricsResponse struct {
	Status string  `json:"status"`
	Data   Metrics `json:"data"`
}

type Metrics struct {
	ResultType string   `json:"resultType"`
	Result     []Vector `json:"result"`
}

type Vector struct {
	Metric labels.Labels  `json:"metric"`
	Value  [2]interface{} `json:"value"`
}

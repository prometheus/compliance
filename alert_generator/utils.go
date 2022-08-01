package testsuite

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/common/sigv4"
	"github.com/prometheus/compliance/alert_generator/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	v1 "github.com/prometheus/prometheus/web/api/v1"
)

// TODO: add retries and set some timeouts.
func DoGetRequest(u string, auth config.AuthConfig) ([]byte, error) {

	// Give the GET request empty body instead of nil to avoid segmentation fault
	// when doing sigv4 signing.
	req, err := http.NewRequest("GET", u, strings.NewReader(""))
	if err != nil {
		return nil, err
	}

	client := &http.Client{}
	transport := client.Transport
	if auth.SigV4Config != nil {
		transport, err = sigv4.NewSigV4RoundTripper(auth.SigV4Config, transport)
		if err != nil {
			return nil, err
		}
	} else if auth.BasicAuthUser != "" {
		req.SetBasicAuth(auth.BasicAuthUser, auth.BasicAuthPass)
	}

	client.Transport = transport
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "get request")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("non 200 response code %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
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

type GETAlertsResponse struct {
	Status string `json:"status"`
	Data   Alerts `json:"data"`
}

type Alerts struct {
	Alerts []v1.Alert `json:"alerts"`
}

// ParseAndGroupRules parses the rules and groups by the rule group name.
// The rules are assumed to have a `rulegroup` label.
func ParseAndGroupRules(b []byte) (map[string]*v1.RuleGroup, error) {
	var res GETRulesResponse
	err := json.Unmarshal(b, &res)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal response into json")
	}

	if res.Status != "success" {
		return nil, errors.Errorf("got non success status %q", res.Status)
	}

	mappedGroups := make(map[string]*v1.RuleGroup)
	for _, g := range res.Data.RuleGroups {
		rg := &v1.RuleGroup{
			Name:           g.Name,
			File:           g.File,
			Interval:       g.Interval,
			EvaluationTime: g.EvaluationTime,
			LastEvaluation: g.LastEvaluation.UTC(),
		}
		for _, r := range g.Rules {
			r.LastEvaluation = r.LastEvaluation.UTC()
			rg.Rules = append(rg.Rules, r)
		}
		mappedGroups[g.Name] = rg
	}

	return mappedGroups, nil
}

type GETRulesResponse struct {
	Status string `json:"status"`
	Data   Data   `json:"data"`
}

type Data struct {
	RuleGroups []*RuleGroup `json:"groups"`
}

type RuleGroup struct {
	Name           string            `json:"name"`
	File           string            `json:"file"`
	Rules          []v1.AlertingRule `json:"rules"`
	Interval       float64           `json:"interval"`
	EvaluationTime float64           `json:"evaluationTime"`
	LastEvaluation time.Time         `json:"lastEvaluation"`
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

// Copy pasted from github.com/prometheus/prometheus/tsdb/errors to prevent go.mod errors.

// multiError type allows combining multiple errors into one.
type multiError []error

// NewMulti returns multiError with provided errors added if not nil.
func NewMulti(errs ...error) multiError { // nolint:golint
	m := multiError{}
	m.Add(errs...)
	return m
}

// Add adds single or many errors to the error list. Each error is added only if not nil.
// If the error is a nonNilMultiError type, the errors inside nonNilMultiError are added to the main multiError.
func (es *multiError) Add(errs ...error) {
	for _, err := range errs {
		if err == nil {
			continue
		}
		if merr, ok := err.(nonNilMultiError); ok {
			*es = append(*es, merr.errs...)
			continue
		}
		*es = append(*es, err)
	}
}

// Err returns the error list as an error or nil if it is empty.
func (es multiError) Err() error {
	if len(es) == 0 {
		return nil
	}
	return nonNilMultiError{errs: es}
}

// nonNilMultiError implements the error interface, and it represents
// multiError with at least one error inside it.
// This type is needed to make sure that nil is returned when no error is combined in multiError for err != nil
// check to work.
type nonNilMultiError struct {
	errs multiError
}

// Error returns a concatenated string of the contained errors.
func (es nonNilMultiError) Error() string {
	var buf bytes.Buffer

	if len(es.errs) > 1 {
		fmt.Fprintf(&buf, "%d errors: ", len(es.errs))
	}

	for i, err := range es.errs {
		if i != 0 {
			buf.WriteString("; ")
		}
		buf.WriteString(err.Error())
	}

	return buf.String()
}

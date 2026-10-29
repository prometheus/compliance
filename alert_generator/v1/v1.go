package v1

import (
	"time"

	"github.com/prometheus/prometheus/model/labels"
)

// Alert models the /api/v1/alerts response item from Prometheus.
type Alert struct {
	Labels      labels.Labels `json:"labels"`
	Annotations labels.Labels `json:"annotations"`
	State       string        `json:"state"`
	ActiveAt    *time.Time    `json:"activeAt,omitempty"`
	Value       string        `json:"value"`
}

// Rule models a recording or alerting rule in /api/v1/rules.
type Rule interface{}

// AlertingRule models an alerting rule in /api/v1/rules.
type AlertingRule struct {
	State          string        `json:"state"`
	Name           string        `json:"name"`
	Query          string        `json:"query"`
	Duration       float64       `json:"duration"`
	KeepFiringFor  float64       `json:"keepFiringFor,omitempty"`
	Labels         labels.Labels `json:"labels"`
	Annotations    labels.Labels `json:"annotations"`
	Alerts         []*Alert      `json:"alerts"`
	Health         string        `json:"health"`
	LastError      string        `json:"lastError,omitempty"`
	EvaluationTime float64       `json:"evaluationTime"`
	LastEvaluation time.Time     `json:"lastEvaluation"`
	Type           string        `json:"type"`
}

// RuleGroup models a rule group in /api/v1/rules.
type RuleGroup struct {
	Name           string    `json:"name"`
	File           string    `json:"file"`
	Rules          []Rule    `json:"rules"`
	Interval       float64   `json:"interval"`
	EvaluationTime float64   `json:"evaluationTime"`
	LastEvaluation time.Time `json:"lastEvaluation"`
}

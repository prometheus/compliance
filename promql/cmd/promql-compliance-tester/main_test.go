package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/prometheus/compliance/promql/comparer"
	"github.com/prometheus/compliance/promql/config"
	"github.com/prometheus/compliance/promql/testcases"
)

type fakePromAPI struct {
	val model.Value
	err error
}

var _ comparer.PromAPI = (*fakePromAPI)(nil)

func (f *fakePromAPI) Query(context.Context, string, time.Time, ...v1.Option) (model.Value, v1.Warnings, error) {
	return f.val, nil, f.err
}

func (f *fakePromAPI) QueryRange(context.Context, string, v1.Range, ...v1.Option) (model.Value, v1.Warnings, error) {
	return f.val, nil, f.err
}

func TestCompareTestCase_ReferenceMismatchRecordedAsFailure(t *testing.T) {
	for _, tc := range []struct {
		name       string
		refAPI     *fakePromAPI
		testAPI    *fakePromAPI
		testCase   *comparer.TestCase
		wantSubstr string
	}{
		{
			name:       "reference=unexpected_success_on_should_fail",
			refAPI:     &fakePromAPI{val: model.Matrix{}},
			testAPI:    &fakePromAPI{val: model.Matrix{}},
			testCase:   &comparer.TestCase{Query: `label_replace(demo_num_cpus, "~invalid", "", "src", "(.*)")`, ShouldFail: true},
			wantSubstr: "expected reference API query",
		},
		{
			name:       "reference=unexpected_error",
			refAPI:     &fakePromAPI{err: errors.New("connection refused")},
			testAPI:    &fakePromAPI{val: model.Matrix{}},
			testCase:   &comparer.TestCase{Query: "demo_num_cpus", ShouldFail: false},
			wantSubstr: "error querying reference API",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			comp := comparer.New(tc.refAPI, tc.testAPI, nil)
			res := compareTestCase(comp, tc.testCase)
			if res == nil {
				t.Fatal("expected non-nil result")
			}
			if res.Success() {
				t.Fatal("expected failed result, got success")
			}
			if !strings.Contains(res.UnexpectedFailure, tc.wantSubstr) {
				t.Fatalf("expected UnexpectedFailure to contain %q, got %q", tc.wantSubstr, res.UnexpectedFailure)
			}
		})
	}
}

func TestPromQLTestQueriesConfig(t *testing.T) {
	cfg, err := config.LoadFromFiles([]string{"../../promql-test-queries.yml"})
	if err != nil {
		t.Fatalf("failed to load promql-test-queries.yml: %v", err)
	}

	end := time.Now().UTC()
	start := end.Add(-10 * time.Minute)
	expanded := testcases.ExpandTestCases(cfg.TestCases, nil, start, end, 10*time.Second)
	if len(expanded) == 0 {
		t.Fatal("expected expanded test cases, got none")
	}

	byQuery := make(map[string]*comparer.TestCase, len(expanded))
	for _, tc := range expanded {
		byQuery[tc.Query] = tc
	}

	for _, exp := range []struct {
		query      string
		shouldFail bool
	}{
		{
			query:      `label_replace(demo_num_cpus, "~valid", "", "src", "(.*)")`,
			shouldFail: false,
		},
		{
			query:      `label_replace(demo_num_cpus, "\xff", "", "src", "(.*)")`,
			shouldFail: true,
		},
		{
			query:      `label_join(demo_num_cpus, "~valid", "-", "instance")`,
			shouldFail: false,
		},
		{
			query:      `label_join(demo_num_cpus, "\xff", "-", "instance")`,
			shouldFail: true,
		},
	} {
		tc, ok := byQuery[exp.query]
		if !ok {
			t.Errorf("missing expected query %q in promql-test-queries.yml", exp.query)
			continue
		}
		if tc.ShouldFail != exp.shouldFail {
			t.Errorf("query %q: got should_fail=%v, want %v", exp.query, tc.ShouldFail, exp.shouldFail)
		}
	}
}

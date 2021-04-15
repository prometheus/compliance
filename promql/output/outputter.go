package output

import (
	"github.com/prometheus/compliance/promql/comparer"
	"github.com/prometheus/compliance/promql/config"
)

// An Outputter outputs a number of test results.
type Outputter func(results []*comparer.Result, includePassing bool, tweaks []*config.QueryTweak)

package output

import (
	"github.com/promlabs/promql-compliance-tester/comparer"
	"github.com/promlabs/promql-compliance-tester/config"
)

// An Outputter outputs a number of test results.
type Outputter func(results []*comparer.Result, includePassing bool, tweaks []*config.QueryTweak)

package v2

import (
	"testing"

	"github.com/prometheus/compliance/remotewrite/sender/cases"
)

func CTZeroSampleTest() cases.Test {
	return cases.Test{
		Name: "CTZeroSampleV2",
		Metrics: cases.StaticHandler([]byte(`# HELP ct_zero_test_v2 V2 counter reset placeholder test
# TYPE ct_zero_test_v2 counter  
ct_zero_test_v2 1.0
`)),
		Expected: func(t *testing.T, bs []cases.Batch) {
			// TODO: Implement v2 CT zero sample validation
		},
	}
}

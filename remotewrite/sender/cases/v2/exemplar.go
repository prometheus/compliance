package v2

import (
	"testing"

	"github.com/prometheus/compliance/remotewrite/sender/cases"
)

func ExemplarTest() cases.Test {
	return cases.Test{
		Name: "ExemplarV2",
		Metrics: cases.StaticHandler([]byte(`# HELP exemplar_test_v2 V2 exemplar placeholder test
# TYPE exemplar_test_v2 counter
exemplar_test_v2 1.0
`)),
		Expected: func(t *testing.T, bs []cases.Batch) {
			// TODO: Implement v2 exemplar validation
		},
	}
}
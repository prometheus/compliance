package cases

import (
	"testing"
)

func ExemplarTest() Test {
	return Test{
		Name: "ExemplarV2",
		Metrics: staticHandler([]byte(`# HELP exemplar_test_v2 V2 exemplar placeholder test
# TYPE exemplar_test_v2 counter
exemplar_test_v2 1.0
`)),
		Expected: func(t *testing.T, bs []Batch) {
			// TODO: Implement v2 exemplar validation
		},
	}
}

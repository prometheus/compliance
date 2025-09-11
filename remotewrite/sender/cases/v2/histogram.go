package v2

import (
	"testing"

	"github.com/prometheus/compliance/remotewrite/sender/cases"
)

func HistogramTest() cases.Test {
	return cases.Test{
		Name: "HistogramV2",
		Metrics: cases.StaticHandler([]byte(`# HELP histogram_test_v2 V2 histogram placeholder test
# TYPE histogram_test_v2 histogram
histogram_test_v2 1.0
`)),
		Expected: func(t *testing.T, bs []cases.Batch) {
			// TODO: Implement v2 histogram validation
		},
	}
}
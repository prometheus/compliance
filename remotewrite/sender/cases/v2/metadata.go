package v2

import (
	"testing"

	"github.com/prometheus/compliance/remotewrite/sender/cases"
)

func MetadataTest() cases.Test {
	return cases.Test{
		Name: "MetadataV2",
		Metrics: cases.StaticHandler([]byte(`# HELP metadata_test_v2 V2 metadata placeholder test  
# TYPE metadata_test_v2 counter
metadata_test_v2 1.0
`)),
		Expected: func(t *testing.T, bs []cases.Batch) {
			// TODO: Implement v2 metadata validation
		},
	}
}
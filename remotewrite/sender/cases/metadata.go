package cases

import (
	"testing"
)

func MetadataTest() Test {
	return Test{
		Name: "MetadataV2",
		Metrics: StaticHandler([]byte(`# HELP metadata_test_v2 V2 metadata placeholder test  
# TYPE metadata_test_v2 counter
metadata_test_v2 1.0
`)),
		Expected: func(t *testing.T, bs []Batch) {
			// TODO: Implement v2 metadata validation
		},
	}
}

module github.com/prometheus/compliance/promql

go 1.26.0

require (
	github.com/google/go-cmp v0.7.0
	github.com/prometheus/client_golang v1.25.0
	github.com/prometheus/common v0.72.0
	github.com/prometheus/compliance/common v0.0.0-20261028123049-5aa00544fbe6
	gopkg.in/yaml.v2 v2.4.0
)

replace github.com/prometheus/compliance/common => ../common

require (
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

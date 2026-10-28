module github.com/prometheus/compliance/openmetrics

go 1.26.0

// TODO(bwplotka): Update github.com/prometheus/common to a released version once https://github.com/prometheus/common/pull/1016 is merged.
require (
	github.com/prometheus/client_golang v1.24.2-0.20261028105007-0b2f01f00376
	github.com/prometheus/client_model v0.6.3
	github.com/prometheus/common v0.72.2-0.20261027144952-e257672f3db6
	github.com/prometheus/compliance/common v0.0.0
	github.com/stretchr/testify v1.12.1
	google.golang.org/protobuf v1.36.12
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/pandatix/go-abnf v0.5.1 // indirect
	github.com/prometheus/procfs v0.22.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

replace github.com/prometheus/compliance/common => ../common

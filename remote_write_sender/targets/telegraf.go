package targets

import (
	"fmt"
	"os"
)

const telegrafURL = "https://dl.influxdata.com/telegraf/releases/telegraf-1.20.2_{{.OS}}_{{.Arch}}.tar.gz"

func RunTelegraf(opts TargetOptions) error {
	binary, err := downloadBinary(telegrafURL, "telegraf")
	if err != nil {
		return err
	}

	cfg := fmt.Sprintf(`
[[inputs.prometheus]]
	## An array of urls to scrape metrics from.
	urls = ["http://%s/metrics"]
	metric_version = 2
	url_tag = "instance"

[[processors.override]]
	[processors.override.tags]
		job = "test"

[[processors.regex]]
	[[processors.regex.tags]]
	    key = "instance"
	    pattern = "http://([^/]+)/metrics"
	    replacement = "${1}"

[[outputs.http]]
	url = "%s"
	data_format = "prometheusremotewrite"
	[outputs.http.headers]
	   Content-Type = "application/x-protobuf"
	   Content-Encoding = "snappy"
	   X-Prometheus-Remote-Write-Version = "0.1.0"
`, opts.ScrapeTarget, opts.ReceiveEndpoint)
	configFileName, err := writeTempFile(cfg, "config-*.toml")
	if err != nil {
		return err
	}
	defer os.Remove(configFileName)

	return runCommand(binary, opts.Timeout, fmt.Sprintf("--config=%s", configFileName))
}

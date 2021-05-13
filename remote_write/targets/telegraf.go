package targets

import (
	"fmt"
	"os"

        "github.com/prometheus/compliance/remote_write/latest"
)

func RunTelegraf(opts TargetOptions) error {
	version := "1.18.2"
	binary, err := downloadBinary(latest.GetDownloadURL("https://dl.influxdata.com/telegraf/releases/telegraf-" + version + "_{{.OS}}_{{.Arch}}.tar.gz"), "telegraf")
	fmt.Println("Telegraf version needs to be updated by hand, for now")
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
